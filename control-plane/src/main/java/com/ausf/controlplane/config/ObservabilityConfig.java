package com.ausf.controlplane.config;

import io.micrometer.observation.ObservationRegistry;
import io.micrometer.observation.aop.ObservedAspect;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Enables {@link io.micrometer.observation.annotation.Observed @Observed} AOP support so that
 * annotated methods in {@code @Service} beans (e.g. {@code AuthenticationManager}) produce
 * OpenTelemetry child spans that are linked to the incoming W3C {@code traceparent} context
 * propagated by the Go microservice.
 */
@Configuration
public class ObservabilityConfig {

    @Bean
    ObservedAspect observedAspect(ObservationRegistry observationRegistry) {
        return new ObservedAspect(observationRegistry);
    }
}
