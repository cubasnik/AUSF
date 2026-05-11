package controlplane

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientShouldRetryTransientServerFailure(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count := atomic.AddInt32(&requests, 1)
		if count == 1 {
			writer.WriteHeader(http.StatusBadGateway)
			_, _ = writer.Write([]byte(`{"message":"temporary failure"}`))
			return
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true,"supi":"imsi-250010000000001","authType":"5G_AKA","servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org","rand":"rand","autn":"autn","hxresStar":"hxres","message":"challenge generated"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)

	response, err := client.Initiate(context.Background(), AuthenticationRequest{SUPI: "imsi-250010000000001", ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org"})
	if err != nil {
		t.Fatalf("Initiate() error = %v", err)
	}

	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
	if response.SUPI != "imsi-250010000000001" {
		t.Fatalf("response supi = %s, want imsi-250010000000001", response.SUPI)
	}
}

func TestClientShouldNotRetryClientFailure(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		atomic.AddInt32(&requests, 1)
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"message":"bad request"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)

	_, err := client.Initiate(context.Background(), AuthenticationRequest{SUPI: "imsi-250010000000001"})
	if err == nil {
		t.Fatal("Initiate() error = nil, want error")
	}

	apiErr, ok := err.(APIError)
	if !ok {
		t.Fatalf("error type = %T, want APIError", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", apiErr.StatusCode, http.StatusBadRequest)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
	_ = fmt.Sprintf("%v", apiErr)
}

func TestClientShouldSendBearerTokenWhenConfigured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer control-plane-token" {
			t.Fatalf("Authorization = %s, want Bearer control-plane-token", got)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true,"supi":"imsi-250010000000001","authType":"5G_AKA","servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org","rand":"rand","autn":"autn","hxresStar":"hxres","message":"challenge generated"}`))
	}))
	defer server.Close()

	client, err := NewClientWithTLSAndBreaker(server.URL, TLSClientConfig{}, defaultBreakerConsecutiveFailures, defaultBreakerTimeout, "control-plane-token")
	if err != nil {
		t.Fatalf("NewClientWithTLSAndBreaker() error = %v", err)
	}

	if _, err := client.Initiate(context.Background(), AuthenticationRequest{SUPI: "imsi-250010000000001", ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org"}); err != nil {
		t.Fatalf("Initiate() error = %v", err)
	}
}

func TestConfirmShouldReturnNotFoundWhenControlPlaneContextIsMissing(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		atomic.AddInt32(&requests, 1)
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"message":"authentication context missing or expired","errorCode":"CONTEXT_NOT_FOUND"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)

	_, err := client.Confirm(context.Background(), "imsi-250010000000001", AuthenticationRequest{ResStar: "deadbeef"})
	if err == nil {
		t.Fatal("Confirm() error = nil, want error")
	}

	apiErr, ok := err.(APIError)
	if !ok {
		t.Fatalf("error type = %T, want APIError", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", apiErr.StatusCode, http.StatusNotFound)
	}
	if apiErr.Message != "authentication context missing or expired" {
		t.Fatalf("message = %s, want authentication context missing or expired", apiErr.Message)
	}
	if apiErr.ErrorCode != "CONTEXT_NOT_FOUND" {
		t.Fatalf("error code = %s, want CONTEXT_NOT_FOUND", apiErr.ErrorCode)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
	_ = fmt.Sprintf("%v", apiErr)
}

func TestInitiateShouldReturnNotFoundWhenSubscriberIsMissing(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		atomic.AddInt32(&requests, 1)
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"message":"subscriber not found in UDM storage","errorCode":"SUBSCRIBER_NOT_FOUND"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)

	_, err := client.Initiate(context.Background(), AuthenticationRequest{SUPI: "imsi-250019999999999", ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org", AuthType: "5G_AKA"})
	if err == nil {
		t.Fatal("Initiate() error = nil, want error")
	}

	apiErr, ok := err.(APIError)
	if !ok {
		t.Fatalf("error type = %T, want APIError", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", apiErr.StatusCode, http.StatusNotFound)
	}
	if apiErr.Message != "subscriber not found in UDM storage" {
		t.Fatalf("message = %s, want subscriber not found in UDM storage", apiErr.Message)
	}
	if apiErr.ErrorCode != "SUBSCRIBER_NOT_FOUND" {
		t.Fatalf("error code = %s, want SUBSCRIBER_NOT_FOUND", apiErr.ErrorCode)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
	_ = fmt.Sprintf("%v", apiErr)
}

func TestConfirmShouldReturnUnauthorizedWhenAuthenticationIsRejected(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		atomic.AddInt32(&requests, 1)
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"message":"RES* verification failed","errorCode":"AUTHENTICATION_REJECTED"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)

	_, err := client.Confirm(context.Background(), "imsi-250010000000001", AuthenticationRequest{ResStar: "deadbeef"})
	if err == nil {
		t.Fatal("Confirm() error = nil, want error")
	}

	apiErr, ok := err.(APIError)
	if !ok {
		t.Fatalf("error type = %T, want APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status code = %d, want %d", apiErr.StatusCode, http.StatusUnauthorized)
	}
	if apiErr.Message != "RES* verification failed" {
		t.Fatalf("message = %s, want RES* verification failed", apiErr.Message)
	}
	if apiErr.ErrorCode != "AUTHENTICATION_REJECTED" {
		t.Fatalf("error code = %s, want AUTHENTICATION_REJECTED", apiErr.ErrorCode)
	}
	if apiErr.EapPayload != "" {
		t.Fatalf("eap payload = %s, want empty", apiErr.EapPayload)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
	_ = fmt.Sprintf("%v", apiErr)
}

func TestConfirmShouldPreserveEapFailurePayloadOnAuthenticationRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"message":"EAP-AKA' verification failed","errorCode":"AUTHENTICATION_REJECTED","eapChallenge":"BAAABA"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)

	_, err := client.Confirm(context.Background(), "auth-eap-1", AuthenticationRequest{EapPayload: "EAP-Response/AKA'-Challenge bad"})
	if err == nil {
		t.Fatal("Confirm() error = nil, want error")
	}

	apiErr, ok := err.(APIError)
	if !ok {
		t.Fatalf("error type = %T, want APIError", err)
	}
	if apiErr.EapPayload != "BAAABA" {
		t.Fatalf("eap payload = %s, want BAAABA", apiErr.EapPayload)
	}
}

func TestClientShouldReturnUnavailableWhenCircuitBreakerIsOpen(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		atomic.AddInt32(&requests, 1)
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte(`{"message":"upstream unavailable"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.breaker = newCircuitBreaker(1, time.Minute)

	_, firstErr := client.Initiate(context.Background(), AuthenticationRequest{SUPI: "imsi-250010000000001", ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org"})
	if firstErr == nil {
		t.Fatal("first Initiate() error = nil, want error")
	}

	_, secondErr := client.Initiate(context.Background(), AuthenticationRequest{SUPI: "imsi-250010000000001", ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org"})
	if secondErr == nil {
		t.Fatal("second Initiate() error = nil, want error")
	}

	apiErr, ok := secondErr.(APIError)
	if !ok {
		t.Fatalf("error type = %T, want APIError", secondErr)
	}
	if apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status code = %d, want %d", apiErr.StatusCode, http.StatusServiceUnavailable)
	}
	if apiErr.ErrorCode != "CONTROL_PLANE_UNAVAILABLE" {
		t.Fatalf("error code = %s, want CONTROL_PLANE_UNAVAILABLE", apiErr.ErrorCode)
	}

	if got := atomic.LoadInt32(&requests); got != maxAttempts {
		t.Fatalf("requests = %d, want %d", got, maxAttempts)
	}
}

func TestClientShouldTrustConfiguredTLSCACertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true,"supi":"imsi-250010000000001","authType":"5G_AKA","servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org","rand":"rand","autn":"autn","hxresStar":"hxres","message":"challenge generated"}`))
	}))
	defer server.Close()

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	caFile, err := os.CreateTemp(t.TempDir(), "ausf-ca-*.pem")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	if _, err := caFile.Write(certPEM); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := caFile.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	client, err := NewClientWithTLSAndBreaker(
		server.URL,
		TLSClientConfig{CACertFile: caFile.Name()},
		defaultBreakerConsecutiveFailures,
		defaultBreakerTimeout,
		"",
	)
	if err != nil {
		t.Fatalf("NewClientWithTLSAndBreaker() error = %v", err)
	}

	response, err := client.Initiate(context.Background(), AuthenticationRequest{SUPI: "imsi-250010000000001", ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org"})
	if err != nil {
		t.Fatalf("Initiate() error = %v", err)
	}
	if response.SUPI != "imsi-250010000000001" {
		t.Fatalf("response supi = %s, want imsi-250010000000001", response.SUPI)
	}
}
