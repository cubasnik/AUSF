package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexey/ausf/microservices/internal/controlplane"
	"github.com/alexey/ausf/microservices/internal/service"
)

type stubControlPlaneClient struct{}

func (stubControlPlaneClient) Initiate(request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{
		Success:            true,
		SUPI:               request.SUPI,
		AuthType:           "5G_AKA",
		ServingNetworkName: request.ServingNetworkName,
		RAND:               "rand",
		AUTN:               "autn",
		HXRESStar:          "hxres",
	}, nil
}

func (stubControlPlaneClient) Confirm(string, controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (stubControlPlaneClient) Context(string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func TestCreateUEAuthenticationShouldRejectInvalidNotificationURI(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()
	payload := map[string]string{
		"supiOrSuci":         "imsi-250010000000001",
		"servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
		"authType":           "5G_AKA",
		"notificationUri":    "/relative/callback",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}

	var problem ProblemDetails
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if problem.Cause != "INVALID_NOTIFICATION_URI" {
		t.Fatalf("cause = %s, want INVALID_NOTIFICATION_URI", problem.Cause)
	}
}

func TestCreateUEAuthenticationShouldAcceptAbsoluteNotificationURI(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()
	payload := map[string]string{
		"supiOrSuci":         "imsi-250010000000001",
		"servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
		"authType":           "5G_AKA",
		"notificationUri":    "http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusCreated, response.Body.String())
	}

	var context service.AuthContext
	if err := json.Unmarshal(response.Body.Bytes(), &context); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if context.NotificationURI != payload["notificationUri"] {
		t.Fatalf("notification uri = %s, want %s", context.NotificationURI, payload["notificationUri"])
	}
}

func TestAuthContextRoutesShouldAcceptEapSessionSubresource(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()
	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications/auth-eap/eap-session", bytes.NewReader([]byte(`{"eapPayload":"EAP-Response/AKA'-Challenge token"}`)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d when context is missing but route is recognized", response.Code, http.StatusNotFound)
	}

	var problem ProblemDetails
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if problem.Cause != "CONTEXT_NOT_FOUND" {
		t.Fatalf("cause = %s, want CONTEXT_NOT_FOUND", problem.Cause)
	}
}

func TestFiveGAkaConfirmationShouldRejectEapPayload(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()
	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications/auth-1/5g-aka-confirmation", bytes.NewReader([]byte(`{"eapPayload":"EAP-Response/AKA'-Challenge token"}`)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}

	var problem ProblemDetails
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if problem.Cause != "INVALID_CONFIRMATION_PAYLOAD" {
		t.Fatalf("cause = %s, want INVALID_CONFIRMATION_PAYLOAD", problem.Cause)
	}
}

func TestEapSessionShouldRejectResStar(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()
	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications/auth-eap/eap-session", bytes.NewReader([]byte(`{"resStar":"deadbeef"}`)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}

	var problem ProblemDetails
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if problem.Cause != "INVALID_CONFIRMATION_PAYLOAD" {
		t.Fatalf("cause = %s, want INVALID_CONFIRMATION_PAYLOAD", problem.Cause)
	}
}

type failingConfirmControlPlaneClient struct{}

func (failingConfirmControlPlaneClient) Initiate(request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{
		Success:            true,
		SUPI:               request.SUPI,
		AuthType:           request.AuthType,
		ServingNetworkName: request.ServingNetworkName,
		RAND:               "rand",
		AUTN:               "autn",
		HXRESStar:          "hxres",
	}, nil
}

func (failingConfirmControlPlaneClient) Confirm(string, controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, controlplane.APIError{
		StatusCode: http.StatusNotFound,
		Message:    "authentication context missing or expired",
		ErrorCode:  "CONTEXT_NOT_FOUND",
	}
}

func (failingConfirmControlPlaneClient) Context(string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

type failingInitiateControlPlaneClient struct{}

func (failingInitiateControlPlaneClient) Initiate(controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, controlplane.APIError{
		StatusCode: http.StatusNotFound,
		Message:    "subscriber not found in UDM storage",
		ErrorCode:  "SUBSCRIBER_NOT_FOUND",
	}
}

func (failingInitiateControlPlaneClient) Confirm(string, controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (failingInitiateControlPlaneClient) Context(string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

type unavailableInitiateControlPlaneClient struct{}

func (unavailableInitiateControlPlaneClient) Initiate(controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, assertAnError("dial tcp 127.0.0.1:8081: connect: connection refused")
}

func (unavailableInitiateControlPlaneClient) Confirm(string, controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (unavailableInitiateControlPlaneClient) Context(string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

type failingRejectedConfirmControlPlaneClient struct{}

func (failingRejectedConfirmControlPlaneClient) Initiate(request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{
		Success:            true,
		SUPI:               request.SUPI,
		AuthType:           request.AuthType,
		ServingNetworkName: request.ServingNetworkName,
		RAND:               "rand",
		AUTN:               "autn",
		HXRESStar:          "hxres",
	}, nil
}

func (failingRejectedConfirmControlPlaneClient) Confirm(string, controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, controlplane.APIError{
		StatusCode: http.StatusUnauthorized,
		Message:    "RES* verification failed",
		ErrorCode:  "AUTHENTICATION_REJECTED",
	}
}

func (failingRejectedConfirmControlPlaneClient) Context(string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

type assertAnError string

func (error assertAnError) Error() string {
	return string(error)
}

func TestCreateUEAuthenticationShouldPropagateSubscriberNotFoundCause(t *testing.T) {
	handler := NewHandler(service.NewAuthService(failingInitiateControlPlaneClient{}, nil)).Routes()
	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", bytes.NewReader([]byte(`{"supiOrSuci":"imsi-250019999999999","servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org","authType":"5G_AKA"}`)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}

	var problem ProblemDetails
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if problem.Cause != "SUBSCRIBER_NOT_FOUND" {
		t.Fatalf("cause = %s, want SUBSCRIBER_NOT_FOUND", problem.Cause)
	}
	if problem.Detail != "subscriber not found in UDM storage" {
		t.Fatalf("detail = %s, want subscriber not found in UDM storage", problem.Detail)
	}
}

func TestCreateUEAuthenticationShouldReturnControlPlaneUnavailableWhenInitiateFails(t *testing.T) {
	handler := NewHandler(service.NewAuthService(unavailableInitiateControlPlaneClient{}, nil)).Routes()
	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", bytes.NewReader([]byte(`{"supiOrSuci":"imsi-250010000000001","servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org","authType":"5G_AKA"}`)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadGateway)
	}

	var problem ProblemDetails
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if problem.Cause != "CONTROL_PLANE_UNAVAILABLE" {
		t.Fatalf("cause = %s, want CONTROL_PLANE_UNAVAILABLE", problem.Cause)
	}
	if problem.Detail != "control-plane initiate failed: dial tcp 127.0.0.1:8081: connect: connection refused" {
		t.Fatalf("detail = %s, want propagated control-plane unavailable detail", problem.Detail)
	}
}

func TestFiveGAkaConfirmationShouldPropagateAuthenticationRejectedCause(t *testing.T) {
	authService := service.NewAuthService(failingRejectedConfirmControlPlaneClient{}, nil)
	handler := NewHandler(authService).Routes()

	_, err := authService.CreateUEAuthentication("imsi-250010000000001", "5G:mnc001.mcc001.3gppnetwork.org", "5G_AKA", "")
	if err != nil {
		t.Fatalf("CreateUEAuthentication() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications/auth-1/5g-aka-confirmation", bytes.NewReader([]byte(`{"resStar":"deadbeef"}`)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	var problem ProblemDetails
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if problem.Cause != "AUTHENTICATION_REJECTED" {
		t.Fatalf("cause = %s, want AUTHENTICATION_REJECTED", problem.Cause)
	}
	if problem.Detail != "RES* verification failed" {
		t.Fatalf("detail = %s, want RES* verification failed", problem.Detail)
	}
}

func TestFiveGAkaConfirmationShouldPropagateControlPlaneNotFoundCause(t *testing.T) {
	authService := service.NewAuthService(failingConfirmControlPlaneClient{}, nil)
	handler := NewHandler(authService).Routes()

	_, err := authService.CreateUEAuthentication("imsi-250010000000001", "5G:mnc001.mcc001.3gppnetwork.org", "5G_AKA", "")
	if err != nil {
		t.Fatalf("CreateUEAuthentication() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications/auth-1/5g-aka-confirmation", bytes.NewReader([]byte(`{"resStar":"deadbeef"}`)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}

	var problem ProblemDetails
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if problem.Cause != "CONTEXT_NOT_FOUND" {
		t.Fatalf("cause = %s, want CONTEXT_NOT_FOUND", problem.Cause)
	}
	if problem.Detail != "authentication context missing or expired" {
		t.Fatalf("detail = %s, want authentication context missing or expired", problem.Detail)
	}
}
