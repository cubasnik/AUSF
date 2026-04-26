package com.ausf.controlplane.authentication;

public class AuthenticationContext {
    private final String supi;
    private String authType;
    private String servingNetworkName;
    private String rand;
    private String autn;
    private String hxresStar;
    private String xresStar;
    private String eapChallenge;
    private String resStar;
    private String kseaf;
    private String status;
    private final long createdAt;
    private final long expiresAt;

    public AuthenticationContext(String supi) {
        this.supi = supi;
        this.createdAt = System.currentTimeMillis();
        this.expiresAt = createdAt + 300000;
        this.status = "CREATED";
    }

    public String getSupi() {
        return supi;
    }

    public String getAuthType() {
        return authType;
    }

    public void setAuthType(String authType) {
        this.authType = authType;
    }

    public String getServingNetworkName() {
        return servingNetworkName;
    }

    public void setServingNetworkName(String servingNetworkName) {
        this.servingNetworkName = servingNetworkName;
    }

    public String getRand() {
        return rand;
    }

    public void setRand(String rand) {
        this.rand = rand;
    }

    public String getAutn() {
        return autn;
    }

    public void setAutn(String autn) {
        this.autn = autn;
    }

    public String getHxresStar() {
        return hxresStar;
    }

    public void setHxresStar(String hxresStar) {
        this.hxresStar = hxresStar;
    }

    public String getXresStar() {
        return xresStar;
    }

    public void setXresStar(String xresStar) {
        this.xresStar = xresStar;
    }

    public String getEapChallenge() {
        return eapChallenge;
    }

    public void setEapChallenge(String eapChallenge) {
        this.eapChallenge = eapChallenge;
    }

    public String getResStar() {
        return resStar;
    }

    public void setResStar(String resStar) {
        this.resStar = resStar;
    }

    public String getKseaf() {
        return kseaf;
    }

    public void setKseaf(String kseaf) {
        this.kseaf = kseaf;
    }

    public String getStatus() {
        return status;
    }

    public void setStatus(String status) {
        this.status = status;
    }

    public long getCreatedAt() {
        return createdAt;
    }

    public long getExpiresAt() {
        return expiresAt;
    }

    public boolean isExpired() {
        return System.currentTimeMillis() > expiresAt;
    }
}
