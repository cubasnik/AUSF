package com.ausf.controlplane.api;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.ausf.controlplane.authentication.AuthenticationManager;
import com.ausf.controlplane.authentication.AuthenticationResponse;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.mock.mockito.MockBean;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;

import static org.mockito.Mockito.when;

@WebMvcTest(AuthenticationController.class)
class AuthenticationControllerTest {
    @Autowired
    private MockMvc mockMvc;

    @MockBean
    private AuthenticationManager authenticationManager;

    @Test
    void shouldReturnNotFoundWhenConfirmationContextIsMissing() throws Exception {
        when(authenticationManager.verifyAuthenticationResponse(eq("missing"), any(), any()))
            .thenReturn(AuthenticationResponse.failure("authentication context missing or expired", "CONTEXT_NOT_FOUND"));

        mockMvc.perform(post("/control-plane/v1/auth/missing/confirm")
                .contentType(MediaType.APPLICATION_JSON)
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
        when(authenticationManager.verifyAuthenticationResponse(eq("imsi-250010000000001"), any(), any()))
            .thenReturn(AuthenticationResponse.failure("RES* verification failed", "AUTHENTICATION_REJECTED"));

        mockMvc.perform(post("/control-plane/v1/auth/imsi-250010000000001/confirm")
                .contentType(MediaType.APPLICATION_JSON)
                .content("""
                    {
                      "resStar": "deadbeef"
                    }
                    """))
            .andExpect(status().isUnauthorized())
            .andExpect(jsonPath("$.errorCode").value("AUTHENTICATION_REJECTED"));
    }
}