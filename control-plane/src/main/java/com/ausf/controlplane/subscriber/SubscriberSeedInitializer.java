package com.ausf.controlplane.subscriber;

import jakarta.annotation.PostConstruct;
import java.util.List;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.stereotype.Component;
import org.springframework.transaction.annotation.Transactional;

@Component
@ConditionalOnProperty(name = "ausf.subscriber.seed.enabled", havingValue = "true", matchIfMissing = true)
public class SubscriberSeedInitializer {
    private final SubscriberJpaRepository subscriberRepository;

    public SubscriberSeedInitializer(SubscriberJpaRepository subscriberRepository) {
        this.subscriberRepository = subscriberRepository;
    }

    @PostConstruct
    @Transactional
    void seedSubscribers() {
        for (SubscriberProfile profile : defaultSubscribers()) {
            if (subscriberRepository.findById(profile.getSupi()).isEmpty()) {
                subscriberRepository.save(profile);
            }
        }
    }

    private List<SubscriberProfile> defaultSubscribers() {
        return List.of(
            createSubscriber(
                "imsi-250010000000001",
                "5G_AKA",
                AkaAlgorithm.MILENAGE,
                "e8ed289deba952e4283b54e88e6183ca",
                "465b5ce8b199b49faa5f0a2ee238a6bc",
                "0001",
                16,
                "5G:mnc001.mcc001.3gppnetwork.org"
            ),
            createSubscriber(
                "imsi-250010000000002",
                "EAP_AKA_PRIME",
                AkaAlgorithm.TUAK,
                "bd04d9530e87513c5d837ac2ad954623a8e2330c115305a73eb45d1f40cccbff",
                "abababababababababababababababab",
                "0002",
                32,
                "5G:mnc001.mcc001.3gppnetwork.org"
            )
        );
    }

    private SubscriberProfile createSubscriber(
        String supi,
        String authMethod,
        AkaAlgorithm akaAlgorithm,
        String opc,
        String permanentKey,
        String routingIndicator,
        long sequenceNumber,
        String servingNetworkName
    ) {
        SubscriberProfile profile = new SubscriberProfile();
        profile.setSupi(supi);
        profile.setAuthMethod(authMethod);
        profile.setAkaAlgorithm(akaAlgorithm);
        profile.setOpc(opc);
        profile.setPermanentKey(permanentKey);
        profile.setRoutingIndicator(routingIndicator);
        profile.setSequenceNumber(sequenceNumber);
        profile.setServingNetworkName(servingNetworkName);
        return profile;
    }
}