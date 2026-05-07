package com.ausf.controlplane.authentication;

import com.ausf.controlplane.udm.AuthenticationVector;
import com.ausf.controlplane.udm.UdmAuthenticationData;
import com.ausf.controlplane.udm.UdmClient;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import org.springframework.stereotype.Service;

@Service
public class AuthenticationManager {
    private static final String EAP_AKA_PRIME_RESPONSE_PREFIX = "EAP-Response/AKA'-Challenge RES*=";
    private static final String EAP_AKA_PRIME_REAUTH_RESPONSE_PREFIX = "EAP-Response/AKA'-Reauthentication";
    private static final String EAP_AKA_PRIME_FAST_REAUTH_RESPONSE_PREFIX = "EAP-Response/AKA'-Fast-Reauthentication";
    private static final String EAP_AKA_PRIME_SYNC_FAILURE_PREFIX = "EAP-Response/AKA'-Synchronization-Failure AUTS=";

    private final Map<String, AuthenticationContext> authContexts = new ConcurrentHashMap<>();
    private final Map<String, String> latestAuthCtxIdsBySupi = new ConcurrentHashMap<>();
    private final CryptographyService cryptographyService;
    private final UdmClient udmClient;

    public AuthenticationManager(CryptographyService cryptographyService, UdmClient udmClient) {
        this.cryptographyService = cryptographyService;
        this.udmClient = udmClient;
    }

    public AuthenticationResponse initiateAuthentication(String authCtxId, String supi, String servingNetworkName, String authType) {
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
        context.setEapChallenge(vector.getEapChallenge());
        context.setKausf(vector.getKausf());
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
            return AuthenticationResponse.failure("5G_AKA confirmation must provide exactly one of RES* or AUTS", "AUTHENTICATION_REJECTED");
        }

        if ("5G_AKA".equals(context.getAuthType()) && auts != null && !auts.isBlank()) {
            if (!cryptographyService.verifyAuthentication(context.getAuts(), auts)) {
                context.setStatus(AuthenticationStatus.FAILED);
                return AuthenticationResponse.failure("AUTS verification failed", "AUTHENTICATION_REJECTED");
            }
            return regenerateChallenge(context, auts);
        }

        if ("EAP_AKA_PRIME".equals(context.getAuthType())) {
            if (isEapReauthenticationResponse(eapPayload)) {
                return regenerateEapAkaPrimeChallenge(context, false);
            }
            if (isEapFastReauthenticationResponse(eapPayload)) {
                return regenerateEapAkaPrimeChallenge(context, true);
            }
            if (isEapSynchronizationFailureResponse(eapPayload)) {
                return regenerateChallenge(context, extractEapAuts(eapPayload));
            }
            String candidateResStar = extractEapResStar(eapPayload);
            if (!cryptographyService.verifyAuthentication(context.getXresStar(), candidateResStar)) {
                context.setStatus(AuthenticationStatus.FAILED);
                return AuthenticationResponse.failure("EAP-AKA' verification failed", "AUTHENTICATION_REJECTED", "EAP-Failure");
            }
            context.setResStar(candidateResStar);
            context.setKseaf(cryptographyService.deriveKseaf(context.getKausf(), context.getServingNetworkName()));
            context.setStatus(AuthenticationStatus.AUTHENTICATED);
            return AuthenticationResponse.success(context.getAuthCtxId(), context.getSupi(), context.getAuthType(), context.getKseaf());
        }

        if (!cryptographyService.verifyAuthentication(context.getXresStar(), resStar)) {
            context.setStatus(AuthenticationStatus.FAILED);
            return AuthenticationResponse.failure("RES* verification failed", "AUTHENTICATION_REJECTED");
        }

        context.setResStar(resStar);
        context.setKseaf(cryptographyService.deriveKseaf(context.getKausf(), context.getServingNetworkName()));
        context.setStatus(AuthenticationStatus.AUTHENTICATED);
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

    private String extractEapResStar(String eapPayload) {
        if (eapPayload == null || !eapPayload.startsWith(EAP_AKA_PRIME_RESPONSE_PREFIX)) {
            return null;
        }
        return eapPayload.substring(EAP_AKA_PRIME_RESPONSE_PREFIX.length()).trim();
    }

    private boolean isEapReauthenticationResponse(String eapPayload) {
        return eapPayload != null && eapPayload.startsWith(EAP_AKA_PRIME_REAUTH_RESPONSE_PREFIX);
    }

    private boolean isEapFastReauthenticationResponse(String eapPayload) {
        return eapPayload != null && eapPayload.startsWith(EAP_AKA_PRIME_FAST_REAUTH_RESPONSE_PREFIX);
    }

    private boolean isEapSynchronizationFailureResponse(String eapPayload) {
        return eapPayload != null && eapPayload.startsWith(EAP_AKA_PRIME_SYNC_FAILURE_PREFIX);
    }

    private String extractEapAuts(String eapPayload) {
        if (!isEapSynchronizationFailureResponse(eapPayload)) {
            return null;
        }
        return eapPayload.substring(EAP_AKA_PRIME_SYNC_FAILURE_PREFIX.length()).trim();
    }

    private AuthenticationResponse regenerateEapAkaPrimeChallenge(AuthenticationContext context, boolean fastReauthentication) {
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

        AuthenticationVector vector = udmAuthenticationData.get().getAuthenticationVector();
        context.setRand(vector.getRand());
        context.setAutn(vector.getAutn());
        context.setAuts(vector.getAuts());
        context.setHxresStar(vector.getHxresStar());
        context.setXresStar(vector.getXresStar());
        context.setEapChallenge(vector.getEapChallenge());
        context.setKausf(vector.getKausf());
        context.setResStar(null);
        context.setKseaf(null);
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
            return AuthenticationResponse.failure(exception.getMessage(), "AUTHENTICATION_REJECTED");
        } catch (IllegalStateException exception) {
            return AuthenticationResponse.failure(exception.getMessage(), "CONTROL_PLANE_UNAVAILABLE");
        }

        if (udmAuthenticationData.isEmpty()) {
            return AuthenticationResponse.failure("subscriber not found in UDM storage", "SUBSCRIBER_NOT_FOUND");
        }

        AuthenticationVector vector = udmAuthenticationData.get().getAuthenticationVector();
        context.setRand(vector.getRand());
        context.setAutn(vector.getAutn());
        context.setAuts(vector.getAuts());
        context.setHxresStar(vector.getHxresStar());
        context.setXresStar(vector.getXresStar());
        context.setEapChallenge(vector.getEapChallenge());
        context.setKausf(vector.getKausf());
        context.setResStar(null);
        context.setKseaf(null);
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
}
