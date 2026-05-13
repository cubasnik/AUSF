package com.ausf.controlplane.udm;

public class AuthenticationVector {
    private final String authType;
    private final String rand;
    private final String autn;
    private final String auts;
    private final String xresStar;
    private final String hxresStar;
    private final String kausf;
    private final String eapChallenge;

    public AuthenticationVector(
        String authType,
        String rand,
        String autn,
        String auts,
        String xresStar,
        String hxresStar,
        String kausf,
        String eapChallenge
    ) {
        this.authType = authType;
        this.rand = rand;
        this.autn = autn;
        this.auts = auts;
        this.xresStar = xresStar;
        this.hxresStar = hxresStar;
        this.kausf = kausf;
        this.eapChallenge = eapChallenge;
    }

    public String getAuthType() {
        return authType;
    }

    public String getRand() {
        return rand;
    }

    public String getAutn() {
        return autn;
    }

    public String getAuts() {
        return auts;
    }

    public String getXresStar() {
        return xresStar;
    }

    public String getHxresStar() {
        return hxresStar;
    }

    public String getKausf() {
        return kausf;
    }

    public String getEapChallenge() {
        return eapChallenge;
    }
}
