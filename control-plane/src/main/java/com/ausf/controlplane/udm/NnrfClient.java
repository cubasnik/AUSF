package com.ausf.controlplane.udm;

import com.ausf.controlplane.config.TlsAwareRestClientBuilderCustomizer;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
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
        if (baseUrl.isBlank()) {
            return Optional.empty();
        }

        RestClientException lastException = null;
        for (int attempt = 1; attempt <= MAX_ATTEMPTS; attempt++) {
            try {
                NnrfDiscoveryResponse response = restClientBuilder.baseUrl(Objects.requireNonNull(baseUrl)).build().get()
                    .uri(uriBuilder -> uriBuilder
                        .path("/nnrf-disc/v1/nf-instances")
                        .queryParam("target-nf-type", "UDM")
                        .queryParam("requester-nf-type", "AUSF")
                        .build())
                    .retrieve()
                    .body(NnrfDiscoveryResponse.class);

                if (response == null || response.getNfInstances() == null) {
                    return Optional.empty();
                }

                for (NfInstance nfInstance : response.getNfInstances()) {
                    if (nfInstance.getServices() == null) {
                        continue;
                    }
                    for (NfService service : nfInstance.getServices()) {
                        if ("nudm-ueau".equals(service.getServiceName()) && service.getApiPrefix() != null && !service.getApiPrefix().isBlank()) {
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

    static class NfInstance {
        private List<NfService> services;

        public List<NfService> getServices() {
            return services;
        }

        public void setServices(List<NfService> services) {
            this.services = services;
        }
    }

    static class NfService {
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