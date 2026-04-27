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
		_, _ = writer.Write([]byte(`{"success":true,"supi":"imsi-250010000000001","authType":"5G_AKA","servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org","rand":"rand","autn":"autn","hxresStar":"hxres","message":"challenge generated"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)

	response, err := client.Initiate(AuthenticationRequest{SUPI: "imsi-250010000000001", ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org"})
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

	_, err := client.Initiate(AuthenticationRequest{SUPI: "imsi-250010000000001"})
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

func TestConfirmShouldReturnNotFoundWhenControlPlaneContextIsMissing(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		atomic.AddInt32(&requests, 1)
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"message":"authentication context missing or expired","errorCode":"CONTEXT_NOT_FOUND"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)

	_, err := client.Confirm("imsi-250010000000001", AuthenticationRequest{ResStar: "deadbeef"})
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

	_, err := client.Initiate(AuthenticationRequest{SUPI: "imsi-250019999999999", ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org", AuthType: "5G_AKA"})
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

	_, err := client.Confirm("imsi-250010000000001", AuthenticationRequest{ResStar: "deadbeef"})
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
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
	_ = fmt.Sprintf("%v", apiErr)
}
