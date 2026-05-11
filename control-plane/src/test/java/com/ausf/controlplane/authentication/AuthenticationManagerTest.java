package com.ausf.controlplane.authentication;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.ausf.controlplane.crypto.Milenage;
import com.ausf.controlplane.eap.EapPacket;
import com.ausf.controlplane.subscriber.AkaAlgorithm;
import com.ausf.controlplane.subscriber.SubscriberProfile;
import com.ausf.controlplane.subscriber.SubscriberRepository;
import com.ausf.controlplane.udm.UdmService;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;
import org.junit.jupiter.api.Test;

class AuthenticationManagerTest {
    private final CryptographyService cryptographyService = new CryptographyService();
    private final InMemorySubscriberRepository subscriberRepository = new InMemorySubscriberRepository();
    private final UdmService udmService = new UdmService(subscriberRepository, cryptographyService);
    private final AuthenticationManager authenticationManager = new AuthenticationManager(cryptographyService, udmService);

    private static final String FIVE_G_AKA_SUPI = "imsi-250010000000001";
    private static final String FIVE_G_AKA_KEY = "465b5ce8b199b49faa5f0a2ee238a6bc";
    private static final String FIVE_G_AKA_OPC = "e8ed289deba952e4283b54e88e6183ca";

    @Test
    void shouldAuthenticateKnownContext() {
        AuthenticationResponse challenge = authenticationManager.initiateAuthentication(
            "auth-1",
            FIVE_G_AKA_SUPI,
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA"
        );
        String resStar = authenticationManager.getContext(FIVE_G_AKA_SUPI).getXresStar();

        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse(challenge.getAuthCtxId(), resStar, null, null);

        assertTrue(result.getSuccess());
        assertNotNull(result.getKseaf());
        assertEquals(AuthenticationStatus.AUTHENTICATED, authenticationManager.getContext(FIVE_G_AKA_SUPI).getStatus());
    }

    @Test
    void shouldRejectEarlierChallengeWhenSameSupiInitiatesAgain() {
        AuthenticationResponse firstChallenge = authenticationManager.initiateAuthentication(
            "auth-1",
            FIVE_G_AKA_SUPI,
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA"
        );
        String firstResStar = authenticationManager.getContext(FIVE_G_AKA_SUPI).getXresStar();

        authenticationManager.initiateAuthentication(
            "auth-2",
            FIVE_G_AKA_SUPI,
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA"
        );

        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse(
            firstChallenge.getAuthCtxId(),
            firstResStar,
            null,
            null
        );

        assertFalse(result.getSuccess());
        assertEquals("AUTHENTICATION_REJECTED", result.getErrorCode());
    }

    @Test
    void shouldRejectRepeatedConfirmationAfterSuccessfulAuthentication() {
        AuthenticationResponse challenge = authenticationManager.initiateAuthentication(
            "auth-1",
            FIVE_G_AKA_SUPI,
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA"
        );
        String resStar = authenticationManager.getContext(FIVE_G_AKA_SUPI).getXresStar();

        AuthenticationResponse firstResult = authenticationManager.verifyAuthenticationResponse(
            challenge.getAuthCtxId(),
            resStar,
            null,
            null
        );
        AuthenticationResponse secondResult = authenticationManager.verifyAuthenticationResponse(
            challenge.getAuthCtxId(),
            resStar,
            null,
            null
        );

        assertTrue(firstResult.getSuccess());
        assertFalse(secondResult.getSuccess());
        assertEquals("AUTHENTICATION_REJECTED", secondResult.getErrorCode());
    }

    @Test
    void shouldAuthenticateEapAkaPrimeContext() {
        AuthenticationResponse challenge = authenticationManager.initiateAuthentication(
            "auth-1",
            "imsi-250010000000002",
            "5G:mnc001.mcc001.3gppnetwork.org",
            "EAP_AKA_PRIME"
        );
        AuthenticationContext ctx = authenticationManager.getContext("imsi-250010000000002");
        byte[] kAutPrime = cryptographyService.deriveKautPrime(ctx.getKausf());
        byte id = EapPacket.extractIdentifier(challenge.getEapChallenge());
        String eapResponse = EapPacket.buildChallengeResponse(id, Milenage.hexToBytes(ctx.getXresStar()), kAutPrime);

        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse(
            challenge.getAuthCtxId(),
            null,
            null,
            eapResponse
        );

        assertTrue(result.getSuccess());
        assertEquals("EAP_AKA_PRIME", result.getAuthType());
    }

