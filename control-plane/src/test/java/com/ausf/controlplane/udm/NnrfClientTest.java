package com.ausf.controlplane.udm;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static java.util.Objects.requireNonNull;
import static org.springframework.http.HttpMethod.GET;
import static org.springframework.test.web.client.match.MockRestRequestMatchers.method;
import static org.springframework.test.web.client.match.MockRestRequestMatchers.requestTo;
import static org.springframework.test.web.client.response.MockRestResponseCreators.withServerError;
import static org.springframework.test.web.client.response.MockRestResponseCreators.withSuccess;

import com.ausf.controlplane.config.TlsAwareRestClientBuilderCustomizer;
import java.util.Optional;
import org.junit.jupiter.api.Test;
import org.springframework.http.MediaType;
import org.springframework.test.web.client.MockRestServiceServer;
import org.springframework.web.client.RestClient;

class NnrfClientTest {
    @Test
    void shouldResolveUdmBaseUrlFromDiscoveryResponse() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
      NnrfClient client = new NnrfClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091/");

        server.expect(requestTo("http://mock-nrf:8091/nnrf-disc/v1/nf-instances?target-nf-type=UDM&requester-nf-type=AUSF"))
          .andExpect(method(requireNonNull(GET)))
            .andRespond(withSuccess(
                """
                {
                  "nfInstances": [
                    {
                      "services": [
                        {"serviceName": "nudm-ueau", "apiPrefix": "http://mock-udm:8090/"}
                      ]
                    }
                  ]
                }
                """,
                MediaType.APPLICATION_JSON
            ));

        Optional<String> result = client.resolveUdmBaseUrl();

        assertEquals(Optional.of("http://mock-udm:8090"), result);
        server.verify();
    }

    @Test
    void shouldReturnEmptyWhenNnrfBaseUrlIsNotConfigured() {
      NnrfClient client = new NnrfClient(RestClient.builder(), TlsAwareRestClientBuilderCustomizer.noop(), "");

        assertFalse(client.resolveUdmBaseUrl().isPresent());
    }

    @Test
    void shouldWrapDiscoveryFailures() {
        RestClient.Builder builder = RestClient.builder();
      MockRestServiceServer server = MockRestServiceServer.bindTo(builder).ignoreExpectOrder(true).build();
        NnrfClient client = new NnrfClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");

        server.expect(requestTo("http://mock-nrf:8091/nnrf-disc/v1/nf-instances?target-nf-type=UDM&requester-nf-type=AUSF"))
            .andExpect(method(requireNonNull(GET)))
            .andRespond(withServerError());
      server.expect(requestTo("http://mock-nrf:8091/nnrf-disc/v1/nf-instances?target-nf-type=UDM&requester-nf-type=AUSF"))
        .andExpect(method(requireNonNull(GET)))
        .andRespond(withServerError());
      server.expect(requestTo("http://mock-nrf:8091/nnrf-disc/v1/nf-instances?target-nf-type=UDM&requester-nf-type=AUSF"))
        .andExpect(method(requireNonNull(GET)))
        .andRespond(withServerError());

        assertThrows(IllegalStateException.class, client::resolveUdmBaseUrl);
        server.verify();
    }

    @Test
    void shouldRetryTransientDiscoveryFailure() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).ignoreExpectOrder(true).build();
      NnrfClient client = new NnrfClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");

        server.expect(requestTo("http://mock-nrf:8091/nnrf-disc/v1/nf-instances?target-nf-type=UDM&requester-nf-type=AUSF"))
          .andExpect(method(requireNonNull(GET)))
            .andRespond(withServerError());
        server.expect(requestTo("http://mock-nrf:8091/nnrf-disc/v1/nf-instances?target-nf-type=UDM&requester-nf-type=AUSF"))
          .andExpect(method(requireNonNull(GET)))
            .andRespond(withSuccess(
                """
                {
                  "nfInstances": [
                    {
                      "services": [
                        {"serviceName": "nudm-ueau", "apiPrefix": "http://mock-udm:8090/"}
                      ]
                    }
                  ]
                }
                """,
                MediaType.APPLICATION_JSON
            ));

        assertEquals(Optional.of("http://mock-udm:8090"), client.resolveUdmBaseUrl());
        server.verify();
    }
}