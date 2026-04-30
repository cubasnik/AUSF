package com.ausf.controlplane.udm;

import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.springframework.web.client.RestClientException;

import java.util.Map;

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

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "test-id-1", 30);
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

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "test-id-2", 30);
        manager.register();

        verify(nnrfClient, never()).registerNfProfile(any(), any());
        assertThat(manager.isRegistered()).isFalse();
        manager.deregister();
    }

    @Test
    void shouldGenerateUuidWhenNfInstanceIdIsBlank() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.isConfigured()).thenReturn(false);

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "", 30);

        assertThat(manager.getNfInstanceId()).isNotBlank();
        // Should be a valid UUID format
        assertThat(manager.getNfInstanceId()).matches(
            "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}");
    }

    @Test
    void shouldNotMarkRegisteredWhenNrfThrowsOnStartup() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.isConfigured()).thenReturn(true);
        doThrow(new RestClientException("connection refused"))
            .when(nnrfClient).registerNfProfile(any(), any());

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "test-id-3", 30);
        manager.register();

        assertThat(manager.isRegistered()).isFalse();
        manager.deregister();
    }

    // ── deregister() ────────────────────────────────────────────────────────

    @Test
    void shouldDeregisterFromNrfOnShutdown() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.isConfigured()).thenReturn(true);

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "test-id-4", 30);
        manager.register();
        manager.deregister();

        verify(nnrfClient).deregister("test-id-4");
    }

    @Test
    void shouldNotCallDeregisterWhenNeverRegistered() {
        NnrfClient nnrfClient = mock(NnrfClient.class);
        when(nnrfClient.isConfigured()).thenReturn(false);

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "test-id-5", 30);
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

        NrfLifecycleManager manager = new NrfLifecycleManager(nnrfClient, "test-id-6", 30);
        manager.register();
        // deregister must not throw
        manager.deregister();
    }
}