    @Test
    void shouldRejectEchoedEapChallengeWithoutResStarResponse() {
        AuthenticationResponse challenge = authenticationManager.initiateAuthentication(
            "auth-1",
            "imsi-250010000000002",
            "5G:mnc001.mcc001.3gppnetwork.org",
            "EAP_AKA_PRIME"
        );
        AuthenticationContext ctx = authenticationManager.getContext("imsi-250010000000002");
        byte[] kAutPrime = cryptographyService.deriveKautPrime(ctx.getKausf());
        byte id = EapPacket.extractIdentifier(challenge.getEapChallenge());
        // All-zero RES* is invalid — will fail verifyAuthentication check
        String eapResponse = EapPacket.buildChallengeResponse(id, new byte[16], kAutPrime);

        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse(
            challenge.getAuthCtxId(),
            null,
            null,
            eapResponse
        );

        assertFalse(result.getSuccess());
        assertEquals("AUTHENTICATION_REJECTED", result.getErrorCode());
    }

    @Test
    void shouldReissueEapChallengeWhenReauthenticationResponseIsProvided() {
        AuthenticationResponse challenge = authenticationManager.initiateAuthentication(
            "auth-1",
            "imsi-250010000000002",
            "5G:mnc001.mcc001.3gppnetwork.org",
            "EAP_AKA_PRIME"
        );
        AuthenticationContext ctx = authenticationManager.getContext("imsi-250010000000002");
        byte[] kAutPrime = cryptographyService.deriveKautPrime(ctx.getKausf());
        byte id = EapPacket.extractIdentifier(challenge.getEapChallenge());
        String reauthResponse = EapPacket.buildReauthenticationResponse(id, kAutPrime, false);

        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse(
            challenge.getAuthCtxId(),
            null,
            null,
            reauthResponse
        );

        assertTrue(result.getSuccess());
        assertEquals("EAP-AKA' re-authentication challenge generated", result.getMessage());
        assertNotNull(result.getEapChallenge());
        assertNotEquals(challenge.getEapChallenge(), result.getEapChallenge());
        assertEquals(AuthenticationStatus.CHALLENGE_SENT, authenticationManager.getContext("imsi-250010000000002").getStatus());
    }

    @Test
    void shouldRejectStaleEapResponseAfterReauthenticationRefresh() {
        AuthenticationResponse challenge = authenticationManager.initiateAuthentication(
            "auth-1",
            "imsi-250010000000002",
            "5G:mnc001.mcc001.3gppnetwork.org",
            "EAP_AKA_PRIME"
        );
        AuthenticationContext ctx = authenticationManager.getContext("imsi-250010000000002");
        String staleXresStar = ctx.getXresStar();
        byte[] staleKAutPrime = cryptographyService.deriveKautPrime(ctx.getKausf());
        byte id = EapPacket.extractIdentifier(challenge.getEapChallenge());
        String reauthResponse = EapPacket.buildReauthenticationResponse(id, staleKAutPrime, false);

        AuthenticationResponse refreshedChallenge = authenticationManager.verifyAuthenticationResponse(
            challenge.getAuthCtxId(),
            null,
            null,
            reauthResponse
        );
        // Stale response: built with old credentials, wrong MAC and wrong RES* for new challenge
        String staleResponse = EapPacket.buildChallengeResponse(id, Milenage.hexToBytes(staleXresStar), staleKAutPrime);
        AuthenticationResponse staleResult = authenticationManager.verifyAuthenticationResponse(
            challenge.getAuthCtxId(),
            null,
            null,
            staleResponse
        );

        assertTrue(refreshedChallenge.getSuccess());
        assertFalse(staleResult.getSuccess());
        assertEquals("AUTHENTICATION_REJECTED", staleResult.getErrorCode());
        assertNotNull(staleResult.getEapChallenge()); // binary EAP-Failure
        assertEquals(AuthenticationStatus.FAILED, authenticationManager.getContext("imsi-250010000000002").getStatus());
    }

