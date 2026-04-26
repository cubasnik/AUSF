package com.ausf.controlplane.subscriber;

public class SubscriberProfile {
    private String supi;
    private String authMethod;
    private String permanentKey;
    private String opc;
    private String servingNetworkName;
    private long sequenceNumber;
    private String routingIndicator;

    public String getSupi() {
        return supi;
    }

    public void setSupi(String supi) {
        this.supi = supi;
    }

    public String getAuthMethod() {
        return authMethod;
    }

    public void setAuthMethod(String authMethod) {
        this.authMethod = authMethod;
    }

    public String getPermanentKey() {
        return permanentKey;
    }

    public void setPermanentKey(String permanentKey) {
        this.permanentKey = permanentKey;
    }

    public String getOpc() {
        return opc;
    }

    public void setOpc(String opc) {
        this.opc = opc;
    }

    public String getServingNetworkName() {
        return servingNetworkName;
    }

    public void setServingNetworkName(String servingNetworkName) {
        this.servingNetworkName = servingNetworkName;
    }

    public long getSequenceNumber() {
        return sequenceNumber;
    }

    public void setSequenceNumber(long sequenceNumber) {
        this.sequenceNumber = sequenceNumber;
    }

    public String getRoutingIndicator() {
        return routingIndicator;
    }

    public void setRoutingIndicator(String routingIndicator) {
        this.routingIndicator = routingIndicator;
    }
}
