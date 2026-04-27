package namf

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestClientShouldRetryTransientFailure(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count := atomic.AddInt32(&requests, 1)
		if count == 1 {
			writer.WriteHeader(http.StatusBadGateway)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL)

	err := client.NotifyUEAuthenticationStatus(UEAuthenticationStatusNotification{AuthCtxID: "auth-1", SUPI: "imsi-250010000000001", AuthResult: "SUCCESS"}, "")
	if err != nil {
		t.Fatalf("NotifyUEAuthenticationStatus() error = %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

func TestClientShouldSkipWhenBaseURLIsBlank(t *testing.T) {
	client := NewClient("")
	if err := client.NotifyUEAuthenticationStatus(UEAuthenticationStatusNotification{AuthCtxID: "auth-1"}, ""); err != nil {
		t.Fatalf("NotifyUEAuthenticationStatus() error = %v", err)
	}
}

func TestClientShouldSendExpectedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/namf-comm/v1/ue-authentications/auth-9/status-notify" {
			t.Fatalf("path = %s, want /namf-comm/v1/ue-authentications/auth-9/status-notify", request.URL.Path)
		}
		var payload UEAuthenticationStatusNotification
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.SUPI != "imsi-250010000000009" {
			t.Fatalf("payload supi = %s, want imsi-250010000000009", payload.SUPI)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.NotifyUEAuthenticationStatus(UEAuthenticationStatusNotification{
		AuthCtxID:  "auth-9",
		SUPI:       "imsi-250010000000009",
		AuthResult: "SUCCESS",
	}, "")
	if err != nil {
		t.Fatalf("NotifyUEAuthenticationStatus() error = %v", err)
	}
}

func TestClientShouldUseNotificationUriTemplateWhenProvided(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/custom/auth-7/callback" {
			t.Fatalf("path = %s, want /custom/auth-7/callback", request.URL.Path)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient("")
	err := client.NotifyUEAuthenticationStatus(UEAuthenticationStatusNotification{
		AuthCtxID:  "auth-7",
		SUPI:       "imsi-250010000000007",
		AuthResult: "SUCCESS",
	}, server.URL+"/custom/{authCtxId}/callback")
	if err != nil {
		t.Fatalf("NotifyUEAuthenticationStatus() error = %v", err)
	}
}
