package com.ausf.controlplane.udm;

import java.util.List;
import java.util.Optional;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

@Component
public class NnrfClient {
    private final RestClient.Builder restClientBuilder;
    private final String baseUrl;

    public NnrfClient(RestClient.Builder restClientBuilder, @Value("${ausf.nnrf.base-url:}") String baseUrl) {
        this.restClientBuilder = restClientBuilder;
        this.baseUrl = sanitizeBaseUrl(baseUrl);
    }

    public Optional<String> resolveUdmBaseUrl() {
        if (baseUrl.isBlank()) {
            return Optional.empty();
        }

        try {
            NnrfDiscoveryResponse response = restClientBuilder.baseUrl(baseUrl).build().get()
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
            throw new IllegalStateException("NRF discovery failed: " + exception.getMessage(), exception);
        }
    }

    private String sanitizeBaseUrl(String value) {
        return value == null ? "" : value.replaceAll("/+$", "");
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