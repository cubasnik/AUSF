package com.ausf.controlplane.udm;

import java.util.Objects;
import java.util.Optional;
import java.util.function.Supplier;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.client.HttpClientErrorException;
import org.springframework.web.client.RestClientResponseException;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

@Component
@ConditionalOnProperty(name = "ausf.udm.mode", havingValue = "http")
public class HttpUdmClient implements UdmClient {
    private static final int MAX_ATTEMPTS = 3;
    private static final long BACKOFF_MILLIS = 200L;

    private final RestClient.Builder restClientBuilder;
    private final NnrfClient nnrfClient;
    private final String configuredBaseUrl;

    public HttpUdmClient(
        RestClient.Builder restClientBuilder,
        NnrfClient nnrfClient,
        @Value("${ausf.udm.base-url:}") String baseUrl
    ) {
        this.restClientBuilder = restClientBuilder;
        this.nnrfClient = nnrfClient;
        this.configuredBaseUrl = sanitizeBaseUrl(baseUrl);
    }

    @Override
    public Optional<UdmAuthenticationData> getAuthenticationData(String supi, String servingNetworkName, String authType) {
        try {
            UdmGenerateAuthDataResponse response = executeWithRetry(() -> restClientBuilder.baseUrl(Objects.requireNonNull(resolveBaseUrl())).build().post()
                .uri("/nudm-ueau/v1/{supi}/security-information/generate-auth-data", supi)
                .contentType(Objects.requireNonNull(MediaType.APPLICATION_JSON))
                .body(new UdmGenerateAuthDataRequest(servingNetworkName, authType))
                .retrieve()
                .body(UdmGenerateAuthDataResponse.class));

            if (response == null) {
                throw new IllegalStateException("UDM returned an empty authentication data response");
            }

            AuthenticationVector authenticationVector = new AuthenticationVector(
                coalesce(response.getAuthType(), authType),
                response.getRand(),
                response.getAutn(),
                response.getXresStar(),
                response.getHxresStar(),
                response.getKausf(),
                response.getEapChallenge()
            );

            return Optional.of(new UdmAuthenticationData(
                coalesce(response.getSupi(), supi),
                coalesce(response.getAuthType(), authType),
                coalesce(response.getServingNetworkName(), servingNetworkName),
                authenticationVector
            ));
        } catch (HttpClientErrorException.NotFound exception) {
            return Optional.empty();
        } catch (RestClientException exception) {
            throw new IllegalStateException("UDM request failed: " + exception.getMessage(), exception);
        }
    }

    private String coalesce(String primary, String fallback) {
        return primary == null || primary.isBlank() ? fallback : primary;
    }

    private String resolveBaseUrl() {
        if (!configuredBaseUrl.isBlank()) {
            return configuredBaseUrl;
        }

        return nnrfClient.resolveUdmBaseUrl().orElseThrow(() ->
            new IllegalStateException("UDM base URL is not configured and NRF discovery did not return a UDM endpoint")
        );
    }

    private String sanitizeBaseUrl(String value) {
        return value == null ? "" : value.replaceAll("/+$", "");
    }

    private <T> T executeWithRetry(Supplier<T> call) {
        RestClientException lastException = null;
        for (int attempt = 1; attempt <= MAX_ATTEMPTS; attempt++) {
            try {
                return call.get();
            } catch (RestClientException exception) {
                lastException = exception;
                if (!shouldRetry(exception) || attempt == MAX_ATTEMPTS) {
                    throw exception;
                }
                sleepBeforeRetry(attempt);
            }
        }
        throw new IllegalStateException(
            "UDM request failed: " + (lastException == null ? "unknown error" : lastException.getMessage()),
            lastException
        );
    }

    private boolean shouldRetry(RestClientException exception) {
        if (exception instanceof HttpClientErrorException.NotFound) {
            return false;
        }
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

    static class UdmGenerateAuthDataRequest {
        private String servingNetworkName;
        private String authType;

        UdmGenerateAuthDataRequest(String servingNetworkName, String authType) {
            this.servingNetworkName = servingNetworkName;
            this.authType = authType;
        }

        public String getServingNetworkName() {
            return servingNetworkName;
        }

        public void setServingNetworkName(String servingNetworkName) {
            this.servingNetworkName = servingNetworkName;
        }

        public String getAuthType() {
            return authType;
        }

        public void setAuthType(String authType) {
            this.authType = authType;
        }
    }

    static class UdmGenerateAuthDataResponse {
        private String supi;
        private String authType;
        private String servingNetworkName;
        private String rand;
        private String autn;
        private String xresStar;
        private String hxresStar;
        private String kausf;
        private String eapChallenge;

        public String getSupi() {
            return supi;
        }

        public void setSupi(String supi) {
            this.supi = supi;
        }

        public String getAuthType() {
            return authType;
        }

        public void setAuthType(String authType) {
            this.authType = authType;
        }

        public String getServingNetworkName() {
            return servingNetworkName;
        }

        public void setServingNetworkName(String servingNetworkName) {
            this.servingNetworkName = servingNetworkName;
        }

        public String getRand() {
            return rand;
        }

        public void setRand(String rand) {
            this.rand = rand;
        }

        public String getAutn() {
            return autn;
        }

        public void setAutn(String autn) {
            this.autn = autn;
        }

        public String getXresStar() {
            return xresStar;
        }

        public void setXresStar(String xresStar) {
            this.xresStar = xresStar;
        }

        public String getHxresStar() {
            return hxresStar;
        }

        public void setHxresStar(String hxresStar) {
            this.hxresStar = hxresStar;
        }

        public String getKausf() {
            return kausf;
        }

        public void setKausf(String kausf) {
            this.kausf = kausf;
        }

        public String getEapChallenge() {
            return eapChallenge;
        }

        public void setEapChallenge(String eapChallenge) {
            this.eapChallenge = eapChallenge;
        }
    }
}