package com.ausf.controlplane.authentication;

import com.ausf.controlplane.crypto.Milenage;
import com.ausf.controlplane.crypto.SuciDeconcealer;
import com.ausf.controlplane.eap.EapPacket;
import com.ausf.controlplane.udm.AuthenticationVector;
import com.ausf.controlplane.udm.UdmAuthenticationData;
import com.ausf.controlplane.udm.UdmClient;
import io.micrometer.observation.annotation.Observed;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

@Service
public class AuthenticationManager {

    private static final String SUCI_PREFIX = "suci-";

    /**
     * Maximum number of SYNC_FAILURE (AUTS / SQN resync) cycles per auth context.
     * 0 means unlimited (default for backward compatibility).
     * Configurable via {@code ausf.auth.maxSyncFailures} property.
     */
    @Value("${ausf.auth.maxSyncFailures:0}")
    int maxSyncFailures;

    /**
     * Maximum number of EAP-AKA' ongoing round-trips (re-auth + fast-reauth + sync-failure)
     * per auth context. 0 means unlimited.
     * Configurable via {@code ausf.auth.maxEapOngoing} property.
     */
    @Value("${ausf.auth.maxEapOngoing:0}")
    int maxEapOngoing;

    private final Map<String, AuthenticationContext> authContexts = new ConcurrentHashMap<>();
    private final Map<String, String> latestAuthCtxIdsBySupi = new ConcurrentHashMap<>();
    private final CryptographyService cryptographyService;
    private final UdmClient udmClient;
    private final SuciDeconcealer suciDeconcealer;

    public AuthenticationManager(CryptographyService cryptographyService, UdmClient udmClient) {
        this(cryptographyService, udmClient, new SuciDeconcealer("", ""));
    }

    public AuthenticationManager(CryptographyService cryptographyService, UdmClient udmClient, SuciDeconcealer suciDeconcealer) {
        this.cryptographyService = cryptographyService;
        this.udmClient = udmClient;
        this.suciDeconcealer = suciDeconcealer;
    }

    @Observed(name = "ausf.auth.initiate", contextualName = "initiate-authentication")
    public AuthenticationResponse initiateAuthentication(String authCtxId, String supiOrSuci, String servingNetworkName, String authType) {
        String supi;
        try {
            supi = resolveSupi(supiOrSuci);
        } catch (IllegalArgumentException | IllegalStateException e) {
            return AuthenticationResponse.failure(e.getMessage(), "SUBSCRIBER_NOT_FOUND");
        }

        Optional<UdmAuthenticationData> udmAuthenticationData;
        try {
            udmAuthenticationData = udmClient.getAuthenticationData(supi, servingNetworkName, authType);
        } catch (IllegalStateException exception) {
            return AuthenticationResponse.failure(exception.getMessage(), "CONTROL_PLANE_UNAVAILABLE");
        }

        if (udmAuthenticationData.isEmpty()) {
            return AuthenticationResponse.failure("subscriber not found in UDM storage", "SUBSCRIBER_NOT_FOUND");
        }

        UdmAuthenticationData authenticationData = udmAuthenticationData.get();
        AuthenticationVector vector = authenticationData.getAuthenticationVector();
        String resolvedAuthCtxId = authCtxId == null || authCtxId.isBlank() ? UUID.randomUUID().toString() : authCtxId;
        AuthenticationContext context = new AuthenticationContext(resolvedAuthCtxId, supi);
        context.setAuthType(authenticationData.getAuthType());
        context.setServingNetworkName(authenticationData.getServingNetworkName());
        context.setRand(vector.getRand());
        context.setAutn(vector.getAutn());
        context.setAuts(vector.getAuts());
        context.setHxresStar(vector.getHxresStar());
        context.setXresStar(vector.getXresStar());
        context.setKausf(vector.getKausf());
        if ("EAP_AKA_PRIME".equals(context.getAuthType())) {
            context.setEapChallenge(buildEapChallenge(context));
        }
        context.setAuthEventId(authenticationData.getAuthEventId());
        context.setStatus(AuthenticationStatus.CHALLENGE_SENT);
        authContexts.put(resolvedAuthCtxId, context);
        latestAuthCtxIdsBySupi.put(supi, resolvedAuthCtxId);

        return AuthenticationResponse.challenge(
            resolvedAuthCtxId,
            supi,
            context.getAuthType(),
            context.getServingNetworkName(),
            context.getRand(),
            context.getAutn(),
            context.getHxresStar(),
            "EAP_AKA_PRIME".equals(context.getAuthType()) ? context.getEapChallenge() : null
        );
    }

