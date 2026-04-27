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
