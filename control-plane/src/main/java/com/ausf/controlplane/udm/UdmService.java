package com.ausf.controlplane.udm;

import com.ausf.controlplane.authentication.CryptographyService;
import com.ausf.controlplane.crypto.Milenage;
import com.ausf.controlplane.crypto.Tuak;
import com.ausf.controlplane.subscriber.AkaAlgorithm;
import com.ausf.controlplane.subscriber.SubscriberProfile;
import com.ausf.controlplane.subscriber.SubscriberRepository;
import java.util.Optional;
import java.util.OptionalLong;
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

        return Optional.of(issueAuthenticationData(subscriberProfile, resolvedServingNetworkName, resolvedAuthType));
    }

    @Override
    public Optional<UdmAuthenticationData> resynchronizeAuthenticationData(
        String supi,
        String servingNetworkName,
        String authType,
        String rand,
        String auts
    ) {
        Optional<SubscriberProfile> subscriber = subscriberRepository.findBySupi(supi);
        if (subscriber.isEmpty()) {
            return Optional.empty();
        }

        SubscriberProfile subscriberProfile = subscriber.get();
        String resolvedServingNetworkName = resolveServingNetworkName(servingNetworkName, subscriberProfile);
        String resolvedAuthType = resolveAuthType(authType, subscriberProfile);
        if (!"5G_AKA".equals(resolvedAuthType) && !"EAP_AKA_PRIME".equals(resolvedAuthType)) {
            throw new IllegalArgumentException("AUTS re-synchronization is only supported for 5G_AKA or EAP_AKA_PRIME");
        }
        if (resolveAkaAlgorithm(subscriberProfile, resolvedAuthType) != AkaAlgorithm.MILENAGE) {
            throw new IllegalArgumentException("AUTS re-synchronization is only supported for Milenage-backed 5G_AKA or EAP_AKA_PRIME");
        }

        OptionalLong recoveredSqn = cryptographyService.validateMilenageAuts(
            rand,
            auts,
            subscriberProfile.getPermanentKey(),
            subscriberProfile.getOpc()
        );
        if (recoveredSqn.isEmpty()) {
            throw new IllegalArgumentException("AUTS verification failed");
        }

        long nextSqn = Math.max(subscriberProfile.getSequenceNumber(), recoveredSqn.getAsLong() + 1L);
        subscriberProfile.setSequenceNumber(nextSqn);
        return Optional.of(issueAuthenticationData(subscriberProfile, resolvedServingNetworkName, resolvedAuthType));
    }

    private AuthenticationVector generateVector(
        SubscriberProfile subscriberProfile,
        String servingNetworkName,
        String authType
    ) {
        String rand = cryptographyService.generateRandomHex(32);
        long sequenceNumber = subscriberProfile.getSequenceNumber();
        AkaAlgorithm akaAlgorithm = resolveAkaAlgorithm(subscriberProfile, authType);

        if (akaAlgorithm == AkaAlgorithm.TUAK) {
            Tuak.TuakVector v = cryptographyService.generateTuakVector(
                rand,
                subscriberProfile.getPermanentKey(),
                subscriberProfile.getOpc(),
                sequenceNumber,
                servingNetworkName
            );
            return new AuthenticationVector(
                authType, v.rand(), v.autn(), null, v.xresStar(), v.hxresStar(), v.kausf(), null
            );
        }

        Milenage.MilenageVector v = cryptographyService.generateMilenageVector(
            rand,
            subscriberProfile.getPermanentKey(),
            subscriberProfile.getOpc(),
            sequenceNumber,
            servingNetworkName
        );
        String auts = "5G_AKA".equals(authType)
            ? cryptographyService.generateMilenageAuts(
                rand,
                subscriberProfile.getPermanentKey(),
                subscriberProfile.getOpc(),
                sequenceNumber
            )
            : null;
        return new AuthenticationVector(
            authType, v.rand(), v.autn(), auts, v.xresStar(), v.hxresStar(), v.kausf(), null
        );
    }

    private UdmAuthenticationData issueAuthenticationData(
        SubscriberProfile subscriberProfile,
        String servingNetworkName,
        String authType
    ) {
        AuthenticationVector vector = generateVector(subscriberProfile, servingNetworkName, authType);
        subscriberProfile.setSequenceNumber(subscriberProfile.getSequenceNumber() + 1L);
        subscriberRepository.save(subscriberProfile);
        return new UdmAuthenticationData(
            subscriberProfile.getSupi(),
            authType,
            servingNetworkName,
            vector
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

    private AkaAlgorithm resolveAkaAlgorithm(SubscriberProfile subscriberProfile, String authType) {
        return subscriberProfile.getAkaAlgorithm() != null
            ? subscriberProfile.getAkaAlgorithm()
            : AkaAlgorithm.defaultForAuthMethod(authType);
    }

}
