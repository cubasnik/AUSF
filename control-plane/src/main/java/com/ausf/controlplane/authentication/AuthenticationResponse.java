package com.ausf.controlplane.authentication;

public class AuthenticationResponse {
    private final boolean success;
    private final String supi;
    private final String authType;
    private final String servingNetworkName;
    private final String rand;
    private final String autn;
    private final String hxresStar;
    private final String eapChallenge;
    private final String kseaf;
    private final String message;

    private AuthenticationResponse(
        boolean success,
        String supi,
        String authType,
        String servingNetworkName,
        String rand,
        String autn,
        String hxresStar,
        String eapChallenge,
        String kseaf,
        String message
    ) {
        this.success = success;
        this.supi = supi;
        this.authType = authType;
        this.servingNetworkName = servingNetworkName;
        this.rand = rand;
        this.autn = autn;
        this.hxresStar = hxresStar;
        this.eapChallenge = eapChallenge;
        this.kseaf = kseaf;
        this.message = message;
    }

    public static AuthenticationResponse challenge(
        String supi,
        String authType,
        String servingNetworkName,
        String rand,
        String autn,
        String hxresStar,
        String eapChallenge
    ) {
        return new AuthenticationResponse(true, supi, authType, servingNetworkName, rand, autn, hxresStar, eapChallenge, null, "challenge generated");
    }

    public static AuthenticationResponse success(String supi, String authType, String kseaf) {
        return new AuthenticationResponse(true, supi, authType, null, null, null, null, null, kseaf, "authentication successful");
    }

    public static AuthenticationResponse failure(String message) {
        return new AuthenticationResponse(false, null, null, null, null, null, null, null, null, message);
    }

    public boolean getSuccess() {
        return success;
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
}
