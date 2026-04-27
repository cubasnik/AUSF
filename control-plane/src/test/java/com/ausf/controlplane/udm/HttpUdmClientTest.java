package com.ausf.controlplane.udm;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.when;
import static org.springframework.http.HttpMethod.POST;
import static org.springframework.test.web.client.match.MockRestRequestMatchers.content;
import static org.springframework.test.web.client.match.MockRestRequestMatchers.method;
import static org.springframework.test.web.client.match.MockRestRequestMatchers.requestTo;
import static org.springframework.test.web.client.response.MockRestResponseCreators.withServerError;
import static org.springframework.test.web.client.response.MockRestResponseCreators.withResourceNotFound;
import static org.springframework.test.web.client.response.MockRestResponseCreators.withSuccess;

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
        HttpUdmClient client = new HttpUdmClient(builder, nnrfClient, "http://mock-udm:8090/");

        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/imsi-250010000000001/security-information/generate-auth-data"))
            .andExpect(method(POST))
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
        verifyNoInteractions(nnrfClient);
        server.verify();
    }

    @Test
    void shouldResolveBaseUrlViaNnrfWhenNotConfigured() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.resolveUdmBaseUrl()).thenReturn(Optional.of("http://mock-udm:8090"));
        HttpUdmClient client = new HttpUdmClient(builder, nnrfClient, "");

        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/imsi-250010000000002/security-information/generate-auth-data"))
            .andExpect(method(POST))
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
        HttpUdmClient client = new HttpUdmClient(builder, nnrfClient, "http://mock-udm:8090");

        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/imsi-250010000000001/security-information/generate-auth-data"))
            .andExpect(method(POST))
            .andRespond(withServerError());
        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/imsi-250010000000001/security-information/generate-auth-data"))
            .andExpect(method(POST))
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
    void shouldReturnEmptyOnUdmNotFound() {
        RestClient.Builder builder = RestClient.builder();
        MockRestServiceServer server = MockRestServiceServer.bindTo(builder).build();
        NnrfClient nnrfClient = mock(NnrfClient.class);
        HttpUdmClient client = new HttpUdmClient(builder, nnrfClient, "http://mock-udm:8090");

        server.expect(requestTo("http://mock-udm:8090/nudm-ueau/v1/missing/security-information/generate-auth-data"))
            .andExpect(method(POST))
            .andRespond(withResourceNotFound());

        assertFalse(client.getAuthenticationData("missing", "5G:mnc001.mcc001.3gppnetwork.org", "5G_AKA").isPresent());
        server.verify();
    }

    @Test
    void shouldFailWhenNeitherConfiguredBaseUrlNorNnrfResolutionExists() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.resolveUdmBaseUrl()).thenReturn(Optional.empty());
        HttpUdmClient client = new HttpUdmClient(RestClient.builder(), nnrfClient, "");

        assertThrows(IllegalStateException.class, () ->
            client.getAuthenticationData("imsi-250010000000003", "5G:mnc001.mcc001.3gppnetwork.org", "5G_AKA")
        );
    }
}