package com.ausf.controlplane.authentication;

public class AuthenticationContext {
    private final String authCtxId;
    private final String supi;
    private String authType;
    private String servingNetworkName;
    private String rand;
    private String autn;
    private String auts;
    private String hxresStar;
    private String xresStar;
    private String eapChallenge;
    private String resStar;
    private String kseaf;
    private String kausf;
    private AuthenticationStatus status;
    private final long createdAt;
    private final long expiresAt;

    public AuthenticationContext(String authCtxId, String supi) {
        this.authCtxId = authCtxId;
        this.supi = supi;
        this.createdAt = System.currentTimeMillis();
        this.expiresAt = createdAt + 300000;
        this.status = AuthenticationStatus.CREATED;
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

    public String getAuts() {
        return auts;
    }

    public void setAuts(String auts) {
        this.auts = auts;
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

    public String getKausf() {
        return kausf;
    }

    public void setKausf(String kausf) {
        this.kausf = kausf;
    }

    public AuthenticationStatus getStatus() {
        return status;
    }

    public void setStatus(AuthenticationStatus status) {
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
