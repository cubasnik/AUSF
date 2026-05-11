package com.ausf.controlplane.udm;

import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.springframework.web.client.RestClientException;

import java.util.List;
import java.util.Map;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.*;

class NrfLifecycleManagerTest {

    // ── register() ──────────────────────────────────────────────────────────

    @Test
    void shouldRegisterWithNrfOnStartupWhenConfigured() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.isConfigured()).thenReturn(true);

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "test-id-1", 30, "");
        manager.register();

        @SuppressWarnings("unchecked")
        ArgumentCaptor<Map<String, Object>> profileCaptor = ArgumentCaptor.forClass(Map.class);
        verify(nnrfClient).registerNfProfile(eq("test-id-1"), profileCaptor.capture());

        Map<String, Object> profile = profileCaptor.getValue();
        assertThat(profile).containsEntry("nfInstanceId", "test-id-1")
                           .containsEntry("nfType", "AUSF")
                           .containsEntry("nfStatus", "REGISTERED");
        assertThat(manager.isRegistered()).isTrue();
        manager.deregister();
    }

    @Test
    void shouldSkipRegistrationWhenNrfNotConfigured() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.isConfigured()).thenReturn(false);

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "test-id-2", 30, "");
        manager.register();

        verify(nnrfClient, never()).registerNfProfile(any(), any());
        assertThat(manager.isRegistered()).isFalse();
        manager.deregister();
    }

    @Test
    void shouldGenerateUuidWhenNfInstanceIdIsBlank() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.isConfigured()).thenReturn(false);

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "", 30, "");

        assertThat(manager.getNfInstanceId()).isNotBlank();
        assertThat(manager.getNfInstanceId()).matches(
            "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}");
    }

    @Test
    void shouldNotMarkRegisteredWhenNrfThrowsOnStartup() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.isConfigured()).thenReturn(true);
        doThrow(new RestClientException("connection refused"))
            .when(nnrfClient).registerNfProfile(any(), any());

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "test-id-3", 30, "");
        manager.register();

        assertThat(manager.isRegistered()).isFalse();
        manager.deregister();
    }

    // ── deregister() ────────────────────────────────────────────────────────

    @Test
    void shouldDeregisterFromNrfOnShutdown() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.isConfigured()).thenReturn(true);

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "test-id-4", 30, "");
        manager.register();
        manager.deregister();

        verify(nnrfClient).deregister("test-id-4");
    }

    @Test
    void shouldNotCallDeregisterWhenNeverRegistered() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.isConfigured()).thenReturn(false);

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "test-id-5", 30, "");
        manager.register();
        manager.deregister();

        verify(nnrfClient, never()).deregister(any());
    }

    @Test
    void shouldHandleDeregisterFailureGracefully() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.isConfigured()).thenReturn(true);
        doThrow(new RestClientException("timeout"))
            .when(nnrfClient).deregister(any());

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "test-id-6", 30, "");
        manager.register();
        // deregister must not throw
        manager.deregister();
    }

    // ── subscription ────────────────────────────────────────────────────────

    @Test
    void shouldSubscribeOnStartupWhenNotificationUriConfigured() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.isConfigured()).thenReturn(true);
        when(nnrfClient.subscribeNfStatusNotify(any(), any())).thenReturn(Optional.of("subs-1"));

        NrfLifecycleManager manager = new NrfLifecycleManager(
            nnrfClient, "test-id-7", 30, "http://ausf:8081/nnrf-nfm/v1/notification");
        manager.register();

        verify(nnrfClient).subscribeNfStatusNotify(
            eq("http://ausf:8081/nnrf-nfm/v1/notification"),
            eq(List.of("NF_REGISTERED", "NF_DEREGISTERED", "NF_PROFILE_CHANGED"))
        );
        assertThat(manager.getSubscriptionId()).isEqualTo("subs-1");
        manager.deregister();
    }

    @Test
    void shouldUnsubscribeOnShutdown() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.isConfigured()).thenReturn(true);
        when(nnrfClient.subscribeNfStatusNotify(any(), any())).thenReturn(Optional.of("subs-2"));

        NrfLifecycleManager manager = new NrfLifecycleManager(
            nnrfClient, "test-id-8", 30, "http://ausf:8081/nnrf-nfm/v1/notification");
        manager.register();
        manager.deregister();

        verify(nnrfClient).unsubscribeNfStatusNotify("subs-2");
    }

    @Test
    void shouldSkipSubscriptionWhenNotificationUriBlank() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.isConfigured()).thenReturn(true);

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "test-id-9", 30, "");
        manager.register();

        verify(nnrfClient, never()).subscribeNfStatusNotify(any(), any());
        assertThat(manager.getSubscriptionId()).isNull();
        manager.deregister();
    }

    // ── getNfProfile() ───────────────────────────────────────────────────────

    @Test
    void shouldDelegateGetNfProfileToNnrfClient() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.isConfigured()).thenReturn(false);
        Map<String, Object> profile = Map.of("nfInstanceId", "remote-1", "nfType", "UDM");
        when(nnrfClient.getNfProfile("remote-1")).thenReturn(Optional.of(profile));

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "test-id-10", 30, "");
        Optional<Map<String, Object>> result = manager.getNfProfile("remote-1");

        assertThat(result).contains(profile);
    }
}

