package com.ausf.controlplane.udm;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

import com.ausf.controlplane.authentication.CryptographyService;
import com.ausf.controlplane.crypto.Milenage;
import com.ausf.controlplane.crypto.Tuak;
import com.ausf.controlplane.subscriber.AkaAlgorithm;
import com.ausf.controlplane.subscriber.SubscriberProfile;
import com.ausf.controlplane.subscriber.SubscriberRepository;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;
import org.junit.jupiter.api.Test;

class UdmServiceTest {
    private final CryptographyService cryptographyService = new CryptographyService();

    @Test
    void shouldUseSubscriberConfiguredTuakForEapAkaPrimeSubscriber() {
        SubscriberProfile profile = new SubscriberProfile();
        profile.setSupi("imsi-250010000000100");
        profile.setAuthMethod("EAP_AKA_PRIME");
        profile.setAkaAlgorithm(AkaAlgorithm.TUAK);
        profile.setPermanentKey("abababababababababababababababab");
        profile.setOpc("bd04d9530e87513c5d837ac2ad954623a8e2330c115305a73eb45d1f40cccbff");
        profile.setServingNetworkName("5G:mnc001.mcc001.3gppnetwork.org");
        profile.setSequenceNumber(32);
        profile.setRoutingIndicator("0100");

        UdmService udmService = new UdmService(new InMemorySubscriberRepository(profile), cryptographyService);

        UdmAuthenticationData response = udmService.getAuthenticationData(
            profile.getSupi(),
            profile.getServingNetworkName(),
            profile.getAuthMethod()
        ).orElseThrow();

        AuthenticationVector vector = response.getAuthenticationVector();
        Tuak.TuakVector expected = cryptographyService.generateTuakVector(
            vector.getRand(),
            profile.getPermanentKey(),
            profile.getOpc(),
            32,
            profile.getServingNetworkName()
        );

        assertEquals(expected.autn(), vector.getAutn());
        assertEquals(expected.xresStar(), vector.getXresStar());
        assertEquals(expected.hxresStar(), vector.getHxresStar());
        assertEquals(expected.kausf(), vector.getKausf());
        // EAP challenge packet is now built by AuthenticationManager, not UdmService
        assertEquals(null, vector.getEapChallenge());
    }

    @Test
    void shouldUseSubscriberConfiguredMilenageForEapAkaPrimeSubscriber() {
        SubscriberProfile profile = new SubscriberProfile();
        profile.setSupi("imsi-250010000000101");
        profile.setAuthMethod("EAP_AKA_PRIME");
        profile.setAkaAlgorithm(AkaAlgorithm.MILENAGE);
        profile.setPermanentKey("465b5ce8b199b49faa5f0a2ee238a6bc");
        profile.setOpc("e8ed289deba952e4283b54e88e6183ca");
        profile.setServingNetworkName("5G:mnc001.mcc001.3gppnetwork.org");
        profile.setSequenceNumber(64);
        profile.setRoutingIndicator("0101");

        UdmService udmService = new UdmService(new InMemorySubscriberRepository(profile), cryptographyService);

        UdmAuthenticationData response = udmService.getAuthenticationData(
            profile.getSupi(),
            profile.getServingNetworkName(),
            profile.getAuthMethod()
        ).orElseThrow();

        AuthenticationVector vector = response.getAuthenticationVector();
        Milenage.MilenageVector expected = cryptographyService.generateMilenageVector(
            vector.getRand(),
            profile.getPermanentKey(),
            profile.getOpc(),
            64,
            profile.getServingNetworkName()
        );

        assertEquals(expected.autn(), vector.getAutn());
        assertEquals(expected.xresStar(), vector.getXresStar());
        assertEquals(expected.hxresStar(), vector.getHxresStar());
        assertEquals(expected.kausf(), vector.getKausf());
        // EAP challenge packet is now built by AuthenticationManager, not UdmService
        assertEquals(null, vector.getEapChallenge());
    }