    @Test
    void shouldReissueEapChallengeWhenFastReauthenticationResponseIsProvided() {
        AuthenticationResponse challenge = authenticationManager.initiateAuthentication(
            "auth-1",
            "imsi-250010000000002",
            "5G:mnc001.mcc001.3gppnetwork.org",
            "EAP_AKA_PRIME"
        );
        AuthenticationContext ctx = authenticationManager.getContext("imsi-250010000000002");
        byte[] kAutPrime = cryptographyService.deriveKautPrime(ctx.getKausf());
        byte id = EapPacket.extractIdentifier(challenge.getEapChallenge());
        String fastReauthResponse = EapPacket.buildReauthenticationResponse(id, kAutPrime, true);

        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse(
            challenge.getAuthCtxId(),
            null,
            null,
            fastReauthResponse
        );

        assertTrue(result.getSuccess());
        assertEquals("EAP-AKA' fast re-authentication challenge generated", result.getMessage());
        assertNotNull(result.getEapChallenge());
        assertNotEquals(challenge.getEapChallenge(), result.getEapChallenge());
        assertEquals(AuthenticationStatus.CHALLENGE_SENT, authenticationManager.getContext("imsi-250010000000002").getStatus());
    }

    @Test
    void shouldReissueEapChallengeWhenSynchronizationFailureResponseIsProvided() {
        AuthenticationResponse challenge = authenticationManager.initiateAuthentication(
            "auth-1",
            "imsi-250010000000003",
            "5G:mnc001.mcc001.3gppnetwork.org",
            "EAP_AKA_PRIME"
        );
        AuthenticationContext ctx = authenticationManager.getContext("imsi-250010000000003");
        byte[] kAutPrime = cryptographyService.deriveKautPrime(ctx.getKausf());
        byte id = EapPacket.extractIdentifier(challenge.getEapChallenge());
        String autsHex = cryptographyService.generateMilenageAuts(
            challenge.getRand(),
            FIVE_G_AKA_KEY,
            FIVE_G_AKA_OPC,
            48
        );
        String syncFailResponse = EapPacket.buildSyncFailureResponse(id, Milenage.hexToBytes(autsHex), kAutPrime);

        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse(
            challenge.getAuthCtxId(),
            null,
            null,
            syncFailResponse
        );

        assertTrue(result.getSuccess());
        assertEquals("EAP-AKA' synchronization-failure challenge generated", result.getMessage());
        assertNotNull(result.getEapChallenge());
        assertNotEquals(challenge.getEapChallenge(), result.getEapChallenge());
        assertEquals(AuthenticationStatus.CHALLENGE_SENT, authenticationManager.getContext("imsi-250010000000003").getStatus());
    }

    @Test
    void shouldRejectUnknownContext() {
        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse("missing", "deadbeef", null, null);

        assertFalse(result.getSuccess());
        assertEquals("CONTEXT_NOT_FOUND", result.getErrorCode());
    }

    @Test
    void shouldRejectUnknownSubscriberDuringInitiate() {
        AuthenticationResponse result = authenticationManager.initiateAuthentication(
            "auth-missing",
            "imsi-250019999999999",
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA"
        );

        assertFalse(result.getSuccess());
        assertEquals("SUBSCRIBER_NOT_FOUND", result.getErrorCode());
    }

    @Test
    void shouldMapUdmFailureToControlPlaneUnavailableDuringInitiate() {
        AuthenticationManager manager = new AuthenticationManager(
            cryptographyService,
            new com.ausf.controlplane.udm.UdmClient() {
                @Override
                public Optional<com.ausf.controlplane.udm.UdmAuthenticationData> getAuthenticationData(
                    String supi,
                    String servingNetworkName,
                    String authType
                ) {
                    throw new IllegalStateException("UDM request failed: 503 Service Unavailable");
                }

                @Override
                public Optional<com.ausf.controlplane.udm.UdmAuthenticationData> resynchronizeAuthenticationData(
                    String supi,
                    String servingNetworkName,
                    String authType,
                    String rand,
                    String auts
                ) {
                    throw new IllegalStateException("UDM request failed: 503 Service Unavailable");
                }
            }
        );

        AuthenticationResponse result = manager.initiateAuthentication(
            "auth-upstream-unavailable",
            "imsi-250010000000503",
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA"
        );

        assertFalse(result.getSuccess());
        assertEquals("CONTROL_PLANE_UNAVAILABLE", result.getErrorCode());
        assertEquals("UDM request failed: 503 Service Unavailable", result.getMessage());
    }

