package com.ausf.controlplane.udm;

import tools.jackson.core.type.TypeReference;
import tools.jackson.databind.ObjectMapper;
import com.ausf.controlplane.config.TlsAwareRestClientBuilderCustomizer;
import java.time.Duration;
import java.time.Instant;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.function.Supplier;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
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
    static final int DEFAULT_BREAKER_FAILURES = 5;
    static final int DEFAULT_BREAKER_OPEN_SECONDS = 10;
    private static final ObjectMapper OBJECT_MAPPER = new ObjectMapper();

    private final RestClient.Builder restClientBuilder;
    private final NnrfClient nnrfClient;
    private final String configuredBaseUrl;
    final UdmCircuitBreaker circuitBreaker;

    public HttpUdmClient(
        RestClient.Builder restClientBuilder,
        TlsAwareRestClientBuilderCustomizer tlsCustomizer,
        NnrfClient nnrfClient,
        @Value("${ausf.udm.base-url:}") String baseUrl,
        @Value("${ausf.udm.breaker.failure-threshold:" + DEFAULT_BREAKER_FAILURES + "}") int breakerFailures,
        @Value("${ausf.udm.breaker.open-duration-seconds:" + DEFAULT_BREAKER_OPEN_SECONDS + "}") int breakerOpenSeconds
    ) {
	    this.restClientBuilder = tlsCustomizer.customize(restClientBuilder);
        this.nnrfClient = nnrfClient;
        this.configuredBaseUrl = sanitizeBaseUrl(baseUrl);
        this.circuitBreaker = new UdmCircuitBreaker(breakerFailures, Duration.ofSeconds(breakerOpenSeconds));
    }

    @Override
    public Optional<UdmAuthenticationData> getAuthenticationData(String supi, String servingNetworkName, String authType) {
        try {
            return requestAuthenticationData(
                supi,
                servingNetworkName,
                authType,
                new UdmGenerateAuthDataRequest(servingNetworkName, authType)
            );
        } catch (HttpClientErrorException.NotFound exception) {
            return Optional.empty();
        } catch (RestClientException exception) {
            throw new IllegalStateException("UDM request failed: " + exception.getMessage(), exception);
        } catch (UdmUnavailableException exception) {
            throw new IllegalStateException("UDM circuit breaker is open: " + exception.getMessage(), exception);
        }
    }

    @Override
    public Optional<UdmAuthenticationData> resynchronizeAuthenticationData(
        String supi,
        String servingNetworkName,
        String authType,
        String rand,
        String auts
    ) {
        try {
            return requestAuthenticationData(
                supi,
                servingNetworkName,
                authType,
                Map.of(
                    "servingNetworkName", servingNetworkName,
                    "authType", authType,
                    "rand", rand,
                    "auts", auts
                )
            );
        } catch (HttpClientErrorException.BadRequest exception) {
            throw new IllegalArgumentException(extractErrorDetail(exception));
        } catch (HttpClientErrorException.NotFound exception) {
            return Optional.empty();
        } catch (RestClientException exception) {
            throw new IllegalStateException("UDM request failed: " + exception.getMessage(), exception);
        } catch (UdmUnavailableException exception) {
            throw new IllegalStateException("UDM circuit breaker is open: " + exception.getMessage(), exception);
        }
    }

    @Override
    public void confirmAuthEvent(
        String supi,
        String authEventId,
        boolean success,
        String authType,
        String servingNetworkName
    ) {
        if (authEventId == null || authEventId.isBlank()) return;
        try {
            String baseUrl = resolveBaseUrl();
            Map<String, Object> body = Map.of(
                "success", success,
                "timeStamp", Instant.now().toString(),
                "authType", authType,
                "servingNetworkName", servingNetworkName
            );
            restClientBuilder.baseUrl(baseUrl).build()
                .put()
                .uri("/nudm-ueau/v1/{supi}/auth-events/{authEventId}", supi, authEventId)
                .contentType(MediaType.APPLICATION_JSON)
                .body(body)
                .retrieve()
                .toBodilessEntity();
        } catch (Exception e) {
            // best-effort: confirmation failure must not fail the authentication flow
        }
    }

    @Override
    public void deleteAuthEvent(String supi, String authEventId) {
        if (authEventId == null || authEventId.isBlank()) return;
        try {
            String baseUrl = resolveBaseUrl();
            restClientBuilder.baseUrl(baseUrl).build()
                .delete()
                .uri("/nudm-ueau/v1/{supi}/auth-events/{authEventId}", supi, authEventId)
                .retrieve()
                .toBodilessEntity();
        } catch (Exception e) {
            // best-effort: deletion failure must not fail the authentication flow
        }
    }

    private Optional<UdmAuthenticationData> requestAuthenticationData(
        String supi,
        String servingNetworkName,
        String authType,
        Object requestPayload
    ) {
        ResponseEntity<UdmGenerateAuthDataResponse> entity = executeWithRetry(() ->
            restClientBuilder.baseUrl(Objects.requireNonNull(resolveBaseUrl())).build().post()
                .uri("/nudm-ueau/v1/{supi}/security-information/generate-auth-data", supi)
                .contentType(Objects.requireNonNull(MediaType.APPLICATION_JSON))
                .body(requestPayload)
                .retrieve()
                .toEntity(UdmGenerateAuthDataResponse.class));

        UdmGenerateAuthDataResponse response = entity.getBody();
        if (response == null) {
            throw new IllegalStateException("UDM returned an empty authentication data response");
        }

        // Support both flat (legacy/mock) and spec-compliant nested authVector format (TS 29.503)
        UdmGenerateAuthDataResponse.NestedAuthVector av = response.getAuthVector();
        String rand          = coalesce(response.getRand(),          av != null ? av.getRand()          : null);
        String autn          = coalesce(response.getAutn(),          av != null ? av.getAutn()          : null);
        String auts          = coalesce(response.getAuts(),          av != null ? av.getAuts()          : null);
        String xresStar      = coalesce(response.getXresStar(),      av != null ? av.getXresStar()      : null);
        String hxresStar     = coalesce(response.getHxresStar(),     av != null ? av.getHxresStar()     : null);
        String kausf         = coalesce(response.getKausf(),         av != null ? av.getKausf()         : null);
        String eapChallenge  = coalesce(response.getEapChallenge(),  av != null ? av.getEapChallenge()  : null);

        AuthenticationVector authenticationVector = new AuthenticationVector(
            coalesce(response.getAuthType(), authType),
            rand, autn, auts, xresStar, hxresStar, kausf, eapChallenge
        );

        UdmAuthenticationData authData = new UdmAuthenticationData(
            coalesce(response.getSupi(), supi),
            coalesce(response.getAuthType(), authType),
            coalesce(response.getServingNetworkName(), servingNetworkName),
            authenticationVector
        );
        authData.setAuthEventId(extractAuthEventId(entity.getHeaders().getFirst("Location")));
        return Optional.of(authData);
    }

    private String extractAuthEventId(String location) {
        if (location == null || location.isBlank()) return null;
        int lastSlash = location.lastIndexOf('/');
        if (lastSlash < 0 || lastSlash == location.length() - 1) return null;
        String id = location.substring(lastSlash + 1).trim();
        return id.isBlank() ? null : id;
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
        if (!circuitBreaker.allowRequest()) {
            throw new UdmUnavailableException("circuit breaker is open — UDM requests are blocked");
        }
        RestClientException lastException = null;
        for (int attempt = 1; attempt <= MAX_ATTEMPTS; attempt++) {
            try {
                T result = call.get();
                circuitBreaker.onSuccess();
                return result;
            } catch (RestClientException exception) {
                lastException = exception;
                if (isBreakerFailure(exception)) {
                    circuitBreaker.onFailure();
                }
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

    /** Returns true for failure types that should "count" against the circuit breaker. */
    private boolean isBreakerFailure(RestClientException exception) {
        if (exception instanceof HttpClientErrorException) {
            // 4xx are client errors, not UDM unavailability
            return false;
        }
        return true;
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

    private String extractErrorDetail(HttpClientErrorException exception) {
        String responseBody = exception.getResponseBodyAsString();
        if (responseBody == null || responseBody.isBlank()) {
            return exception.getMessage();
        }
        try {
            Map<String, Object> payload = OBJECT_MAPPER.readValue(responseBody, new TypeReference<>() { });
            Object detail = payload.get("detail");
            if (detail instanceof String detailString && !detailString.isBlank()) {
                return detailString;
            }
        } catch (Exception ignored) {
            // Fall through to the raw exception message when the response body is not JSON.
        }
        return exception.getMessage();
    }

    static class UdmGenerateAuthDataRequest {
        private String servingNetworkName;
        private String authType;
        private String rand;
        private String auts;

        UdmGenerateAuthDataRequest(String servingNetworkName, String authType) {
            this.servingNetworkName = servingNetworkName;
            this.authType = authType;
        }

        UdmGenerateAuthDataRequest(String servingNetworkName, String authType, String rand, String auts) {
            this.servingNetworkName = servingNetworkName;
            this.authType = authType;
            this.rand = rand;
            this.auts = auts;
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

        public String getRand() {
            return rand;
        }

        public void setRand(String rand) {
            this.rand = rand;
        }

        public String getAuts() {
            return auts;
        }

        public void setAuts(String auts) {
            this.auts = auts;
        }
    }

    static class UdmGenerateAuthDataResponse {
        private String supi;
        private String authType;
        private String servingNetworkName;
        private String rand;
        private String autn;
        private String auts;
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

        public String getAuts() {
            return auts;
        }

        public void setAuts(String auts) {
            this.auts = auts;
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

        private NestedAuthVector authVector;

        public NestedAuthVector getAuthVector() {
            return authVector;
        }

        public void setAuthVector(NestedAuthVector authVector) {
            this.authVector = authVector;
        }

        /** Nested authentication-vector used by spec-compliant UDMs (TS 29.503 §6.1.6.2). */
        static class NestedAuthVector {
            private String rand;
            private String autn;
            private String auts;
            private String xresStar;
            private String hxresStar;
            private String kausf;
            private String eapChallenge;

            public String getRand()          { return rand; }
            public void   setRand(String v)  { this.rand = v; }
            public String getAutn()          { return autn; }
            public void   setAutn(String v)  { this.autn = v; }
            public String getAuts()          { return auts; }
            public void   setAuts(String v)  { this.auts = v; }
            public String getXresStar()      { return xresStar; }
            public void   setXresStar(String v) { this.xresStar = v; }
            public String getHxresStar()     { return hxresStar; }
            public void   setHxresStar(String v) { this.hxresStar = v; }
            public String getKausf()         { return kausf; }
            public void   setKausf(String v) { this.kausf = v; }
            public String getEapChallenge()  { return eapChallenge; }
            public void   setEapChallenge(String v) { this.eapChallenge = v; }
        }
    }
}