package com.ausf.controlplane.udm;

import com.ausf.controlplane.authentication.CryptographyService;
import com.ausf.controlplane.subscriber.SubscriberProfile;
import com.ausf.controlplane.subscriber.SubscriberRepository;
import java.util.Optional;
import org.springframework.stereotype.Service;

@Service
public class UdmService {
    private final SubscriberRepository subscriberRepository;
    private final CryptographyService cryptographyService;

    public UdmService(SubscriberRepository subscriberRepository, CryptographyService cryptographyService) {
        this.subscriberRepository = subscriberRepository;
        this.cryptographyService = cryptographyService;
    }

    public Optional<SubscriberProfile> findSubscriber(String supi) {
        return subscriberRepository.findBySupi(supi);
    }

    public AuthenticationVector generateVector(SubscriberProfile subscriberProfile) {
        String rand = cryptographyService.digestHex(
            subscriberProfile.getPermanentKey() + subscriberProfile.getOpc() + subscriberProfile.getSequenceNumber()
        ).substring(0, 32);
        String autn = cryptographyService.digestHex(
            subscriberProfile.getOpc() + subscriberProfile.getServingNetworkName() + subscriberProfile.getRoutingIndicator()
        ).substring(0, 32);
        String xresStar = cryptographyService.digestHex(rand + autn + subscriberProfile.getPermanentKey()).substring(0, 32);
        String hxresStar = cryptographyService.digestHex(rand + xresStar).substring(0, 32);
        String kausf = cryptographyService.digestHex(subscriberProfile.getPermanentKey() + subscriberProfile.getServingNetworkName());
        String eapChallenge = "EAP-Request/AKA'-Challenge " + cryptographyService.digestHex(autn + xresStar).substring(0, 24);

        return new AuthenticationVector(
            subscriberProfile.getAuthMethod(),
            rand,
            autn,
            xresStar,
            hxresStar,
            kausf,
            eapChallenge
        );
    }
}
