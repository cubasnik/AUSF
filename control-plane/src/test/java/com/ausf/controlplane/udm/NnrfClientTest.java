package com.ausf.controlplane.udm;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static java.util.Objects.requireNonNull;
import static org.springframework.http.HttpMethod.DELETE;
import static org.springframework.http.HttpMethod.GET;
import static org.springframework.http.HttpMethod.POST;
import static org.springframework.test.web.client.match.MockRestRequestMatchers.method;
import static org.springframework.test.web.client.match.MockRestRequestMatchers.requestTo;
import static org.springframework.test.web.client.response.MockRestResponseCreators.withNoContent;
import static org.springframework.test.web.client.response.MockRestResponseCreators.withServerError;
import static org.springframework.test.web.client.response.MockRestResponseCreators.withStatus;
import static org.springframework.test.web.client.response.MockRestResponseCreators.withSuccess;

import com.ausf.controlplane.config.TlsAwareRestClientBuilderCustomizer;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
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

class NnrfClientGetNfProfileTest {

    @Test
    void shouldReturnNfProfileWhenFound() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient client = new NnrfClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");

        server.expect(requestTo("http://mock-nrf:8091/nnrf-nfm/v1/nf-instances/remote-udm-1"))
            .andExpect(method(requireNonNull(GET)))
            .andRespond(withSuccess(
                """
                {"nfInstanceId":"remote-udm-1","nfType":"UDM","nfStatus":"REGISTERED"}
                """,
                MediaType.APPLICATION_JSON
            ));

        Optional<Map<String, Object>> result = client.getNfProfile("remote-udm-1");

        assertTrue(result.isPresent());
        assertEquals("remote-udm-1", result.get().get("nfInstanceId"));
        server.verify();
    }

    @Test
    void shouldReturnEmptyWhenNfNotFound() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient client = new NnrfClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");

        server.expect(requestTo("http://mock-nrf:8091/nnrf-nfm/v1/nf-instances/unknown"))
            .andExpect(method(requireNonNull(GET)))
            .andRespond(withStatus(HttpStatus.NOT_FOUND));

        Optional<Map<String, Object>> result = client.getNfProfile("unknown");

        assertFalse(result.isPresent());
        server.verify();
    }

    @Test
    void shouldReturnEmptyWhenNrfNotConfigured() {
        NnrfClient client = new NnrfClient(RestClient.builder(), TlsAwareRestClientBuilderCustomizer.noop(), "");
        assertFalse(client.getNfProfile("any").isPresent());
    }
}

class NnrfClientSubscriptionTest {

    @Test
    void shouldSubscribeAndReturnSubsId() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient client = new NnrfClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");

        server.expect(requestTo("http://mock-nrf:8091/nnrf-nfm/v1/subscriptions"))
            .andExpect(method(requireNonNull(POST)))
            .andRespond(withStatus(HttpStatus.CREATED)
                .header("Location", "http://mock-nrf:8091/nnrf-nfm/v1/subscriptions/sub-abc")
                .body("{\"subsId\":\"sub-abc\"}")
                .contentType(MediaType.APPLICATION_JSON));

        Optional<String> subsId = client.subscribeNfStatusNotify(
            "http://ausf:8081/nnrf-nfm/v1/notification",
            List.of("NF_REGISTERED", "NF_DEREGISTERED")
        );

        assertTrue(subsId.isPresent());
        assertEquals("sub-abc", subsId.get());
        server.verify();
    }

    @Test
    void shouldReturnEmptyWhenNotificationUriBlank() {
        NnrfClient client = new NnrfClient(RestClient.builder(), TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");
        assertFalse(client.subscribeNfStatusNotify("", List.of("NF_REGISTERED")).isPresent());
    }

    @Test
    void shouldReturnEmptyWhenNrfNotConfigured() {
        NnrfClient client = new NnrfClient(RestClient.builder(), TlsAwareRestClientBuilderCustomizer.noop(), "");
        assertFalse(client.subscribeNfStatusNotify("http://ausf/notification", List.of("NF_REGISTERED")).isPresent());
    }

    @Test
    void shouldUnsubscribe() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient client = new NnrfClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");

        server.expect(requestTo("http://mock-nrf:8091/nnrf-nfm/v1/subscriptions/sub-xyz"))
            .andExpect(method(requireNonNull(DELETE)))
            .andRespond(withNoContent());

        client.unsubscribeNfStatusNotify("sub-xyz");
        server.verify();
    }

    @Test
    void shouldSilentlyIgnoreUnsubscribeWhenSubsIdBlank() {
        NnrfClient client = new NnrfClient(RestClient.builder(), TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");
        // must not throw
        client.unsubscribeNfStatusNotify("");
        client.unsubscribeNfStatusNotify(null);
    }
}

class NnrfClientGenericDiscoveryTest {

    @Test
    void shouldResolveServiceUrlUsingNfServicesField() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient client = new NnrfClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");

        server.expect(requestTo("http://mock-nrf:8091/nnrf-disc/v1/nf-instances?target-nf-type=UDM&requester-nf-type=AUSF"))
            .andExpect(method(requireNonNull(GET)))
            .andRespond(withSuccess(
                """
                {
                  "nfInstances": [{
                    "nfInstanceId": "udm-1", "nfType": "UDM", "nfStatus": "REGISTERED",
                    "nfServices": [{"serviceName": "nudm-ueau", "apiPrefix": "http://udm:8090/"}]
                  }]
                }
                """,
                MediaType.APPLICATION_JSON
            ));