    @Test
    void shouldRejectInvalidResStar() {
        AuthenticationResponse challenge = authenticationManager.initiateAuthentication(
            "auth-1",
            "imsi-250010000000001",
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA"
        );

        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse(
            challenge.getAuthCtxId(),
            "deadbeef",
            null,
            null
        );

        assertFalse(result.getSuccess());
        assertEquals("AUTHENTICATION_REJECTED", result.getErrorCode());
    }

    @Test
    void shouldHonorServingNetworkNameOverrideWhenInitiatingAuthentication() {
        AuthenticationResponse challenge = authenticationManager.initiateAuthentication(
            "auth-1",
            FIVE_G_AKA_SUPI,
            "5G:mnc999.mcc999.3gppnetwork.org",
            "5G_AKA"
        );

        assertEquals("5G:mnc999.mcc999.3gppnetwork.org", challenge.getServingNetworkName());
    }

    @Test
    void shouldRegenerateFiveGAkaChallengeWhenAutsIsProvided() {
        AuthenticationResponse firstChallenge = authenticationManager.initiateAuthentication(
            "auth-1",
            FIVE_G_AKA_SUPI,
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA"
        );
        String auts = cryptographyService.generateMilenageAuts(
            firstChallenge.getRand(),
            FIVE_G_AKA_KEY,
            FIVE_G_AKA_OPC,
            16
        );

        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse(
            firstChallenge.getAuthCtxId(),
            null,
            auts,
            null
        );

        assertTrue(result.getSuccess());
        assertEquals("re-synchronization challenge generated", result.getMessage());
        assertNotNull(result.getRand());
        assertNotNull(result.getAutn());
        assertNotNull(result.getHxresStar());
        assertNotEquals(firstChallenge.getAutn(), result.getAutn());
        assertEquals(AuthenticationStatus.CHALLENGE_SENT, authenticationManager.getContext(FIVE_G_AKA_SUPI).getStatus());
        assertEquals(18, subscriberRepository.findBySupi(FIVE_G_AKA_SUPI).orElseThrow().getSequenceNumber());
    }

    @Test
    void shouldRejectAmbiguousFiveGAkaConfirmationWhenAutsAndResStarAreBothProvided() {
        AuthenticationResponse firstChallenge = authenticationManager.initiateAuthentication(
            "auth-1",
            FIVE_G_AKA_SUPI,
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA"
        );
        String auts = cryptographyService.generateMilenageAuts(
            firstChallenge.getRand(),
            FIVE_G_AKA_KEY,
            FIVE_G_AKA_OPC,
            16
        );
        String resStar = authenticationManager.getContext(FIVE_G_AKA_SUPI).getXresStar();

        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse(
            firstChallenge.getAuthCtxId(),
            resStar,
            auts,
            null
        );

        assertFalse(result.getSuccess());
        assertEquals("AUTHENTICATION_REJECTED", result.getErrorCode());
        assertEquals(AuthenticationStatus.FAILED, authenticationManager.getContext(FIVE_G_AKA_SUPI).getStatus());
    }

    @Test
    void shouldRejectInvalidAutsWhenResynchronizationFails() {
        AuthenticationResponse firstChallenge = authenticationManager.initiateAuthentication(
            "auth-1",
            FIVE_G_AKA_SUPI,
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA"
        );

        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse(
            firstChallenge.getAuthCtxId(),
            null,
            "deadbeef",
            null
        );

        assertFalse(result.getSuccess());
        assertEquals("AUTHENTICATION_REJECTED", result.getErrorCode());
        assertEquals(AuthenticationStatus.FAILED, authenticationManager.getContext(FIVE_G_AKA_SUPI).getStatus());
    }

    @Test
    void shouldRejectOriginalResStarAfterResynchronizationChallengeIsIssued() {
        AuthenticationResponse firstChallenge = authenticationManager.initiateAuthentication(
            "auth-1",
            FIVE_G_AKA_SUPI,
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA"
        );
        String firstResStar = authenticationManager.getContext(FIVE_G_AKA_SUPI).getXresStar();
        String auts = cryptographyService.generateMilenageAuts(
            firstChallenge.getRand(),
            FIVE_G_AKA_KEY,
            FIVE_G_AKA_OPC,
            16
        );

        AuthenticationResponse resyncChallenge = authenticationManager.verifyAuthenticationResponse(
            firstChallenge.getAuthCtxId(),
            null,
            auts,
            null
        );
        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse(
            resyncChallenge.getAuthCtxId(),
            firstResStar,
            null,
            null
        );

        assertTrue(resyncChallenge.getSuccess());
        assertFalse(result.getSuccess());
        assertEquals("AUTHENTICATION_REJECTED", result.getErrorCode());
    }