    @Observed(name = "ausf.auth.confirm", contextualName = "verify-auth-response")
    public AuthenticationResponse verifyAuthenticationResponse(String authCtxId, String resStar, String auts, String eapPayload) {
        AuthenticationContext context = authContexts.get(authCtxId);
        if (context == null || context.isExpired()) {
            return AuthenticationResponse.failure("authentication context missing or expired", "CONTEXT_NOT_FOUND");
        }
        if (!isLatestPendingContext(context, authCtxId)) {
            return AuthenticationResponse.failure("authentication context is no longer pending", "AUTHENTICATION_REJECTED");
        }

        if ("5G_AKA".equals(context.getAuthType()) && !isValidFiveGAkaConfirmation(resStar, auts, context)) {
            context.setStatus(AuthenticationStatus.FAILED);
            notifyAuthResult(context, false);
            return AuthenticationResponse.failure("5G_AKA confirmation must provide exactly one of RES* or AUTS", "AUTHENTICATION_REJECTED");
        }

        if ("5G_AKA".equals(context.getAuthType()) && auts != null && !auts.isBlank()) {
            if (!cryptographyService.verifyAuthentication(context.getAuts(), auts)) {
                context.setStatus(AuthenticationStatus.FAILED);
                notifyAuthResult(context, false);
                return AuthenticationResponse.failure("AUTS verification failed", "AUTHENTICATION_REJECTED");
            }
            return regenerateChallenge(context, auts);
        }

        if ("EAP_AKA_PRIME".equals(context.getAuthType())) {
            EapPacket.DecodedResponse decoded = EapPacket.decodeResponse(eapPayload);
            if (decoded == null) {
                context.setStatus(AuthenticationStatus.FAILED);
                notifyAuthResult(context, false);
                return AuthenticationResponse.failure("invalid EAP payload", "AUTHENTICATION_REJECTED", buildEapFailure(context));
            }
            byte[] rawPacket = decodeBase64url(eapPayload);
            byte[] kAutPrime = cryptographyService.deriveKautPrime(context.getKausf());
            if (rawPacket != null && !EapPacket.verifyMac(kAutPrime, rawPacket)) {
                context.setStatus(AuthenticationStatus.FAILED);
                notifyAuthResult(context, false);
                return AuthenticationResponse.failure("EAP AT_MAC verification failed", "AUTHENTICATION_REJECTED", buildEapFailure(context));
            }
            if (decoded.subtype() == EapPacket.SUBTYPE_AKA_REAUTHENTICATION) {
                return regenerateEapAkaPrimeChallenge(context, decoded.hasFastReauth());
            }
            if (decoded.subtype() == EapPacket.SUBTYPE_AKA_SYNC_FAILURE) {
                if (decoded.auts() == null) {
                    context.setStatus(AuthenticationStatus.FAILED);
                    notifyAuthResult(context, false);
                    return AuthenticationResponse.failure("EAP sync-failure missing AT_AUTS", "AUTHENTICATION_REJECTED", buildEapFailure(context));
                }
                return regenerateChallenge(context, Milenage.bytesToHex(decoded.auts()));
            }
            if (decoded.subtype() != EapPacket.SUBTYPE_AKA_CHALLENGE || decoded.resStar() == null) {
                context.setStatus(AuthenticationStatus.FAILED);
                notifyAuthResult(context, false);
                return AuthenticationResponse.failure("EAP-AKA' verification failed", "AUTHENTICATION_REJECTED", buildEapFailure(context));
            }
            String candidateResStar = Milenage.bytesToHex(decoded.resStar());
            if (!cryptographyService.verifyAuthentication(context.getXresStar(), candidateResStar)) {
                context.setStatus(AuthenticationStatus.FAILED);
                notifyAuthResult(context, false);
                return AuthenticationResponse.failure("EAP-AKA' verification failed", "AUTHENTICATION_REJECTED", buildEapFailure(context));
            }
            context.setResStar(candidateResStar);
            context.setKseaf(cryptographyService.deriveKseaf(context.getKausf(), context.getServingNetworkName()));
            context.setStatus(AuthenticationStatus.AUTHENTICATED);
            notifyAuthResult(context, true);
            return AuthenticationResponse.success(context.getAuthCtxId(), context.getSupi(), context.getAuthType(), context.getKseaf());
        }

        if (!cryptographyService.verifyAuthentication(context.getXresStar(), resStar)) {
            context.setStatus(AuthenticationStatus.FAILED);
            notifyAuthResult(context, false);
            return AuthenticationResponse.failure("RES* verification failed", "AUTHENTICATION_REJECTED");
        }

        context.setResStar(resStar);
        context.setKseaf(cryptographyService.deriveKseaf(context.getKausf(), context.getServingNetworkName()));
        context.setStatus(AuthenticationStatus.AUTHENTICATED);
        notifyAuthResult(context, true);
        return AuthenticationResponse.success(context.getAuthCtxId(), context.getSupi(), context.getAuthType(), context.getKseaf());
    }

