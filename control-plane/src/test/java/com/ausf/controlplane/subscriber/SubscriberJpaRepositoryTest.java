package com.ausf.controlplane.subscriber;

import java.util.Optional;
import static org.junit.jupiter.api.Assertions.*;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.data.jpa.test.autoconfigure.DataJpaTest;

@DataJpaTest
class SubscriberJpaRepositoryTest {
    @Autowired
    private SubscriberJpaRepository repository;

    @Test
    void canSaveAndFindById() {
        SubscriberProfile profile = new SubscriberProfile();
        profile.setSupi("imsi-250010000000003");
        profile.setAuthMethod("5G_AKA");
        profile.setAkaAlgorithm(AkaAlgorithm.MILENAGE);
        profile.setPermanentKey("00112233445566778899aabbccddeeff");
        profile.setOpc("ffeeddccbbaa99887766554433221100");
        profile.setServingNetworkName("5G:mnc001.mcc001.3gppnetwork.org");
        profile.setSequenceNumber(42);
        profile.setRoutingIndicator("0003");
        repository.save(profile);

        Optional<SubscriberProfile> found = repository.findById("imsi-250010000000003");
        assertTrue(found.isPresent());
        assertEquals("5G_AKA", found.get().getAuthMethod());
        assertEquals(AkaAlgorithm.MILENAGE, found.get().getAkaAlgorithm());
    }
}
