package com.ausf.controlplane.udm;

import com.ausf.controlplane.config.TlsAwareRestClientBuilderCustomizer;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.stereotype.Component;
import org.springframework.web.client.HttpClientErrorException;
import org.springframework.web.client.RestClientResponseException;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

@Component
public class NnrfClient {
    private static final int MAX_ATTEMPTS = 3;
    private static final long BACKOFF_MILLIS = 200L;

    private final RestClient.Builder restClientBuilder;
    private final String baseUrl;

    public NnrfClient(
        RestClient.Builder restClientBuilder,
        TlsAwareRestClientBuilderCustomizer tlsCustomizer,
        @Value("${ausf.nnrf.base-url:}") String baseUrl
    ) {
	    this.restClientBuilder = tlsCustomizer.customize(restClientBuilder);
        this.baseUrl = sanitizeBaseUrl(baseUrl);
    }

    public Optional<String> resolveUdmBaseUrl() {
        return resolveServiceUrl("UDM", "nudm-ueau");
    }

    /**
     * Discovers NF instances of any type and returns the apiPrefix of the first
     * matching service. Retries on transient NRF failures.
     *
     * @param targetNfType the 3GPP NF type (e.g. "UDM", "AMF")
     * @param serviceName  the SBI service name (e.g. "nudm-ueau")
     */
    public Optional<String> resolveServiceUrl(String targetNfType, String serviceName) {
        if (baseUrl.isBlank()) return Optional.empty();
        RestClientException lastException = null;
        for (int attempt = 1; attempt <= MAX_ATTEMPTS; attempt++) {
            try {
                for (NfInstance instance : fetchNfInstances(targetNfType)) {
                    for (NfService service : coalesceServices(instance)) {
                        if (serviceName.equals(service.getServiceName())
                                && service.getApiPrefix() != null
                                && !service.getApiPrefix().isBlank()) {
                            return Optional.of(sanitizeBaseUrl(service.getApiPrefix()));
                        }
                    }
                }
                return Optional.empty();
            } catch (RestClientException exception) {
                lastException = exception;
                if (!shouldRetry(exception) || attempt == MAX_ATTEMPTS) {
                    throw new IllegalStateException("NRF discovery failed: " + exception.getMessage(), exception);
                }
                sleepBeforeRetry(attempt);
            }
        }
        throw new IllegalStateException(
            "NRF discovery failed: " + (lastException == null ? "unknown error" : lastException.getMessage()),
            lastException
        );
    }

    /**
     * Returns all NF instances of a given type registered with the NRF.
     * No retry — one-shot query; caller may choose to retry.
     *
     * @param targetNfType the 3GPP NF type (e.g. "UDM", "AMF")
     */
    public List<NfInstance> discoverNfInstances(String targetNfType) {
        if (baseUrl.isBlank()) return List.of();
        try {
            return fetchNfInstances(targetNfType);
        } catch (RestClientException e) {
            throw new IllegalStateException("NRF discovery failed: " + e.getMessage(), e);
        }
    }

    private List<NfInstance> fetchNfInstances(String targetNfType) {
        NnrfDiscoveryResponse response = restClientBuilder.baseUrl(Objects.requireNonNull(baseUrl)).build()
            .get()
            .uri(uriBuilder -> uriBuilder
                .path("/nnrf-disc/v1/nf-instances")
                .queryParam("target-nf-type", targetNfType)
                .queryParam("requester-nf-type", "AUSF")
                .build())
            .retrieve()
            .body(NnrfDiscoveryResponse.class);
        if (response == null || response.getNfInstances() == null) {
            return List.of();
        }
        return response.getNfInstances();
    }

    private List<NfService> coalesceServices(NfInstance instance) {
        // Spec uses nfServices; legacy mock uses services — accept both.
        if (instance.getNfServices() != null && !instance.getNfServices().isEmpty()) {
            return instance.getNfServices();
        }
        return instance.getServices() != null ? instance.getServices() : List.of();
    }

    // ── NRF NFm: register, heartbeat, deregister ─────────────────────────────

    /**
     * Registers this NF instance with the NRF.
     * PUT /nnrf-nfm/v1/nf-instances/{nfInstanceId}
     */
    void registerNfProfile(String nfInstanceId, Map<String, Object> nfProfile) {
        if (baseUrl.isBlank()) return;
        restClientBuilder.baseUrl(baseUrl).build()
            .put()
            .uri("/nnrf-nfm/v1/nf-instances/" + nfInstanceId)
            .contentType(MediaType.APPLICATION_JSON)
            .body(nfProfile)
            .retrieve()
            .toBodilessEntity();
    }

