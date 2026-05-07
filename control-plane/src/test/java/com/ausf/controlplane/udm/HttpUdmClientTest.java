package com.ausf.controlplane.udm;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static java.util.Objects.requireNonNull;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.when;
import static org.springframework.http.HttpMethod.POST;
import static org.springframework.test.web.client.match.MockRestRequestMatchers.content;
import static org.springframework.test.web.client.match.MockRestRequestMatchers.method;
import static org.springframework.test.web.client.match.MockRestRequestMatchers.requestTo;
import static org.springframework.test.web.client.response.MockRestResponseCreators.withBadRequest;
import static org.springframework.test.web.client.response.MockRestResponseCreators.withServerError;
import static org.springframework.test.web.client.response.MockRestResponseCreators.withResourceNotFound;
import static org.springframework.test.web.client.response.MockRestResponseCreators.withSuccess;

import com.ausf.controlplane.config.TlsAwareRestClientBuilderCustomizer;
import java.util.Optional;
import org.junit.jupiter.api.Test;
import org.springframework.http.MediaType;
import org.springframework.test.web.client.MockRestServiceServer;
import org.springframework.web.client.RestClient;

class HttpUdmClientTest {
    @Test
    void shouldUseConfiguredBaseUrlWithoutCallingNnrf() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient nnrfClient = mock(NnrfClient.class);
        HttpUdmClient client = new HttpUdmClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), nnrfClient, "http://mock-udm:8090/",
            HttpUdmClient.DEFAULT_BREAKER_FAILURES, HttpUdmClient.DEFAULT_BREAKER_OPEN_SECONDS);

        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/imsi-250010000000001/security-information/generate-auth-data"))
            .andExpect(method(requireNonNull(POST)))
            .andExpect(content().json("""
                {
                  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
                  "authType": "5G_AKA"
                }
                """))
            .andRespond(withSuccess(
                """
                {
                  "supi": "imsi-250010000000001",
                  "authType": "5G_AKA",
                  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
                  "rand": "rand",
                  "autn": "autn",
                                    "auts": "auts",
                  "xresStar": "xres",
                  "hxresStar": "hxres",
                  "kausf": "kausf",
                  "eapChallenge": null
                }
                """,
                MediaType.APPLICATION_JSON
            ));

        Optional<UdmAuthenticationData> result = client.getAuthenticationData(
            "imsi-250010000000001",
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA"
        );

        assertEquals("imsi-250010000000001", result.orElseThrow().getSupi());
        assertEquals("kausf", result.orElseThrow().getAuthenticationVector().getKausf());
        assertEquals("auts", result.orElseThrow().getAuthenticationVector().getAuts());
        verifyNoInteractions(nnrfClient);
        server.verify();
    }

    @Test
    void shouldResolveBaseUrlViaNnrfWhenNotConfigured() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.resolveUdmBaseUrl()).thenReturn(Optional.of("http://mock-udm:8090"));
        HttpUdmClient client = new HttpUdmClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), nnrfClient, "",
            HttpUdmClient.DEFAULT_BREAKER_FAILURES, HttpUdmClient.DEFAULT_BREAKER_OPEN_SECONDS);

        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/imsi-250010000000002/security-information/generate-auth-data"))
            .andExpect(method(requireNonNull(POST)))
            .andRespond(withSuccess(
                """
                {
                  "supi": "imsi-250010000000002",
                  "authType": "EAP_AKA_PRIME",
                  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
                  "rand": "rand2",
                  "autn": "autn2",
                  "xresStar": "xres2",
                  "hxresStar": "hxres2",
                  "kausf": "kausf2",
                  "eapChallenge": "challenge"
                }
                """,
                MediaType.APPLICATION_JSON
            ));

        Optional<UdmAuthenticationData> result = client.getAuthenticationData(
            "imsi-250010000000002",
            "5G:mnc001.mcc001.3gppnetwork.org",
            "EAP_AKA_PRIME"
        );

        assertEquals("challenge", result.orElseThrow().getAuthenticationVector().getEapChallenge());
        server.verify();
    }

    @Test
    void shouldRetryOnTransientUdmFailure() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).ignoreExpectOrder(true).build();
        NnrfClient nnrfClient = mock(NnrfClient.class);
        HttpUdmClient client = new HttpUdmClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), nnrfClient, "http://mock-udm:8090",
            HttpUdmClient.DEFAULT_BREAKER_FAILURES, HttpUdmClient.DEFAULT_BREAKER_OPEN_SECONDS);

        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/imsi-250010000000001/security-information/generate-auth-data"))
            .andExpect(method(requireNonNull(POST)))
            .andRespond(withServerError());
        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/imsi-250010000000001/security-information/generate-auth-data"))
            .andExpect(method(requireNonNull(POST)))
            .andRespond(withSuccess(
                """
                {
                  "supi": "imsi-250010000000001",
                  "authType": "5G_AKA",
                  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
                  "rand": "rand",
                  "autn": "autn",
                  "xresStar": "xres",
                  "hxresStar": "hxres",
                  "kausf": "kausf",
                  "eapChallenge": null
                }
                """,
                MediaType.APPLICATION_JSON
            ));

        Optional<UdmAuthenticationData> result = client.getAuthenticationData(
            "imsi-250010000000001",
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA"
        );

        assertEquals("kausf", result.orElseThrow().getAuthenticationVector().getKausf());
        server.verify();
    }

    @Test
    void shouldRequestResynchronizedAuthenticationDataWithRandAndAuts() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient nnrfClient = mock(NnrfClient.class);
        HttpUdmClient client = new HttpUdmClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), nnrfClient, "http://mock-udm:8090",
            HttpUdmClient.DEFAULT_BREAKER_FAILURES, HttpUdmClient.DEFAULT_BREAKER_OPEN_SECONDS);

        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/imsi-250010000000001/security-information/generate-auth-data"))
            .andExpect(method(requireNonNull(POST)))
            .andExpect(content().json("""
                {
                  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
                  "authType": "5G_AKA",
                  "rand": "rand-1",
                  "auts": "auts-1"
                }
                """))
            .andRespond(withSuccess(
                """
                {
                  "supi": "imsi-250010000000001",
                  "authType": "5G_AKA",
                  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
                  "rand": "rand-2",
                  "autn": "autn-2",
                  "auts": "auts-2",
                  "xresStar": "xres-2",
                  "hxresStar": "hxres-2",
                  "kausf": "kausf-2",
                  "eapChallenge": null
                }
                """,
                MediaType.APPLICATION_JSON
            ));

        Optional<UdmAuthenticationData> result = client.resynchronizeAuthenticationData(
            "imsi-250010000000001",
            "5G:mnc001.mcc001.3gppnetwork.org",
            "5G_AKA",
            "rand-1",
            "auts-1"
        );

        assertEquals("rand-2", result.orElseThrow().getAuthenticationVector().getRand());
        assertEquals("auts-2", result.orElseThrow().getAuthenticationVector().getAuts());
        assertNotEquals("rand-1", result.orElseThrow().getAuthenticationVector().getRand());
        server.verify();
    }

    @Test
    void shouldMapResynchronizationBadRequestToIllegalArgumentException() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient nnrfClient = mock(NnrfClient.class);
        HttpUdmClient client = new HttpUdmClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), nnrfClient, "http://mock-udm:8090",
            HttpUdmClient.DEFAULT_BREAKER_FAILURES, HttpUdmClient.DEFAULT_BREAKER_OPEN_SECONDS);

        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/imsi-250010000000001/security-information/generate-auth-data"))
            .andExpect(method(requireNonNull(POST)))
            .andRespond(withBadRequest()
                .contentType(MediaType.APPLICATION_JSON)
                .body(
                    """
                    {
                      \"detail\": \"AUTS verification failed\",
                      \"cause\": \"AUTHENTICATION_REJECTED\"
                    }
                    """
                ));

        IllegalArgumentException error = assertThrows(IllegalArgumentException.class, () ->
            client.resynchronizeAuthenticationData(
                "imsi-250010000000001",
                "5G:mnc001.mcc001.3gppnetwork.org",
                "5G_AKA",
                "rand-1",
                "auts-1"
            )
        );

        assertEquals("AUTS verification failed", error.getMessage());
        server.verify();
    }

    @Test
    void shouldReturnEmptyOnUdmNotFound() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient nnrfClient = mock(NnrfClient.class);
        HttpUdmClient client = new HttpUdmClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), nnrfClient, "http://mock-udm:8090",
            HttpUdmClient.DEFAULT_BREAKER_FAILURES, HttpUdmClient.DEFAULT_BREAKER_OPEN_SECONDS);

        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/missing/security-information/generate-auth-data"))
            .andExpect(method(requireNonNull(POST)))
            .andRespond(withResourceNotFound());

        assertFalse(client.getAuthenticationData("missing", "5G:mnc001.mcc001.3gppnetwork.org", "5G_AKA").isPresent());
        server.verify();
    }

    @Test
    void shouldFailWhenNeitherConfiguredBaseUrlNorNnrfResolutionExists() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.resolveUdmBaseUrl()).thenReturn(Optional.empty());
        HttpUdmClient client = new HttpUdmClient(RestClient.builder(), TlsAwareRestClientBuilderCustomizer.noop(), nnrfClient, "",
            HttpUdmClient.DEFAULT_BREAKER_FAILURES, HttpUdmClient.DEFAULT_BREAKER_OPEN_SECONDS);

        assertThrows(IllegalStateException.class, () ->
            client.getAuthenticationData("imsi-250010000000003", "5G:mnc001.mcc001.3gppnetwork.org", "5G_AKA")
        );
    }

    @Test
    void shouldOpenCircuitBreakerAfterConsecutiveServerErrors() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).ignoreExpectOrder(true).build();
        NnrfClient nnrfClient = mock(NnrfClient.class);
        // threshold=1 so the breaker opens after a single failure
        HttpUdmClient client = new HttpUdmClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), nnrfClient, "http://mock-udm:8090", 1, 60);

        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/imsi-250010000000001/security-information/generate-auth-data"))
            .andExpect(method(requireNonNull(POST)))
            .andRespond(withServerError());
        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/imsi-250010000000001/security-information/generate-auth-data"))
            .andExpect(method(requireNonNull(POST)))
            .andRespond(withServerError());
        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/imsi-250010000000001/security-information/generate-auth-data"))
            .andExpect(method(requireNonNull(POST)))
            .andRespond(withServerError());

        // First call exhausts retries and opens the breaker
        assertThrows(IllegalStateException.class, () ->
            client.getAuthenticationData("imsi-250010000000001", "5G:mnc001.mcc001.3gppnetwork.org", "5G_AKA")
        );

        assertEquals(UdmCircuitBreaker.State.OPEN, client.circuitBreaker.getState());

        // Second call is rejected immediately without hitting the mock server
        assertThrows(IllegalStateException.class, () ->
            client.getAuthenticationData("imsi-250010000000001", "5G:mnc001.mcc001.3gppnetwork.org", "5G_AKA")
        );

        server.verify();
    }

    @Test
    void shouldNotCountClientErrorsAsBreakerFailures() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).ignoreExpectOrder(true).build();
        NnrfClient nnrfClient = mock(NnrfClient.class);
        // threshold=1 – breaker opens after a single transport/5xx failure
        HttpUdmClient client = new HttpUdmClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), nnrfClient, "http://mock-udm:8090", 1, 60);

        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/missing/security-information/generate-auth-data"))
            .andExpect(method(requireNonNull(POST)))
            .andRespond(withResourceNotFound());

        // 404 should not count as a breaker failure
        assertFalse(client.getAuthenticationData("missing", "5G:mnc001.mcc001.3gppnetwork.org", "5G_AKA").isPresent());

        assertEquals(UdmCircuitBreaker.State.CLOSED, client.circuitBreaker.getState());
        server.verify();
    }

    @Test
    void shouldCloseCircuitBreakerAfterSuccessfulProbe() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).ignoreExpectOrder(true).build();
        NnrfClient nnrfClient = mock(NnrfClient.class);
        HttpUdmClient client = new HttpUdmClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), nnrfClient, "http://mock-udm:8090", 1, 60);

        // Force the breaker open
        client.circuitBreaker.onFailure();
        assertEquals(UdmCircuitBreaker.State.OPEN, client.circuitBreaker.getState());

        // Manually transition to HALF_OPEN to simulate elapsed timeout
        client.circuitBreaker.onSuccess(); // reset to CLOSED first, then re-open at threshold
        // Simulate HALF_OPEN by direct manipulation for test: use package-private fields via sub-test
        // Instead: use a breaker with openDuration=0 so it transitions immediately
        HttpUdmClient probeClient = new HttpUdmClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), nnrfClient, "http://mock-udm:8090", 1, 0);
        probeClient.circuitBreaker.onFailure(); // open it
        // After 0-second open window the probe is allowed; a successful call should close it

        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/imsi-250010000000001/security-information/generate-auth-data"))
            .andExpect(method(requireNonNull(POST)))
            .andRespond(withSuccess(
                """
                {
                  "supi": "imsi-250010000000001",
                  "authType": "5G_AKA",
                  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
                  "rand": "rand",
                  "autn": "autn",
                  "xresStar": "xres",
                  "hxresStar": "hxres",
                  "kausf": "kausf",
                  "eapChallenge": null
                }
                """,
                MediaType.APPLICATION_JSON
            ));

        Optional<UdmAuthenticationData> result = probeClient.getAuthenticationData(
            "imsi-250010000000001", "5G:mnc001.mcc001.3gppnetwork.org", "5G_AKA");

        assertEquals("kausf", result.orElseThrow().getAuthenticationVector().getKausf());
        assertEquals(UdmCircuitBreaker.State.CLOSED, probeClient.circuitBreaker.getState());
        server.verify();
    }
}