    public AuthenticationContext getContext(String supi) {
        String authCtxId = latestAuthCtxIdsBySupi.get(supi);
        return authCtxId == null ? null : authContexts.get(authCtxId);
    }

    private boolean isLatestPendingContext(AuthenticationContext context, String authCtxId) {
        return AuthenticationStatus.CHALLENGE_SENT == context.getStatus()
            && authCtxId.equals(latestAuthCtxIdsBySupi.get(context.getSupi()));
    }

    private boolean isValidFiveGAkaConfirmation(String resStar, String auts, AuthenticationContext context) {
        boolean hasResStar = resStar != null && !resStar.isBlank();
        boolean hasAuts = auts != null && !auts.isBlank();
        return hasResStar ^ hasAuts;
    }

    /** Build binary EAP-Request/AKA'-Challenge from the current context state. */
    private String buildEapChallenge(AuthenticationContext context) {
        byte[] rand     = hexToBytes(context.getRand());
        byte[] autn     = hexToBytes(context.getAutn());
        byte[] kAutPrime = cryptographyService.deriveKautPrime(context.getKausf());
        return EapPacket.buildChallenge(rand, autn, context.getServingNetworkName(), kAutPrime);
    }

    /** Build binary EAP-Failure matching the identifier of the stored EAP-Request. */
    private String buildEapFailure(AuthenticationContext context) {
        byte id = EapPacket.extractIdentifier(context.getEapChallenge());
        return EapPacket.buildFailure(id);
    }

    private static byte[] hexToBytes(String hex) {
        return Milenage.hexToBytes(hex);
    }

    private static byte[] decodeBase64url(String s) {
        if (s == null || s.isBlank()) return null;
        try {
            int rem = s.length() % 4;
            String padded = rem == 0 ? s : s + "=".repeat(4 - rem);
            return java.util.Base64.getUrlDecoder().decode(padded);
        } catch (Exception e) {
            return null;
        }
    }

    private void notifyAuthResult(AuthenticationContext context, boolean success) {
        udmClient.confirmAuthEvent(
            context.getSupi(),
            context.getAuthEventId(),
            success,
            context.getAuthType(),
            context.getServingNetworkName()
        );
    }

    private AuthenticationResponse regenerateEapAkaPrimeChallenge(AuthenticationContext context, boolean fastReauthentication) {
        int newCount = context.incrementAndGetEapOngoingCount();
        if (maxEapOngoing > 0 && newCount > maxEapOngoing) {
            context.setStatus(AuthenticationStatus.FAILED);
            notifyAuthResult(context, false);
            return AuthenticationResponse.failure(
                "EAP-AKA' max ongoing round-trips exceeded (" + maxEapOngoing + ")",
                "AUTHENTICATION_REJECTED",
                buildEapFailure(context)
            );
        }

        Optional<UdmAuthenticationData> udmAuthenticationData;
        try {
            udmAuthenticationData = udmClient.getAuthenticationData(
                context.getSupi(),
                context.getServingNetworkName(),
                context.getAuthType()
            );
        } catch (IllegalStateException exception) {
            return AuthenticationResponse.failure(exception.getMessage(), "CONTROL_PLANE_UNAVAILABLE");
        }

        if (udmAuthenticationData.isEmpty()) {
            return AuthenticationResponse.failure("subscriber not found in UDM storage", "SUBSCRIBER_NOT_FOUND");
        }

        context.setAuthEventId(udmAuthenticationData.get().getAuthEventId());
        AuthenticationVector vector = udmAuthenticationData.get().getAuthenticationVector();
        context.setRand(vector.getRand());
        context.setAutn(vector.getAutn());
        context.setAuts(vector.getAuts());
        context.setHxresStar(vector.getHxresStar());
        context.setXresStar(vector.getXresStar());
        context.setKausf(vector.getKausf());
        context.setResStar(null);
        context.setKseaf(null);
        context.setEapChallenge(buildEapChallenge(context));
        context.setStatus(AuthenticationStatus.CHALLENGE_SENT);

        return fastReauthentication
            ? AuthenticationResponse.fastReauthenticationChallenge(
                context.getAuthCtxId(),
                context.getSupi(),
                context.getAuthType(),
                context.getServingNetworkName(),
                context.getRand(),
                context.getAutn(),
                context.getHxresStar(),
                context.getEapChallenge()
            )
            : AuthenticationResponse.reauthenticationChallenge(
            context.getAuthCtxId(),
            context.getSupi(),
            context.getAuthType(),
            context.getServingNetworkName(),
            context.getRand(),
            context.getAutn(),
            context.getHxresStar(),
            context.getEapChallenge()
            );
    }

