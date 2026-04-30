package com.ausf.controlplane.udm;

import jakarta.annotation.PostConstruct;
import jakarta.annotation.PreDestroy;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.TimeUnit;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClientException;

/**
 * Manages AUSF NF registration lifecycle with the NRF:
 * <ul>
 *   <li>Registers on startup via PUT /nnrf-nfm/v1/nf-instances/{id}</li>
 *   <li>Sends periodic heartbeats via PATCH</li>
 *   <li>Deregisters on shutdown via DELETE</li>
 * </ul>
 * Does nothing when {@code ausf.nnrf.base-url} is blank.
 */
@Component
class NrfLifecycleManager {

    private static final Logger log = LoggerFactory.getLogger(NrfLifecycleManager.class);

    private final NnrfClient nnrfClient;
    private final String nfInstanceId;
    private final int heartbeatIntervalSeconds;

    private final ScheduledExecutorService scheduler = Executors.newSingleThreadScheduledExecutor(r -> {
        Thread t = new Thread(r, "nrf-heartbeat");
        t.setDaemon(true);
        return t;
    });

    private ScheduledFuture<?> heartbeatTask;
    private volatile boolean registered = false;

    NrfLifecycleManager(
            NnrfClient nnrfClient,
            @Value("${ausf.nnrf.nf-instance-id:}") String nfInstanceId,
            @Value("${ausf.nnrf.heartbeat-interval-seconds:30}") int heartbeatIntervalSeconds) {
        this.nnrfClient = nnrfClient;
        this.nfInstanceId = nfInstanceId.isBlank() ? UUID.randomUUID().toString() : nfInstanceId;
        this.heartbeatIntervalSeconds = heartbeatIntervalSeconds;
    }

    @PostConstruct
    void register() {
        if (!nnrfClient.isConfigured()) {
            log.info("NRF base-url not configured — skipping NF registration");
            return;
        }
        try {
            nnrfClient.registerNfProfile(nfInstanceId, buildNfProfile());
            registered = true;
            log.info("Registered with NRF as nfInstanceId={}", nfInstanceId);
            scheduleHeartbeat();
        } catch (RestClientException e) {
            log.warn("NRF registration failed (will retry via heartbeat): {}", e.getMessage());
            scheduleHeartbeat();
        }
    }

    @PreDestroy
    void deregister() {
        if (heartbeatTask != null) {
            heartbeatTask.cancel(false);
        }
        scheduler.shutdownNow();
        if (!nnrfClient.isConfigured() || !registered) {
            return;
        }
        try {
            nnrfClient.deregister(nfInstanceId);
            log.info("Deregistered from NRF nfInstanceId={}", nfInstanceId);
        } catch (RestClientException e) {
            log.warn("NRF deregistration failed: {}", e.getMessage());
        }
    }

    String getNfInstanceId() {
        return nfInstanceId;
    }

    boolean isRegistered() {
        return registered;
    }

    private void scheduleHeartbeat() {
        heartbeatTask = scheduler.scheduleAtFixedRate(
            this::heartbeat,
            heartbeatIntervalSeconds,
            heartbeatIntervalSeconds,
            TimeUnit.SECONDS
        );
    }

    private void heartbeat() {
        try {
            if (!registered) {
                nnrfClient.registerNfProfile(nfInstanceId, buildNfProfile());
                registered = true;
                log.info("Re-registered with NRF nfInstanceId={}", nfInstanceId);
            } else {
                nnrfClient.sendHeartbeat(nfInstanceId);
                log.debug("Heartbeat sent to NRF nfInstanceId={}", nfInstanceId);
            }
        } catch (RestClientException e) {
            log.warn("NRF heartbeat failed: {}", e.getMessage());
            registered = false;
        }
    }

    private Map<String, Object> buildNfProfile() {
        Map<String, Object> profile = new LinkedHashMap<>();
        profile.put("nfInstanceId", nfInstanceId);
        profile.put("nfType", "AUSF");
        profile.put("nfStatus", "REGISTERED");
        return profile;
    }
}