    /**
     * Sends a heartbeat (NFStatusNotify PATCH) to the NRF.
     * PATCH /nnrf-nfm/v1/nf-instances/{nfInstanceId}
     * Body: [{"op":"replace","path":"/nfStatus","value":"REGISTERED"}]
     */
    void sendHeartbeat(String nfInstanceId) {
        if (baseUrl.isBlank()) return;
        List<Map<String, String>> patch = List.of(
            Map.of("op", "replace", "path", "/nfStatus", "value", "REGISTERED")
        );
        restClientBuilder.baseUrl(baseUrl).build()
            .patch()
            .uri("/nnrf-nfm/v1/nf-instances/" + nfInstanceId)
            .contentType(MediaType.valueOf("application/json-patch+json"))
            .body(patch)
            .retrieve()
            .toBodilessEntity();
    }

    /**
     * Deregisters this NF instance from the NRF.
     * DELETE /nnrf-nfm/v1/nf-instances/{nfInstanceId}
     */
    void deregister(String nfInstanceId) {
        if (baseUrl.isBlank()) return;
        restClientBuilder.baseUrl(baseUrl).build()
            .delete()
            .uri("/nnrf-nfm/v1/nf-instances/" + nfInstanceId)
            .retrieve()
            .toBodilessEntity();
    }

    /**
     * Retrieves the NF profile of any registered NF instance.
     * GET /nnrf-nfm/v1/nf-instances/{nfInstanceId}
     *
     * @return the NF profile as a generic map, or empty if the NF is not found.
     */
    Optional<Map<String, Object>> getNfProfile(String nfInstanceId) {
        if (baseUrl.isBlank()) return Optional.empty();
        try {
            @SuppressWarnings("unchecked")
            Map<String, Object> profile = restClientBuilder.baseUrl(baseUrl).build()
                .get()
                .uri("/nnrf-nfm/v1/nf-instances/" + nfInstanceId)
                .retrieve()
                .body(Map.class);
            return Optional.ofNullable(profile);
        } catch (HttpClientErrorException.NotFound e) {
            return Optional.empty();
        }
    }

    /**
     * Subscribes to NF-status change notifications.
     * POST /nnrf-nfm/v1/subscriptions
     *
     * @param notificationUri the callback URI that NRF will POST to on status changes
     * @param reqNotifEvents  list of event types (e.g. "NF_REGISTERED", "NF_DEREGISTERED")
     * @return the subscription ID extracted from the Location header, or empty if NRF not configured.
     */
    Optional<String> subscribeNfStatusNotify(String notificationUri, List<String> reqNotifEvents) {
        if (baseUrl.isBlank() || notificationUri == null || notificationUri.isBlank()) {
            return Optional.empty();
        }
        Map<String, Object> body = Map.of(
            "nfStatusNotificationUri", notificationUri,
            "reqNotifEvents", reqNotifEvents,
            "subscrCond", Map.of("nfType", "UDM")
        );
        ResponseEntity<Void> response = restClientBuilder.baseUrl(baseUrl).build()
            .post()
            .uri("/nnrf-nfm/v1/subscriptions")
            .contentType(MediaType.APPLICATION_JSON)
            .body(body)
            .retrieve()
            .toBodilessEntity();
        String location = response.getHeaders().getFirst("Location");
        if (location == null || location.isBlank()) {
            return Optional.empty();
        }
        // Extract subscription ID from Location: .../subscriptions/{subsId}
        String subsId = location.substring(location.lastIndexOf('/') + 1);
        return subsId.isBlank() ? Optional.empty() : Optional.of(subsId);
    }

    /**
     * Cancels an NF-status subscription.
     * DELETE /nnrf-nfm/v1/subscriptions/{subsId}
     */
    void unsubscribeNfStatusNotify(String subsId) {
        if (baseUrl.isBlank() || subsId == null || subsId.isBlank()) return;
        try {
            restClientBuilder.baseUrl(baseUrl).build()
                .delete()
                .uri("/nnrf-nfm/v1/subscriptions/" + subsId)
                .retrieve()
                .toBodilessEntity();
        } catch (RestClientException e) {
            // best-effort: log but do not propagate
        }
    }

