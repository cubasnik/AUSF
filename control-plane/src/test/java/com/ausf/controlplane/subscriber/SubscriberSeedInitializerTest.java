package com.ausf.controlplane.subscriber;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.orm.jpa.DataJpaTest;
import org.springframework.context.annotation.Import;

@DataJpaTest
@Import(SubscriberSeedInitializer.class)
class SubscriberSeedInitializerTest {
    @Autowired
    private SubscriberJpaRepository repository;

    @Test
    void shouldSeedSubscriberProfilesFromInitializer() {
        assertEquals(2, repository.count());
        assertTrue(repository.findById("imsi-250010000000001").isPresent());
        assertTrue(repository.findById("imsi-250010000000002").isPresent());
    }
}