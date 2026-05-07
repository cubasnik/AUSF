package com.ausf.controlplane.subscriber;

import java.util.Optional;
import org.springframework.stereotype.Repository;

@Repository
public class DatabaseSubscriberRepository implements SubscriberRepository {
    private final SubscriberJpaRepository jpaRepository;

    public DatabaseSubscriberRepository(SubscriberJpaRepository jpaRepository) {
        this.jpaRepository = jpaRepository;
    }

    @Override
    public Optional<SubscriberProfile> findBySupi(String supi) {
        return jpaRepository.findById(supi);
    }

    @Override
    public SubscriberProfile save(SubscriberProfile subscriberProfile) {
        return jpaRepository.save(subscriberProfile);
    }
}
