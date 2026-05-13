package com.ausf.controlplane.subscriber;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.Table;

@Entity
@Table(name = "subscribers")

public class SubscriberProfile {

    @Id
    @Column(nullable = false, unique = true)
    private String supi;

    @Column(nullable = false)
    private String authMethod;

    @Enumerated(EnumType.STRING)
    private AkaAlgorithm akaAlgorithm;

    @Column(nullable = false)
    private String permanentKey;

    @Column(nullable = false)
    private String opc;

    @Column(nullable = false)
    private String servingNetworkName;

    @Column(nullable = false)
    private long sequenceNumber;

    @Column(nullable = false)
    private String routingIndicator;

    public SubscriberProfile() {
        // JPA requires a no-arg constructor
    }

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

    public AkaAlgorithm getAkaAlgorithm() {
        return akaAlgorithm;
    }

    public void setAkaAlgorithm(AkaAlgorithm akaAlgorithm) {
        this.akaAlgorithm = akaAlgorithm;
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
