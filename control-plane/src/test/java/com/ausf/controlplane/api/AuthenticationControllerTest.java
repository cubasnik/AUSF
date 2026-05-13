package com.ausf.controlplane.api;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static java.util.Objects.requireNonNull;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.ausf.controlplane.authentication.AuthenticationManager;
import com.ausf.controlplane.authentication.AuthenticationResponse;
import com.ausf.controlplane.eap.EapPacket;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.webmvc.test.autoconfigure.WebMvcTest;
import org.springframework.http.MediaType;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import static org.mockito.Mockito.when;

@WebMvcTest(AuthenticationController.class)
class AuthenticationControllerTest {
    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private AuthenticationManager authenticationManager;

    @Test
    void shouldReturnNotFoundWhenInitiateCannotFindSubscriber() throws Exception {
        when(authenticationManager.initiateAuthentication(any(), eq("imsi-250019999999999"), any(), any()))
            .thenReturn(AuthenticationResponse.failure("subscriber not found in UDM storage", "SUBSCRIBER_NOT_FOUND"));

        mockMvc.perform(post("/control-plane/v1/auth/initiate")
            .contentType(requireNonNull(MediaType.APPLICATION_JSON))
                .content("""
                    {
                      "supi": "imsi-250019999999999",
                      "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
                      "authType": "5G_AKA"
                    }
                    """))
            .andExpect(status().isNotFound())
            .andExpect(jsonPath("$.errorCode").value("SUBSCRIBER_NOT_FOUND"));
    }

    @Test
    void shouldReturnBadGatewayWhenInitiateCannotReachUpstreamAuthenticationSource() throws Exception {
        when(authenticationManager.initiateAuthentication(any(), eq("imsi-250010000000503"), any(), any()))
            .thenReturn(AuthenticationResponse.failure("UDM request failed: 503 Service Unavailable", "CONTROL_PLANE_UNAVAILABLE"));

        mockMvc.perform(post("/control-plane/v1/auth/initiate")
            .contentType(requireNonNull(MediaType.APPLICATION_JSON))
                .content("""
                    {
                      "supi": "imsi-250010000000503",
                      "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
                      "authType": "5G_AKA"
                    }
                    """))
            .andExpect(status().isBadGateway())
            .andExpect(jsonPath("$.errorCode").value("CONTROL_PLANE_UNAVAILABLE"));
    }

    @Test
    void shouldReturnNotFoundWhenConfirmationContextIsMissing() throws Exception {
        when(authenticationManager.verifyAuthenticationResponse(eq("missing"), any(), any(), any()))
            .thenReturn(AuthenticationResponse.failure("authentication context missing or expired", "CONTEXT_NOT_FOUND"));

        mockMvc.perform(post("/control-plane/v1/auth/missing/confirm")
            .contentType(requireNonNull(MediaType.APPLICATION_JSON))
                .content("""
                    {
                      "resStar": "deadbeef"
                    }
                    """))
            .andExpect(status().isNotFound())
            .andExpect(jsonPath("$.errorCode").value("CONTEXT_NOT_FOUND"));
    }

    @Test
    void shouldReturnUnauthorizedWhenConfirmationFails() throws Exception {
        when(authenticationManager.verifyAuthenticationResponse(eq("imsi-250010000000001"), any(), any(), any()))
            .thenReturn(AuthenticationResponse.failure("RES* verification failed", "AUTHENTICATION_REJECTED"));

        mockMvc.perform(post("/control-plane/v1/auth/imsi-250010000000001/confirm")
            .contentType(requireNonNull(MediaType.APPLICATION_JSON))
                .content("""
                    {
                      "resStar": "deadbeef"
                    }
                    """))
            .andExpect(status().isUnauthorized())
            .andExpect(jsonPath("$.errorCode").value("AUTHENTICATION_REJECTED"));
    }

    @Test
    void shouldReturnEapFailurePayloadWhenEapConfirmationFails() throws Exception {
        String eapFailurePayload = EapPacket.buildFailure((byte) 1);
        when(authenticationManager.verifyAuthenticationResponse(eq("auth-eap-1"), any(), any(), any()))
            .thenReturn(AuthenticationResponse.failure("EAP-AKA' verification failed", "AUTHENTICATION_REJECTED", eapFailurePayload));

        mockMvc.perform(post("/control-plane/v1/auth/auth-eap-1/confirm")
            .contentType(requireNonNull(MediaType.APPLICATION_JSON))
                .content("""
                    {
                      "eapPayload": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
                    }
                    """))
            .andExpect(status().isUnauthorized())
            .andExpect(jsonPath("$.errorCode").value("AUTHENTICATION_REJECTED"))
            .andExpect(jsonPath("$.eapChallenge").value(eapFailurePayload));
    }

    @Test
    void shouldReturnOkWhenConfirmationTriggersResynchronization() throws Exception {
        when(authenticationManager.verifyAuthenticationResponse(eq("auth-1"), any(), any(), any()))
            .thenReturn(AuthenticationResponse.resynchronizationChallenge(
                "auth-1",
                "imsi-250010000000001",
                "5G_AKA",
                "5G:mnc001.mcc001.3gppnetwork.org",
                "rand-2",
                "autn-2",
                "hxres-2",
                null
            ));

        mockMvc.perform(post("/control-plane/v1/auth/auth-1/confirm")
            .contentType(requireNonNull(MediaType.APPLICATION_JSON))
                .content("""
                    {
                      "auts": "auts-token"
                    }
                    """))
            .andExpect(status().isOk())
            .andExpect(jsonPath("$.message").value("re-synchronization challenge generated"))
            .andExpect(jsonPath("$.rand").value("rand-2"));
    }
}