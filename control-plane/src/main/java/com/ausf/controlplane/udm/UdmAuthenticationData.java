package com.ausf.controlplane.udm;

public class UdmAuthenticationData {
    private final String supi;
    private final String authType;
    private final String servingNetworkName;
    private final AuthenticationVector authenticationVector;
    private String authEventId;

    public UdmAuthenticationData(
        String supi,
        String authType,
        String servingNetworkName,
        AuthenticationVector authenticationVector
    ) {
        this.supi = supi;
        this.authType = authType;
        this.servingNetworkName = servingNetworkName;
        this.authenticationVector = authenticationVector;
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

    public AuthenticationVector getAuthenticationVector() {
        return authenticationVector;
    }

    public String getAuthEventId() {
        return authEventId;
    }

    void setAuthEventId(String authEventId) {
        this.authEventId = authEventId;
    }
}