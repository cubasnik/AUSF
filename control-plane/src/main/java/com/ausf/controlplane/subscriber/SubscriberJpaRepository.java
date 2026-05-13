package com.ausf.controlplane.subscriber;

import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.stereotype.Repository;

@Repository
public interface SubscriberJpaRepository extends JpaRepository<SubscriberProfile, String> {
    // findBySupi is covered by findById
}