    @Test
    void shouldRejectOriginalAutsAfterResynchronizationChallengeRefreshesRand() {
        AuthenticationResponse firstChallenge = authenticationManager.initiateAuthentication(
            "auth-1",
            FIVE_G_AKA_SUPI,
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA"
        );
        String auts = cryptographyService.generateMilenageAuts(
            firstChallenge.getRand(),
            FIVE_G_AKA_KEY,
            FIVE_G_AKA_OPC,
            16
        );

        AuthenticationResponse resyncChallenge = authenticationManager.verifyAuthenticationResponse(
            firstChallenge.getAuthCtxId(),
            null,
            auts,
            null
        );
        AuthenticationResponse result = authenticationManager.verifyAuthenticationResponse(
            resyncChallenge.getAuthCtxId(),
            null,
            auts,
            null
        );

        assertTrue(resyncChallenge.getSuccess());
        assertFalse(result.getSuccess());
        assertEquals("AUTHENTICATION_REJECTED", result.getErrorCode());
        assertEquals(AuthenticationStatus.FAILED, authenticationManager.getContext(FIVE_G_AKA_SUPI).getStatus());
    }

    @Test
    void shouldRejectFiveGAkaSyncFailureWhenMaxAttemptsExceeded() {
        AuthenticationManager limitedManager = new AuthenticationManager(cryptographyService, udmService);
        limitedManager.maxSyncFailures = 1;

        AuthenticationResponse firstChallenge = limitedManager.initiateAuthentication(
            "auth-1", FIVE_G_AKA_SUPI, "5G:mnc001.mcc001.3gppnetwork.org", "5G_AKA"
        );
        // Use the server-precomputed AUTS stored in the context
        String auts1 = limitedManager.getContext(FIVE_G_AKA_SUPI).getAuts();

        // First SYNC_FAILURE: allowed (count = 1, limit = 1)
        AuthenticationResponse resyncChallenge = limitedManager.verifyAuthenticationResponse(
            firstChallenge.getAuthCtxId(), null, auts1, null
        );
        assertTrue(resyncChallenge.getSuccess(), "first sync-failure should be allowed");

        // Read the server-precomputed AUTS for the new challenge
        String auts2 = limitedManager.getContext(FIVE_G_AKA_SUPI).getAuts();

        // Second SYNC_FAILURE: rejected (count = 2 > limit = 1)
        AuthenticationResponse result = limitedManager.verifyAuthenticationResponse(
            resyncChallenge.getAuthCtxId(), null, auts2, null
        );
        assertFalse(result.getSuccess());
        assertEquals("AUTHENTICATION_REJECTED", result.getErrorCode());
        assertTrue(result.getMessage().contains("max SYNC_FAILURE attempts exceeded"),
            "message should indicate limit exceeded; was: " + result.getMessage());
        assertEquals(AuthenticationStatus.FAILED, limitedManager.getContext(FIVE_G_AKA_SUPI).getStatus());
    }

