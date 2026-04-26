package com.ausf.controlplane.subscriber;

import java.util.Optional;

public interface SubscriberRepository {
    Optional<SubscriberProfile> findBySupi(String supi);
}
