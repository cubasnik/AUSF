package com.ausf.controlplane.authentication;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.ausf.controlplane.subscriber.SubscriberProfile;
import com.ausf.controlplane.subscriber.SubscriberRepository;
import com.ausf.controlplane.udm.UdmService;
import java.util.Optional;
import org.junit.jupiter.api.Test;

class AuthenticationManagerTest {
    private final CryptographyService cryptographyService = new CryptographyService();
    private final SubscriberRepository subscriberRepository = supi -> {
        if ("imsi-250010000000001".equals(supi)) {
            SubscriberProfile profile = new SubscriberProfile();
            profile.setSupi(supi);
            profile.setAuthMethod("5G_AKA");
            profile.setPermanentKey("465b5ce8b199b49faa5f0a2ee238a6bc");
            profile.setOpc("e8ed289deba952e4283b54e88e6183ca");
            profile.setServingNetworkName("5G:mnc001.mcc001.3gppnetwork.org");
            profile.setSequenceNumber(16);
            profile.setRoutingIndicator("0001");
            return Optional.of(profile);
        }
        if ("imsi-250010000000002".equals(supi)) {
            SubscriberProfile profile = new SubscriberProfile();
            profile.setSupi(supi);
            profile.setAuthMethod("EAP_AKA_PRIME");
            profile.setPermanentKey("9f47cd1b2ac34cfeb61c7c8f4bb5fda1");
            profile.setOpc("ab12cd34ef56ab78cd90ef12ab34cd56");
            profile.setServingNetworkName("5G:mnc001.mcc001.3gppnetwork.org");
            profile.setSequenceNumber(32);
            profile.setRoutingIndicator("0002");
            return Optional.of(profile);
        }
        return Optional.empty();
    };
    private final UdmService udmService = new UdmService(subscriberRepository, cryptographyService);
    private final AuthenticationManager authenticationManager = new AuthenticationManager(cryptographyService, udmService);

    @Test
    void shouldAuthenticateKnownContext() {
        authenticationManager.initiateAuthentication(
            "imsi-250010000000001",
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA"
        );
        String resStar = authenticationManager.getContext("imsi-250010000000001").getXresStar();

        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse("imsi-250010000000001", resStar, null);

        assertTrue(result.getSuccess());
        assertNotNull(result.getKseaf());
        assertEquals("AUTHENTICATED", authenticationManager.getContext("imsi-250010000000001").getStatus());
    }

    @Test
    void shouldAuthenticateEapAkaPrimeContext() {
        AuthenticationResponse challenge = authenticationManager.initiateAuthentication(
            "imsi-250010000000002",
            "5G:mnc001.mcc001.3gppnetwork.org",
            "EAP_AKA_PRIME"
        );

        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse(
            "imsi-250010000000002",
            null,
            challenge.getEapChallenge().replace("Request", "Response")
        );

        assertTrue(result.getSuccess());
        assertEquals("EAP_AKA_PRIME", result.getAuthType());
    }

    @Test
    void shouldRejectUnknownContext() {
        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse("missing", "deadbeef", null);

        assertFalse(result.getSuccess());
    }

    @Test
    void shouldHonorServingNetworkNameOverrideWhenInitiatingAuthentication() {
        AuthenticationResponse challenge = authenticationManager.initiateAuthentication(
            "imsi-250010000000001",
            "5G:mnc999.mcc999.3gppnetwork.org",
            "5G_AKA"
        );

        assertEquals("5G:mnc999.mcc999.3gppnetwork.org", challenge.getServingNetworkName());
    }
}
