package com.ausf.controlplane.subscriber;

public enum AkaAlgorithm {
    MILENAGE,
    TUAK;

    public static AkaAlgorithm defaultForAuthMethod(String authMethod) {
        return "EAP_AKA_PRIME".equals(authMethod) ? TUAK : MILENAGE;
    }
}