    @Test
    void shouldResynchronizeMilenageBackedEapAkaPrimeSubscriber() {
        SubscriberProfile profile = new SubscriberProfile();
        profile.setSupi("imsi-250010000000103");
        profile.setAuthMethod("EAP_AKA_PRIME");
        profile.setAkaAlgorithm(AkaAlgorithm.MILENAGE);
        profile.setPermanentKey("465b5ce8b199b49faa5f0a2ee238a6bc");
        profile.setOpc("e8ed289deba952e4283b54e88e6183ca");
        profile.setServingNetworkName("5G:mnc001.mcc001.3gppnetwork.org");
        profile.setSequenceNumber(64);
        profile.setRoutingIndicator("0103");

        UdmService udmService = new UdmService(new InMemorySubscriberRepository(profile), cryptographyService);

        UdmAuthenticationData firstResponse = udmService.getAuthenticationData(
            profile.getSupi(),
            profile.getServingNetworkName(),
            profile.getAuthMethod()
        ).orElseThrow();
        String auts = cryptographyService.generateMilenageAuts(
            firstResponse.getAuthenticationVector().getRand(),
            profile.getPermanentKey(),
            profile.getOpc(),
            64
        );

        UdmAuthenticationData resynchronized = udmService.resynchronizeAuthenticationData(
            profile.getSupi(),
            profile.getServingNetworkName(),
            profile.getAuthMethod(),
            firstResponse.getAuthenticationVector().getRand(),
            auts
        ).orElseThrow();

        AuthenticationVector vector = resynchronized.getAuthenticationVector();
        assertEquals("EAP_AKA_PRIME", resynchronized.getAuthType());
        assertEquals("EAP_AKA_PRIME", vector.getAuthType());
        assertEquals("5G:mnc001.mcc001.3gppnetwork.org", resynchronized.getServingNetworkName());
        assertEquals(32, vector.getAutn().length());
        assertEquals(32, vector.getHxresStar().length());
        assertEquals(64, vector.getKausf().length());
        assertEquals(32, vector.getXresStar().length());
        // EAP challenge packet is now built by AuthenticationManager, not UdmService
        assertEquals(null, vector.getEapChallenge());
    }

    @Test
    void shouldRejectResynchronizationForTuakBackedFiveGAkaSubscriber() {
        SubscriberProfile profile = new SubscriberProfile();
        profile.setSupi("imsi-250010000000102");
        profile.setAuthMethod("5G_AKA");
        profile.setAkaAlgorithm(AkaAlgorithm.TUAK);
        profile.setPermanentKey("465b5ce8b199b49faa5f0a2ee238a6bc");
        profile.setOpc("e8ed289deba952e4283b54e88e6183ca");
        profile.setServingNetworkName("5G:mnc001.mcc001.3gppnetwork.org");
        profile.setSequenceNumber(16);
        profile.setRoutingIndicator("0102");

        UdmService udmService = new UdmService(new InMemorySubscriberRepository(profile), cryptographyService);

        IllegalArgumentException error = assertThrows(
            IllegalArgumentException.class,
            () -> udmService.resynchronizeAuthenticationData(
                profile.getSupi(),
                profile.getServingNetworkName(),
                profile.getAuthMethod(),
                "23553cbe9637a89d218ae64dae47bf35",
                "00112233445566778899aabbccdd"
            )
        );

        assertEquals("AUTS re-synchronization is only supported for Milenage-backed 5G_AKA or EAP_AKA_PRIME", error.getMessage());
    }

    private static final class InMemorySubscriberRepository implements SubscriberRepository {
        private final Map<String, SubscriberProfile> subscribers = new ConcurrentHashMap<>();

        private InMemorySubscriberRepository(SubscriberProfile subscriberProfile) {
            subscribers.put(subscriberProfile.getSupi(), subscriberProfile);
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
    }
}