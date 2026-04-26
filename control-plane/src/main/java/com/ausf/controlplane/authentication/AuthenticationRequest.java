package com.ausf.controlplane.authentication;

public class AuthenticationRequest {
    private String supi;
    private String servingNetworkName;
    private String authType;
    private String resStar;
    private String eapPayload;

    public String getSupi() {
        return supi;
    }

    public void setSupi(String supi) {
        this.supi = supi;
    }

    public String getServingNetworkName() {
        return servingNetworkName;
    }

    public void setServingNetworkName(String servingNetworkName) {
        this.servingNetworkName = servingNetworkName;
    }

    public String getAuthType() {
        return authType;
    }

    public void setAuthType(String authType) {
        this.authType = authType;
    }

    public String getResStar() {
        return resStar;
    }

    public void setResStar(String resStar) {
        this.resStar = resStar;
    }

    public String getEapPayload() {
        return eapPayload;
    }

    public void setEapPayload(String eapPayload) {
        this.eapPayload = eapPayload;
    }
}
