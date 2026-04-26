package com.ausf.controlplane.udm;

import java.util.Optional;

public interface UdmClient {
    Optional<UdmAuthenticationData> getAuthenticationData(String supi, String servingNetworkName, String authType);
}