    // ── NRF NFDiscovery: discovery subscriptions ─────────────────────────────

    /**
     * Subscribes to NF-discovery notifications for one or more NF types.
     * POST /nnrf-disc/v1/subscriptions
     *
     * @param callbackUri   URI where NRF will POST discovery notifications
     * @param targetNfTypes list of NF types to watch (e.g. ["UDM", "AMF"])
     * @return subscription ID extracted from Location header, or empty if not configured
     */
    public Optional<String> subscribeNfDiscovery(String callbackUri, List<String> targetNfTypes) {
        if (baseUrl.isBlank() || callbackUri == null || callbackUri.isBlank()) return Optional.empty();
        Map<String, Object> body = Map.of(
            "callbackUri", callbackUri,
            "reqNotifEvents", List.of("NF_REGISTERED"),
            "nfTypeCondition", Map.of("nfTypes", targetNfTypes)
        );
        ResponseEntity<Void> response = restClientBuilder.baseUrl(baseUrl).build()
            .post()
            .uri("/nnrf-disc/v1/subscriptions")
            .contentType(MediaType.APPLICATION_JSON)
            .body(body)
            .retrieve()
            .toBodilessEntity();
        String location = response.getHeaders().getFirst("Location");
        if (location == null || location.isBlank()) return Optional.empty();
        String subsId = location.substring(location.lastIndexOf('/') + 1);
        return subsId.isBlank() ? Optional.empty() : Optional.of(subsId);
    }

    /**
     * Cancels an NF-discovery subscription.
     * DELETE /nnrf-disc/v1/subscriptions/{subsId}
     */
    public void unsubscribeNfDiscovery(String subsId) {
        if (baseUrl.isBlank() || subsId == null || subsId.isBlank()) return;
        try {
            restClientBuilder.baseUrl(baseUrl).build()
                .delete()
                .uri("/nnrf-disc/v1/subscriptions/" + subsId)
                .retrieve()
                .toBodilessEntity();
        } catch (RestClientException e) {
            // best-effort
        }
    }

    boolean isConfigured() {
        return !baseUrl.isBlank();
    }

    private String sanitizeBaseUrl(String value) {
        return value == null ? "" : value.replaceAll("/+$", "");
    }

    private boolean shouldRetry(RestClientException exception) {
        if (exception instanceof RestClientResponseException responseException) {
            return responseException.getStatusCode().is5xxServerError();
        }
        return true;
    }

    private void sleepBeforeRetry(int attempt) {
        try {
            Thread.sleep(BACKOFF_MILLIS * attempt);
        } catch (InterruptedException exception) {
            Thread.currentThread().interrupt();
            throw new IllegalStateException("retry interrupted", exception);
        }
    }

    static class NnrfDiscoveryResponse {
        private List<NfInstance> nfInstances;

        public List<NfInstance> getNfInstances() {
            return nfInstances;
        }

        public void setNfInstances(List<NfInstance> nfInstances) {
            this.nfInstances = nfInstances;
        }
    }

    public static class NfInstance {
        private String nfInstanceId;
        private String nfType;
        private String nfStatus;
        private List<NfService> services;
        private List<NfService> nfServices;

        public String getNfInstanceId() { return nfInstanceId; }
        public void setNfInstanceId(String nfInstanceId) { this.nfInstanceId = nfInstanceId; }
        public String getNfType() { return nfType; }
        public void setNfType(String nfType) { this.nfType = nfType; }
        public String getNfStatus() { return nfStatus; }
        public void setNfStatus(String nfStatus) { this.nfStatus = nfStatus; }
        public List<NfService> getServices() { return services; }
        public void setServices(List<NfService> services) { this.services = services; }
        public List<NfService> getNfServices() { return nfServices; }
        public void setNfServices(List<NfService> nfServices) { this.nfServices = nfServices; }
    }

    public static class NfService {
        private String serviceName;
        private String apiPrefix;

        public String getServiceName() {
            return serviceName;
        }

        public void setServiceName(String serviceName) {
            this.serviceName = serviceName;
        }

        public String getApiPrefix() {
            return apiPrefix;
        }

        public void setApiPrefix(String apiPrefix) {
            this.apiPrefix = apiPrefix;
        }
    }
}