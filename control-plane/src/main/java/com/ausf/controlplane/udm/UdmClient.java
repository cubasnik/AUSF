package com.ausf.controlplane.udm;

import java.util.Optional;

public interface UdmClient {
    Optional<UdmAuthenticationData> getAuthenticationData(String supi, String servingNetworkName, String authType);

    Optional<UdmAuthenticationData> resynchronizeAuthenticationData(
        String supi,
        String servingNetworkName,
        String authType,
        String rand,
        String auts
    );

    /**
     * Notify UDM of authentication outcome (TS 29.503 §6.1.6.2).
     * Best-effort — implementations must not propagate failures.
     */
    default void confirmAuthEvent(
        String supi,
        String authEventId,
        boolean success,
        String authType,
        String servingNetworkName
    ) {}

    /**
     * Remove auth event record from UDM (TS 29.503 §6.1.6.3).
     * Best-effort — implementations must not propagate failures.
     */
    default void deleteAuthEvent(String supi, String authEventId) {}
}