    @Test
    void shouldRejectEapAkaPrimeOngoingWhenMaxRoundTripsExceeded() {
        AuthenticationManager limitedManager = new AuthenticationManager(cryptographyService, udmService);
        limitedManager.maxEapOngoing = 1;

        String supi = "imsi-250010000000002";
        AuthenticationResponse challenge = limitedManager.initiateAuthentication(
            "auth-1", supi, "5G:mnc001.mcc001.3gppnetwork.org", "EAP_AKA_PRIME"
        );
        AuthenticationContext ctx = limitedManager.getContext(supi);
        byte[] kAutPrime = cryptographyService.deriveKautPrime(ctx.getKausf());
        byte id = EapPacket.extractIdentifier(challenge.getEapChallenge());
        String reauthResponse = EapPacket.buildReauthenticationResponse(id, kAutPrime, false);

        // First re-auth: allowed (count = 1, limit = 1)
        AuthenticationResponse refreshedChallenge = limitedManager.verifyAuthenticationResponse(
            challenge.getAuthCtxId(), null, null, reauthResponse
        );
        assertTrue(refreshedChallenge.getSuccess(), "first re-auth should be allowed");
        assertEquals("EAP-AKA' re-authentication challenge generated", refreshedChallenge.getMessage());

        // Second re-auth: rejected (count = 2 > limit = 1)
        ctx = limitedManager.getContext(supi);
        byte[] kAutPrime2 = cryptographyService.deriveKautPrime(ctx.getKausf());
        byte id2 = EapPacket.extractIdentifier(refreshedChallenge.getEapChallenge());
        String secondReauthResponse = EapPacket.buildReauthenticationResponse(id2, kAutPrime2, false);
        AuthenticationResponse result = limitedManager.verifyAuthenticationResponse(
            challenge.getAuthCtxId(), null, null, secondReauthResponse
        );
        assertFalse(result.getSuccess());
        assertEquals("AUTHENTICATION_REJECTED", result.getErrorCode());
        assertTrue(result.getMessage().contains("max ongoing round-trips exceeded"),
            "message should indicate limit exceeded; was: " + result.getMessage());
        assertNotNull(result.getEapChallenge(), "should include EAP-Failure payload");
        assertEquals(AuthenticationStatus.FAILED, limitedManager.getContext(supi).getStatus());
    }

    private static final class InMemorySubscriberRepository implements SubscriberRepository {
        private final Map<String, SubscriberProfile> subscribers = new ConcurrentHashMap<>();

        private InMemorySubscriberRepository() {
            subscribers.put(FIVE_G_AKA_SUPI, createFiveGAkaSubscriber());
            subscribers.put("imsi-250010000000002", createEapAkaPrimeSubscriber());
            subscribers.put("imsi-250010000000003", createEapAkaPrimeMilenageSubscriber());
        }

        @Override
        public Optional<SubscriberProfile> findBySupi(String supi) {
            return Optional.ofNullable(subscribers.get(supi));
        }

        @Override
        public SubscriberProfile save(SubscriberProfile subscriberProfile) {
            subscribers.put(subscriberProfile.getSupi(), subscriberProfile);
            return subscriberProfile;
        }

        private static SubscriberProfile createFiveGAkaSubscriber() {
            SubscriberProfile profile = new SubscriberProfile();
            profile.setSupi(FIVE_G_AKA_SUPI);
            profile.setAuthMethod("5G_AKA");
            profile.setAkaAlgorithm(AkaAlgorithm.MILENAGE);
            profile.setPermanentKey(FIVE_G_AKA_KEY);
            profile.setOpc(FIVE_G_AKA_OPC);
            profile.setServingNetworkName("5G:mnc001.mcc001.3gppnetwork.org");
            profile.setSequenceNumber(16);
            profile.setRoutingIndicator("0001");
            return profile;
        }

        private static SubscriberProfile createEapAkaPrimeSubscriber() {
            SubscriberProfile profile = new SubscriberProfile();
            profile.setSupi("imsi-250010000000002");
            profile.setAuthMethod("EAP_AKA_PRIME");
            profile.setAkaAlgorithm(AkaAlgorithm.TUAK);
            profile.setPermanentKey("abababababababababababababababab");
            profile.setOpc("bd04d9530e87513c5d837ac2ad954623a8e2330c115305a73eb45d1f40cccbff");
            profile.setServingNetworkName("5G:mnc001.mcc001.3gppnetwork.org");
            profile.setSequenceNumber(32);
            profile.setRoutingIndicator("0002");
            return profile;
        }

        private static SubscriberProfile createEapAkaPrimeMilenageSubscriber() {
            SubscriberProfile profile = new SubscriberProfile();
            profile.setSupi("imsi-250010000000003");
            profile.setAuthMethod("EAP_AKA_PRIME");
            profile.setAkaAlgorithm(AkaAlgorithm.MILENAGE);
            profile.setPermanentKey(FIVE_G_AKA_KEY);
            profile.setOpc(FIVE_G_AKA_OPC);
            profile.setServingNetworkName("5G:mnc001.mcc001.3gppnetwork.org");
            profile.setSequenceNumber(48);
            profile.setRoutingIndicator("0003");
            return profile;
        }
    }
}