    private AuthenticationResponse regenerateChallenge(AuthenticationContext context, String auts) {
        int newCount = context.incrementAndGetSyncFailureCount();
        if (maxSyncFailures > 0 && newCount > maxSyncFailures) {
            context.setStatus(AuthenticationStatus.FAILED);
            notifyAuthResult(context, false);
            String eapFailure = "EAP_AKA_PRIME".equals(context.getAuthType()) ? buildEapFailure(context) : null;
            return AuthenticationResponse.failure(
                "max SYNC_FAILURE attempts exceeded (" + maxSyncFailures + ")",
                "AUTHENTICATION_REJECTED",
                eapFailure
            );
        }

        Optional<UdmAuthenticationData> udmAuthenticationData;
        try {
            udmAuthenticationData = udmClient.resynchronizeAuthenticationData(
                context.getSupi(),
                context.getServingNetworkName(),
                context.getAuthType(),
                context.getRand(),
                auts
            );
        } catch (IllegalArgumentException exception) {
            context.setStatus(AuthenticationStatus.FAILED);
            notifyAuthResult(context, false);
            return AuthenticationResponse.failure(exception.getMessage(), "AUTHENTICATION_REJECTED");
        } catch (IllegalStateException exception) {
            return AuthenticationResponse.failure(exception.getMessage(), "CONTROL_PLANE_UNAVAILABLE");
        }

        if (udmAuthenticationData.isEmpty()) {
            return AuthenticationResponse.failure("subscriber not found in UDM storage", "SUBSCRIBER_NOT_FOUND");
        }

        context.setAuthEventId(udmAuthenticationData.get().getAuthEventId());
        AuthenticationVector vector = udmAuthenticationData.get().getAuthenticationVector();
        context.setRand(vector.getRand());
        context.setAutn(vector.getAutn());
        context.setAuts(vector.getAuts());
        context.setHxresStar(vector.getHxresStar());
        context.setXresStar(vector.getXresStar());
        context.setKausf(vector.getKausf());
        context.setResStar(null);
        context.setKseaf(null);
        if ("EAP_AKA_PRIME".equals(context.getAuthType())) {
            context.setEapChallenge(buildEapChallenge(context));
        }
        context.setStatus(AuthenticationStatus.CHALLENGE_SENT);

        if ("EAP_AKA_PRIME".equals(context.getAuthType())) {
            return AuthenticationResponse.synchronizationFailureChallenge(
                context.getAuthCtxId(),
                context.getSupi(),
                context.getAuthType(),
                context.getServingNetworkName(),
                context.getRand(),
                context.getAutn(),
                context.getHxresStar(),
                context.getEapChallenge()
            );
        }

        return AuthenticationResponse.resynchronizationChallenge(
            context.getAuthCtxId(),
            context.getSupi(),
            context.getAuthType(),
            context.getServingNetworkName(),
            context.getRand(),
            context.getAutn(),
            context.getHxresStar(),
            null
        );
    }

    // ── SoR Protection (TS 29.509 §6.2, TS 33.501 A.18) ─────────────────────

    /**
     * Compute SoR MAC for the given auth context.
     * Requires that the context has already been authenticated (KAUSF present).
     *
     * @param authCtxId        the authentication context identifier
     * @param steeringContainer hex-encoded steering container bytes
     * @param ackIndication    whether UE acknowledgement is requested
     * @return ProtectionResult containing sorMacIausf (32 hex chars) and counterSor (4 hex chars)
     */
    public ProtectionResult computeSoRProtection(String authCtxId, String steeringContainer, boolean ackIndication) {
        return computeSoRProtection(authCtxId, steeringContainer, ackIndication, null);
    }

