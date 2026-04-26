package controlplane

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
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
		_, _ = writer.Write([]byte(`{"success":true,"supi":"imsi-001","authType":"5G_AKA","servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org","rand":"rand","autn":"autn","hxresStar":"hxres","message":"challenge generated"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)

	response, err := client.Initiate(AuthenticationRequest{SUPI: "imsi-001", ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org"})
	if err != nil {
		t.Fatalf("Initiate() error = %v", err)
	}

	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
	if response.SUPI != "imsi-001" {
		t.Fatalf("response supi = %s, want imsi-001", response.SUPI)
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

	_, err := client.Initiate(AuthenticationRequest{SUPI: "imsi-001"})
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
