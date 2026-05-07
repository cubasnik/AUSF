package com.ausf.controlplane.authentication;

public class AuthenticationResponse {
    private final boolean success;
    private final String authCtxId;
    private final String supi;
    private final String authType;
    private final String servingNetworkName;
    private final String rand;
    private final String autn;
    private final String hxresStar;
    private final String eapChallenge;
    private final String kseaf;
    private final String message;
    private final String errorCode;

    private AuthenticationResponse(
        boolean success,
        String authCtxId,
        String supi,
        String authType,
        String servingNetworkName,
        String rand,
        String autn,
        String hxresStar,
        String eapChallenge,
        String kseaf,
        String message,
        String errorCode
    ) {
        this.success = success;
        this.authCtxId = authCtxId;
        this.supi = supi;
        this.authType = authType;
        this.servingNetworkName = servingNetworkName;
        this.rand = rand;
        this.autn = autn;
        this.hxresStar = hxresStar;
        this.eapChallenge = eapChallenge;
        this.kseaf = kseaf;
        this.message = message;
        this.errorCode = errorCode;
    }

    public static AuthenticationResponse challenge(
        String authCtxId,
        String supi,
        String authType,
        String servingNetworkName,
        String rand,
        String autn,
        String hxresStar,
        String eapChallenge
    ) {
        return new AuthenticationResponse(true, authCtxId, supi, authType, servingNetworkName, rand, autn, hxresStar, eapChallenge, null, "challenge generated", null);
    }

    public static AuthenticationResponse resynchronizationChallenge(
        String authCtxId,
        String supi,
        String authType,
        String servingNetworkName,
        String rand,
        String autn,
        String hxresStar,
        String eapChallenge
    ) {
        return new AuthenticationResponse(
            true,
            authCtxId,
            supi,
            authType,
            servingNetworkName,
            rand,
            autn,
            hxresStar,
            eapChallenge,
            null,
            "re-synchronization challenge generated",
            null
        );
    }

    public static AuthenticationResponse reauthenticationChallenge(
        String authCtxId,
        String supi,
        String authType,
        String servingNetworkName,
        String rand,
        String autn,
        String hxresStar,
        String eapChallenge
    ) {
        return new AuthenticationResponse(
            true,
            authCtxId,
            supi,
            authType,
            servingNetworkName,
            rand,
            autn,
            hxresStar,
            eapChallenge,
            null,
            "EAP-AKA' re-authentication challenge generated",
            null
        );
    }

    public static AuthenticationResponse fastReauthenticationChallenge(
        String authCtxId,
        String supi,
        String authType,
        String servingNetworkName,
        String rand,
        String autn,
        String hxresStar,
        String eapChallenge
    ) {
        return new AuthenticationResponse(
            true,
            authCtxId,
            supi,
            authType,
            servingNetworkName,
            rand,
            autn,
            hxresStar,
            eapChallenge,
            null,
            "EAP-AKA' fast re-authentication challenge generated",
            null
        );
    }

    public static AuthenticationResponse synchronizationFailureChallenge(
        String authCtxId,
        String supi,
        String authType,
        String servingNetworkName,
        String rand,
        String autn,
        String hxresStar,
        String eapChallenge
    ) {
        return new AuthenticationResponse(
            true,
            authCtxId,
            supi,
            authType,
            servingNetworkName,
            rand,
            autn,
            hxresStar,
            eapChallenge,
            null,
            "EAP-AKA' synchronization-failure challenge generated",
            null
        );
    }

    public static AuthenticationResponse success(String authCtxId, String supi, String authType, String kseaf) {
        return new AuthenticationResponse(true, authCtxId, supi, authType, null, null, null, null, null, kseaf, "authentication successful", null);
    }

    public static AuthenticationResponse failure(String message, String errorCode) {
        return failure(message, errorCode, null);
    }

    public static AuthenticationResponse failure(String message, String errorCode, String eapChallenge) {
        return new AuthenticationResponse(false, null, null, null, null, null, null, null, eapChallenge, null, message, errorCode);
    }

    public boolean getSuccess() {
        return success;
    }

    public String getAuthCtxId() {
        return authCtxId;
    }

    public String getSupi() {
        return supi;
    }

    public String getAuthType() {
        return authType;
    }

    public String getServingNetworkName() {
        return servingNetworkName;
    }

    public String getRand() {
        return rand;
    }

    public String getAutn() {
        return autn;
    }

    public String getHxresStar() {
        return hxresStar;
    }

    public String getEapChallenge() {
        return eapChallenge;
    }

    public String getKseaf() {
        return kseaf;
    }

    public String getMessage() {
        return message;
    }

    public String getErrorCode() {
        return errorCode;
    }
}