        Optional<String> result = client.resolveServiceUrl("UDM", "nudm-ueau");

        assertEquals(Optional.of("http://udm:8090"), result);
        server.verify();
    }

    @Test
    void shouldResolveServiceUrlFallingBackToServicesField() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient client = new NnrfClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");

        server.expect(requestTo("http://mock-nrf:8091/nnrf-disc/v1/nf-instances?target-nf-type=UDM&requester-nf-type=AUSF"))
            .andExpect(method(requireNonNull(GET)))
            .andRespond(withSuccess(
                """
                {
                  "nfInstances": [{
                    "services": [{"serviceName": "nudm-ueau", "apiPrefix": "http://udm-legacy:8090"}]
                  }]
                }
                """,
                MediaType.APPLICATION_JSON
            ));

        Optional<String> result = client.resolveServiceUrl("UDM", "nudm-ueau");

        assertEquals(Optional.of("http://udm-legacy:8090"), result);
        server.verify();
    }

    @Test
    void shouldReturnEmptyServiceUrlWhenNrfNotConfigured() {
        NnrfClient client = new NnrfClient(RestClient.builder(), TlsAwareRestClientBuilderCustomizer.noop(), "");
        assertFalse(client.resolveServiceUrl("UDM", "nudm-ueau").isPresent());
    }

    @Test
    void shouldDiscoverNfInstancesWithEnrichedFields() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient client = new NnrfClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");

        server.expect(requestTo("http://mock-nrf:8091/nnrf-disc/v1/nf-instances?target-nf-type=UDM&requester-nf-type=AUSF"))
            .andExpect(method(requireNonNull(GET)))
            .andRespond(withSuccess(
                """
                {"nfInstances": [
                  {"nfInstanceId": "udm-1", "nfType": "UDM", "nfStatus": "REGISTERED"}
                ]}
                """,
                MediaType.APPLICATION_JSON
            ));

        List<NnrfClient.NfInstance> instances = client.discoverNfInstances("UDM");

        assertEquals(1, instances.size());
        assertEquals("udm-1", instances.get(0).getNfInstanceId());
        assertEquals("UDM", instances.get(0).getNfType());
        assertEquals("REGISTERED", instances.get(0).getNfStatus());
        server.verify();
    }

    @Test
    void shouldReturnEmptyListWhenNoMatchingInstances() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient client = new NnrfClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");

        server.expect(requestTo("http://mock-nrf:8091/nnrf-disc/v1/nf-instances?target-nf-type=AMF&requester-nf-type=AUSF"))
            .andExpect(method(requireNonNull(GET)))
            .andRespond(withSuccess(
                """
                {"nfInstances":[]}
                """, MediaType.APPLICATION_JSON));

        List<NnrfClient.NfInstance> instances = client.discoverNfInstances("AMF");

        assertTrue(instances.isEmpty());
        server.verify();
    }

    @Test
    void shouldReturnEmptyListWhenDiscoveryNotConfigured() {
        NnrfClient client = new NnrfClient(RestClient.builder(), TlsAwareRestClientBuilderCustomizer.noop(), "");
        assertTrue(client.discoverNfInstances("UDM").isEmpty());
    }
}

class NnrfClientDiscoverySubscriptionTest {

    @Test
    void shouldSubscribeNfDiscoveryAndReturnSubsId() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient client = new NnrfClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");

        server.expect(requestTo("http://mock-nrf:8091/nnrf-disc/v1/subscriptions"))
            .andExpect(method(requireNonNull(POST)))
            .andRespond(withStatus(HttpStatus.CREATED)
                .header("Location", "http://mock-nrf:8091/nnrf-disc/v1/subscriptions/disc-sub-1"));

        Optional<String> subsId = client.subscribeNfDiscovery(
            "http://ausf:8081/nnrf-disc/v1/notification",
            List.of("UDM")
        );

        assertEquals(Optional.of("disc-sub-1"), subsId);
        server.verify();
    }

    @Test
    void shouldReturnEmptyWhenCallbackUriBlankForDiscSubs() {
        NnrfClient client = new NnrfClient(RestClient.builder(), TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");
        assertFalse(client.subscribeNfDiscovery("", List.of("UDM")).isPresent());
    }

    @Test
    void shouldReturnEmptyWhenNrfNotConfiguredForDiscSubs() {
        NnrfClient client = new NnrfClient(RestClient.builder(), TlsAwareRestClientBuilderCustomizer.noop(), "");
        assertFalse(client.subscribeNfDiscovery("http://ausf/notify", List.of("UDM")).isPresent());
    }

    @Test
    void shouldUnsubscribeNfDiscovery() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient client = new NnrfClient(builder, TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");

        server.expect(requestTo("http://mock-nrf:8091/nnrf-disc/v1/subscriptions/disc-sub-2"))
            .andExpect(method(requireNonNull(DELETE)))
            .andRespond(withNoContent());

        client.unsubscribeNfDiscovery("disc-sub-2");
        server.verify();
    }

    @Test
    void shouldSilentlyIgnoreUnsubscribeDiscWhenBlank() {
        NnrfClient client = new NnrfClient(RestClient.builder(), TlsAwareRestClientBuilderCustomizer.noop(), "http://mock-nrf:8091");
        // must not throw
        client.unsubscribeNfDiscovery("");
        client.unsubscribeNfDiscovery(null);
    }
}
