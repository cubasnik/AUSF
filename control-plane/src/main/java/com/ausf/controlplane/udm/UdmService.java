package com.ausf.controlplane.udm;

import com.ausf.controlplane.authentication.CryptographyService;
import com.ausf.controlplane.subscriber.SubscriberProfile;
import com.ausf.controlplane.subscriber.SubscriberRepository;
import java.util.Optional;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.stereotype.Service;

@Service
@ConditionalOnProperty(name = "ausf.udm.mode", havingValue = "mock", matchIfMissing = true)
public class UdmService implements UdmClient {
    private final SubscriberRepository subscriberRepository;
    private final CryptographyService cryptographyService;

    public UdmService(SubscriberRepository subscriberRepository, CryptographyService cryptographyService) {
        this.subscriberRepository = subscriberRepository;
        this.cryptographyService = cryptographyService;
    }

    @Override
    public Optional<UdmAuthenticationData> getAuthenticationData(String supi, String servingNetworkName, String authType) {
        Optional<SubscriberProfile> subscriber = subscriberRepository.findBySupi(supi);
        if (subscriber.isEmpty()) {
            return Optional.empty();
        }

        SubscriberProfile subscriberProfile = subscriber.get();
        String resolvedServingNetworkName = resolveServingNetworkName(servingNetworkName, subscriberProfile);
        String resolvedAuthType = resolveAuthType(authType, subscriberProfile);

        return Optional.of(new UdmAuthenticationData(
            subscriberProfile.getSupi(),
            resolvedAuthType,
            resolvedServingNetworkName,
            generateVector(subscriberProfile, resolvedServingNetworkName, resolvedAuthType)
        ));
    }

    private AuthenticationVector generateVector(
        SubscriberProfile subscriberProfile,
        String servingNetworkName,
        String authType
    ) {
        String rand = cryptographyService.digestHex(
            subscriberProfile.getPermanentKey() + subscriberProfile.getOpc() + subscriberProfile.getSequenceNumber()
        ).substring(0, 32);
        String autn = cryptographyService.digestHex(
            subscriberProfile.getOpc() + servingNetworkName + subscriberProfile.getRoutingIndicator()
        ).substring(0, 32);
        String xresStar = cryptographyService.digestHex(rand + autn + subscriberProfile.getPermanentKey()).substring(0, 32);
        String hxresStar = cryptographyService.digestHex(rand + xresStar).substring(0, 32);
        String kausf = cryptographyService.digestHex(subscriberProfile.getPermanentKey() + servingNetworkName);
        String eapChallenge = "EAP-Request/AKA'-Challenge " + cryptographyService.digestHex(autn + xresStar).substring(0, 24);

        return new AuthenticationVector(
            authType,
            rand,
            autn,
            xresStar,
            hxresStar,
            kausf,
            eapChallenge
        );
    }

    private String resolveServingNetworkName(String requestedServingNetworkName, SubscriberProfile subscriberProfile) {
        return requestedServingNetworkName == null || requestedServingNetworkName.isBlank()
            ? subscriberProfile.getServingNetworkName()
            : requestedServingNetworkName;
    }

    private String resolveAuthType(String requestedAuthType, SubscriberProfile subscriberProfile) {
        return requestedAuthType == null || requestedAuthType.isBlank()
            ? subscriberProfile.getAuthMethod()
            : requestedAuthType;
    }
}
