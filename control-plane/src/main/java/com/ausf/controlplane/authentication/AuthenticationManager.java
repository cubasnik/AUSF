package com.ausf.controlplane.authentication;

import com.ausf.controlplane.udm.AuthenticationVector;
import com.ausf.controlplane.udm.UdmAuthenticationData;
import com.ausf.controlplane.udm.UdmClient;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;
import org.springframework.stereotype.Service;

@Service
public class AuthenticationManager {
    private final Map<String, AuthenticationContext> authContexts = new ConcurrentHashMap<>();
    private final CryptographyService cryptographyService;
    private final UdmClient udmClient;

    public AuthenticationManager(CryptographyService cryptographyService, UdmClient udmClient) {
        this.cryptographyService = cryptographyService;
        this.udmClient = udmClient;
    }

    public AuthenticationResponse initiateAuthentication(String supi, String servingNetworkName, String authType) {
        Optional<UdmAuthenticationData> udmAuthenticationData = udmClient.getAuthenticationData(supi, servingNetworkName, authType);
        if (udmAuthenticationData.isEmpty()) {
            return AuthenticationResponse.failure("subscriber not found in UDM storage", "SUBSCRIBER_NOT_FOUND");
        }

        UdmAuthenticationData authenticationData = udmAuthenticationData.get();
        AuthenticationVector vector = authenticationData.getAuthenticationVector();
        AuthenticationContext context = new AuthenticationContext(supi);
        context.setAuthType(authenticationData.getAuthType());
        context.setServingNetworkName(authenticationData.getServingNetworkName());
        context.setRand(vector.getRand());
        context.setAutn(vector.getAutn());
        context.setHxresStar(vector.getHxresStar());
        context.setXresStar(vector.getXresStar());
        context.setEapChallenge(vector.getEapChallenge());
        context.setStatus("CHALLENGE_SENT");
        authContexts.put(supi, context);

        return AuthenticationResponse.challenge(
            supi,
            context.getAuthType(),
            context.getServingNetworkName(),
            context.getRand(),
            context.getAutn(),
            context.getHxresStar(),
            "EAP_AKA_PRIME".equals(context.getAuthType()) ? context.getEapChallenge() : null
        );
    }

    public AuthenticationResponse verifyAuthenticationResponse(String supi, String resStar, String eapPayload) {
        AuthenticationContext context = authContexts.get(supi);
        if (context == null || context.isExpired()) {
            return AuthenticationResponse.failure("authentication context missing or expired", "CONTEXT_NOT_FOUND");
        }

        if ("EAP_AKA_PRIME".equals(context.getAuthType())) {
            String expectedPayload = context.getEapChallenge() == null ? null : context.getEapChallenge().replace("Request", "Response");
            if (!cryptographyService.verifyAuthentication(expectedPayload, eapPayload)) {
                context.setStatus("FAILED");
                return AuthenticationResponse.failure("EAP-AKA' verification failed", "AUTHENTICATION_REJECTED");
            }
            context.setResStar(eapPayload);
            context.setKseaf(cryptographyService.digestHex(context.getSupi() + context.getServingNetworkName() + "KSEAF"));
            context.setStatus("AUTHENTICATED");
            return AuthenticationResponse.success(supi, context.getAuthType(), context.getKseaf());
        }

        if (!cryptographyService.verifyAuthentication(context.getXresStar(), resStar)) {
            context.setStatus("FAILED");
            return AuthenticationResponse.failure("RES* verification failed", "AUTHENTICATION_REJECTED");
        }

        context.setResStar(resStar);
        context.setKseaf(cryptographyService.deriveKey(context.getRand(), "KSEAF"));
        context.setStatus("AUTHENTICATED");
        return AuthenticationResponse.success(supi, context.getAuthType(), context.getKseaf());
    }

    public AuthenticationContext getContext(String supi) {
        return authContexts.get(supi);
    }
}