    public ProtectionResult computeSoRProtection(String authCtxId, String steeringContainer, boolean ackIndication, String sorHeader) {
        AuthenticationContext context = authContexts.get(authCtxId);
        if (context == null) {
            throw new IllegalArgumentException("Authentication context not found: " + authCtxId);
        }
        if (context.getStatus() != AuthenticationStatus.AUTHENTICATED || context.getKausf() == null) {
            throw new IllegalStateException("Authentication context is not in AUTHENTICATED state with KAUSF present");
        }
        int counterValue = context.getAndIncrementSorCounter();
        byte[] counter = new byte[]{(byte) (counterValue >> 8), (byte) (counterValue & 0xFF)};
        String ksorsaf  = cryptographyService.deriveKsorsaf(context.getKausf(), context.getServingNetworkName(), counter);
        String macSoR   = cryptographyService.computeProtectionMAC(ksorsaf, counter, sorHeader, steeringContainer, ackIndication);
        String counterHex = String.format("%04x", counterValue);
        return new ProtectionResult(macSoR, counterHex);
    }

    // ── UPU Protection (TS 29.509 §6.3, TS 33.501 A.19) ─────────────────────

    /**
     * Compute UPU MAC for the given auth context.
     */
    public ProtectionResult computeUPUProtection(String authCtxId, String upuData, boolean ackIndication) {
        return computeUPUProtection(authCtxId, upuData, ackIndication, null);
    }

    public ProtectionResult computeUPUProtection(String authCtxId, String upuData, boolean ackIndication, String upuHeader) {
        AuthenticationContext context = authContexts.get(authCtxId);
        if (context == null) {
            throw new IllegalArgumentException("Authentication context not found: " + authCtxId);
        }
        if (context.getStatus() != AuthenticationStatus.AUTHENTICATED || context.getKausf() == null) {
            throw new IllegalStateException("Authentication context is not in AUTHENTICATED state with KAUSF present");
        }
        int counterValue = context.getAndIncrementUpuCounter();
        byte[] counter   = new byte[]{(byte) (counterValue >> 8), (byte) (counterValue & 0xFF)};
        String kupusaf   = cryptographyService.deriveKupusaf(context.getKausf(), context.getServingNetworkName(), counter);
        String macUPU    = cryptographyService.computeProtectionMAC(kupusaf, counter, upuHeader, upuData, ackIndication);
        String counterHex = String.format("%04x", counterValue);
        return new ProtectionResult(macUPU, counterHex);
    }

    // ── SUCI resolution ───────────────────────────────────────────────────────

    /**
     * Resolve a SUPI-or-SUCI to a plaintext SUPI.
     * Null-scheme (protectionSchemeId=0) is handled locally.
     * Profile A/B requires the configured home-network private key.
     *
     * <p>SUCI format (TS 23.003 §2.2B.1):
     * {@code suci-<supiType>-<mcc>-<mnc>-<routingIndicator>-<schemeId>-<keyId>-<schemeOutput>}
     */
    private String resolveSupi(String supiOrSuci) {
        if (!supiOrSuci.startsWith(SUCI_PREFIX)) {
            return supiOrSuci;
        }
        // Split into exactly 8 parts: suci, supiType, mcc, mnc, ri, schemeId, keyId, schemeOutput
        String[] parts = supiOrSuci.split("-", 8);
        if (parts.length != 8) {
            throw new IllegalArgumentException("Invalid SUCI format (expected 7 dashes): " + supiOrSuci);
        }
        if (!"0".equals(parts[1])) {
            throw new IllegalArgumentException("Unsupported SUPI type in SUCI (only IMSI/type-0 supported): " + parts[1]);
        }
        String mcc          = parts[2];
        String mnc          = parts[3];
        int protectionSchemeId;
        try {
            protectionSchemeId = Integer.parseInt(parts[5]);
        } catch (NumberFormatException e) {
            throw new IllegalArgumentException("Invalid SUCI protection scheme ID: " + parts[5]);
        }
        String schemeOutput = parts[7];
        String msin = suciDeconcealer.deconcealment(protectionSchemeId, schemeOutput);
        return "imsi-" + mcc + mnc + msin;
    }

    /** Result type for SoR/UPU protection operations. */
    public record ProtectionResult(String mac, String counter) {}
}
