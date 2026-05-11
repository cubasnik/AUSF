package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexey/ausf/microservices/internal/controlplane"
	"github.com/alexey/ausf/microservices/internal/service"
)

type stubControlPlaneClient struct{}

func (stubControlPlaneClient) Initiate(_ context.Context, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
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

func (stubControlPlaneClient) Confirm(_ context.Context, _ string, _ controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (stubControlPlaneClient) Context(_ context.Context, _ string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (stubControlPlaneClient) SoRProtect(_ context.Context, _ string, _ controlplane.SoRProtectionRequest) (controlplane.SoRProtectionResponse, error) {
	return controlplane.SoRProtectionResponse{SoRMacIAUSF: "00000000000000000000000000000000", CounterSoR: "0000"}, nil
}

func (stubControlPlaneClient) UPUProtect(_ context.Context, _ string, _ controlplane.UPUProtectionRequest) (controlplane.UPUProtectionResponse, error) {
	return controlplane.UPUProtectionResponse{UPUMacIAUSF: "00000000000000000000000000000000", CounterUPU: "0000"}, nil
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

func TestRoutesShouldExposePrometheusMetrics(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if !strings.Contains(body, "ausf_http_requests_total") {
		t.Fatalf("metrics body does not contain ausf_http_requests_total")
	}
}

func TestRoutesShouldReturnTraceHeaders(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	traceID := response.Header().Get(traceHeaderName)
	if len(traceID) != 32 {
		t.Fatalf("trace id length = %d, want 32", len(traceID))
	}

	traceparent := response.Header().Get(traceparentHeader)
	if !strings.HasPrefix(traceparent, "00-") {
		t.Fatalf("traceparent = %s, want OpenTelemetry format", traceparent)
	}
}

func TestCreateUEAuthenticationShouldRequireBearerTokenWhenConfigured(t *testing.T) {
	handler := NewHandlerWithAuthorization(service.NewAuthService(stubControlPlaneClient{}, nil), AuthorizationConfig{BearerToken: "secret-token"}).Routes()
	payload := map[string]string{
		"supiOrSuci":         "imsi-250010000000001",
		"servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
		"authType":           "5G_AKA",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if response.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("WWW-Authenticate = %s, want Bearer", response.Header().Get("WWW-Authenticate"))
	}

	var problem ProblemDetails
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if problem.Cause != "UNAUTHORIZED" {
		t.Fatalf("cause = %s, want UNAUTHORIZED", problem.Cause)
	}
}

func TestCreateUEAuthenticationShouldAcceptConfiguredBearerToken(t *testing.T) {
	handler := NewHandlerWithAuthorization(service.NewAuthService(stubControlPlaneClient{}, nil), AuthorizationConfig{BearerToken: "secret-token"}).Routes()
	payload := map[string]string{
		"supiOrSuci":         "imsi-250010000000001",
		"servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
		"authType":           "5G_AKA",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer secret-token")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
}

func TestHealthShouldNotRequireBearerTokenWhenConfigured(t *testing.T) {
	handler := NewHandlerWithAuthorization(service.NewAuthService(stubControlPlaneClient{}, nil), AuthorizationConfig{BearerToken: "secret-token"}).Routes()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestCreateUEAuthenticationShouldRejectMalformedJSON(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()
	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", bytes.NewReader([]byte(`{"supiOrSuci":`)))
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
	if problem.Cause != "MALFORMED_REQUEST" {
		t.Fatalf("cause = %s, want MALFORMED_REQUEST", problem.Cause)
	}
	if problem.Detail != "request body must be valid JSON" {
		t.Fatalf("detail = %s, want request body must be valid JSON", problem.Detail)
	}
}

func TestCreateUEAuthenticationShouldRejectUnsupportedAuthType(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()
	payload := map[string]string{
		"supiOrSuci":         "imsi-250010000000001",
		"servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
		"authType":           "AKA_TLS",
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
	if problem.Cause != "SCHEMA_VALIDATION_FAILED" {
		t.Fatalf("cause = %s, want SCHEMA_VALIDATION_FAILED", problem.Cause)
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

func TestGetAuthContextShouldReturnContextNotFoundCause(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()
	request := httptest.NewRequest(http.MethodGet, "/nausf-auth/v1/ue-authentications/missing", nil)
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
	if problem.Detail != "authentication context not found" {
		t.Fatalf("detail = %s, want authentication context not found", problem.Detail)
	}
}

func TestDeleteAuthContextShouldReturnContextNotFoundCause(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()
	request := httptest.NewRequest(http.MethodDelete, "/nausf-auth/v1/ue-authentications/missing", nil)
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
	if problem.Detail != "authentication context not found" {
		t.Fatalf("detail = %s, want authentication context not found", problem.Detail)
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

func TestFiveGAkaConfirmationShouldRejectEmptyPayloadWithMandatoryIEMissing(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()
	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications/auth-1/5g-aka-confirmation", bytes.NewReader([]byte(`{}`)))
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
	if problem.Cause != "MANDATORY_IE_MISSING" {
		t.Fatalf("cause = %s, want MANDATORY_IE_MISSING", problem.Cause)
	}
	if problem.Detail != "resStar or auts is required" {
		t.Fatalf("detail = %s, want resStar or auts is required", problem.Detail)
	}
}

func TestFiveGAkaConfirmationShouldRejectAmbiguousResStarAndAuts(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()
	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications/auth-1/5g-aka-confirmation", bytes.NewReader([]byte(`{"resStar":"deadbeef","auts":"auts-token"}`)))
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
	if problem.Detail != "exactly one of resStar or auts is required for 5g-aka-confirmation and eapPayload must be omitted" {
		t.Fatalf("detail = %s, want invalid 5G_AKA confirmation payload detail", problem.Detail)
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

type resyncConfirmControlPlaneClient struct{}

func (resyncConfirmControlPlaneClient) Initiate(_ context.Context, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
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

func (resyncConfirmControlPlaneClient) Confirm(_ context.Context, _ string, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	if request.AUTS == "" {
		return controlplane.AuthenticationResponse{}, assertAnError("expected AUTS in confirm request")
	}
	return controlplane.AuthenticationResponse{
		Success:   true,
		RAND:      "rand-2",
		AUTN:      "autn-2",
		HXRESStar: "hxres-2",
		Message:   "re-synchronization challenge generated",
	}, nil
}

func (resyncConfirmControlPlaneClient) Context(_ context.Context, _ string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (resyncConfirmControlPlaneClient) SoRProtect(_ context.Context, _ string, _ controlplane.SoRProtectionRequest) (controlplane.SoRProtectionResponse, error) {
	return controlplane.SoRProtectionResponse{}, nil
}

func (resyncConfirmControlPlaneClient) UPUProtect(_ context.Context, _ string, _ controlplane.UPUProtectionRequest) (controlplane.UPUProtectionResponse, error) {
	return controlplane.UPUProtectionResponse{}, nil
}

func TestFiveGAkaConfirmationShouldAcceptAutsAndReturnSyncFailure(t *testing.T) {
	authService := service.NewAuthService(resyncConfirmControlPlaneClient{}, nil)
	handler := NewHandler(authService).Routes()

	_, err := authService.CreateUEAuthentication(context.Background(), "imsi-250010000000001", "5G:mnc001.mcc001.3gppnetwork.org", "5G_AKA", "")
	if err != nil {
		t.Fatalf("CreateUEAuthentication() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications/auth-1/5g-aka-confirmation", bytes.NewReader([]byte(`{"auts":"auts-token"}`)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}

	var result service.ConfirmationResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if result.AuthResult != "SYNC_FAILURE" {
		t.Fatalf("auth result = %s, want SYNC_FAILURE", result.AuthResult)
	}
	if result.AuthData == nil || result.AuthData.RAND != "rand-2" {
		t.Fatalf("auth data = %#v, want refreshed challenge", result.AuthData)
	}
}

type reauthConfirmControlPlaneClient struct{}

func (reauthConfirmControlPlaneClient) Initiate(_ context.Context, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{
		Success:            true,
		SUPI:               request.SUPI,
		AuthType:           request.AuthType,
		ServingNetworkName: request.ServingNetworkName,
		EapChallenge:       "EAP-Request/AKA'-Challenge initial-token",
	}, nil
}

func (reauthConfirmControlPlaneClient) Confirm(_ context.Context, _ string, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	if request.EapPayload != "EAP-Response/AKA'-Reauthentication token" {
		return controlplane.AuthenticationResponse{}, assertAnError("expected re-authentication payload in confirm request")
	}
	return controlplane.AuthenticationResponse{
		Success:      true,
		RAND:         "rand-2",
		AUTN:         "autn-2",
		HXRESStar:    "hxres-2",
		EapChallenge: "EAP-Request/AKA'-Challenge refreshed-token",
		Message:      "EAP-AKA' re-authentication challenge generated",
	}, nil
}

func (reauthConfirmControlPlaneClient) Context(_ context.Context, _ string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (reauthConfirmControlPlaneClient) SoRProtect(_ context.Context, _ string, _ controlplane.SoRProtectionRequest) (controlplane.SoRProtectionResponse, error) {
	return controlplane.SoRProtectionResponse{}, nil
}

func (reauthConfirmControlPlaneClient) UPUProtect(_ context.Context, _ string, _ controlplane.UPUProtectionRequest) (controlplane.UPUProtectionResponse, error) {
	return controlplane.UPUProtectionResponse{}, nil
}

func TestEapSessionShouldReturnOngoingWithRefreshedChallengeOnReauthentication(t *testing.T) {
	authService := service.NewAuthService(reauthConfirmControlPlaneClient{}, nil)
	handler := NewHandler(authService).Routes()

	_, err := authService.CreateUEAuthentication(context.Background(), "imsi-250010000000002", "5G:mnc001.mcc001.3gppnetwork.org", "EAP_AKA_PRIME", "")
	if err != nil {
		t.Fatalf("CreateUEAuthentication() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications/auth-1/eap-session", bytes.NewReader([]byte(`{"eapPayload":"EAP-Response/AKA'-Reauthentication token"}`)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}

	var result service.ConfirmationResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if result.AuthResult != "ONGOING" {
		t.Fatalf("auth result = %s, want ONGOING", result.AuthResult)
	}
	if result.EapSession == nil || result.EapSession.Payload != "EAP-Request/AKA'-Challenge refreshed-token" {
		t.Fatalf("eap session = %#v, want refreshed challenge", result.EapSession)
	}
}

type fastReauthConfirmControlPlaneClient struct{}

func (fastReauthConfirmControlPlaneClient) Initiate(_ context.Context, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{
		Success:            true,
		SUPI:               request.SUPI,
		AuthType:           request.AuthType,
		ServingNetworkName: request.ServingNetworkName,
		EapChallenge:       "EAP-Request/AKA'-Challenge initial-token",
	}, nil
}

func (fastReauthConfirmControlPlaneClient) Confirm(_ context.Context, _ string, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	if request.EapPayload != "EAP-Response/AKA'-Fast-Reauthentication token" {
		return controlplane.AuthenticationResponse{}, assertAnError("expected fast re-authentication payload in confirm request")
	}
	return controlplane.AuthenticationResponse{
		Success:      true,
		RAND:         "rand-2",
		AUTN:         "autn-2",
		HXRESStar:    "hxres-2",
		EapChallenge: "EAP-Request/AKA'-Challenge refreshed-fast-token",
		Message:      "EAP-AKA' fast re-authentication challenge generated",
	}, nil
}

func (fastReauthConfirmControlPlaneClient) Context(_ context.Context, _ string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (fastReauthConfirmControlPlaneClient) SoRProtect(_ context.Context, _ string, _ controlplane.SoRProtectionRequest) (controlplane.SoRProtectionResponse, error) {
	return controlplane.SoRProtectionResponse{}, nil
}

func (fastReauthConfirmControlPlaneClient) UPUProtect(_ context.Context, _ string, _ controlplane.UPUProtectionRequest) (controlplane.UPUProtectionResponse, error) {
	return controlplane.UPUProtectionResponse{}, nil
}

func TestEapSessionShouldReturnOngoingWithRefreshedChallengeOnFastReauthentication(t *testing.T) {
	authService := service.NewAuthService(fastReauthConfirmControlPlaneClient{}, nil)
	handler := NewHandler(authService).Routes()

	_, err := authService.CreateUEAuthentication(context.Background(), "imsi-250010000000002", "5G:mnc001.mcc001.3gppnetwork.org", "EAP_AKA_PRIME", "")
	if err != nil {
		t.Fatalf("CreateUEAuthentication() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications/auth-1/eap-session", bytes.NewReader([]byte(`{"eapPayload":"EAP-Response/AKA'-Fast-Reauthentication token"}`)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}

	var result service.ConfirmationResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if result.AuthResult != "ONGOING" {
		t.Fatalf("auth result = %s, want ONGOING", result.AuthResult)
	}
	if result.EapSession == nil || result.EapSession.Payload != "EAP-Request/AKA'-Challenge refreshed-fast-token" {
		t.Fatalf("eap session = %#v, want refreshed fast challenge", result.EapSession)
	}
}

type failingConfirmControlPlaneClient struct{}

func (failingConfirmControlPlaneClient) Initiate(_ context.Context, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
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

func (failingConfirmControlPlaneClient) Confirm(_ context.Context, _ string, _ controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, controlplane.APIError{
		StatusCode: http.StatusNotFound,
		Message:    "authentication context missing or expired",
		ErrorCode:  "CONTEXT_NOT_FOUND",
	}
}

func (failingConfirmControlPlaneClient) Context(_ context.Context, _ string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (failingConfirmControlPlaneClient) SoRProtect(_ context.Context, _ string, _ controlplane.SoRProtectionRequest) (controlplane.SoRProtectionResponse, error) {
	return controlplane.SoRProtectionResponse{}, nil
}

func (failingConfirmControlPlaneClient) UPUProtect(_ context.Context, _ string, _ controlplane.UPUProtectionRequest) (controlplane.UPUProtectionResponse, error) {
	return controlplane.UPUProtectionResponse{}, nil
}

type failingInitiateControlPlaneClient struct{}

func (failingInitiateControlPlaneClient) Initiate(_ context.Context, _ controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, controlplane.APIError{
		StatusCode: http.StatusNotFound,
		Message:    "subscriber not found in UDM storage",
		ErrorCode:  "SUBSCRIBER_NOT_FOUND",
	}
}

func (failingInitiateControlPlaneClient) Confirm(_ context.Context, _ string, _ controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (failingInitiateControlPlaneClient) Context(_ context.Context, _ string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (failingInitiateControlPlaneClient) SoRProtect(_ context.Context, _ string, _ controlplane.SoRProtectionRequest) (controlplane.SoRProtectionResponse, error) {
	return controlplane.SoRProtectionResponse{}, nil
}

func (failingInitiateControlPlaneClient) UPUProtect(_ context.Context, _ string, _ controlplane.UPUProtectionRequest) (controlplane.UPUProtectionResponse, error) {
	return controlplane.UPUProtectionResponse{}, nil
}

type unavailableInitiateControlPlaneClient struct{}

func (unavailableInitiateControlPlaneClient) Initiate(_ context.Context, _ controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, assertAnError("dial tcp 127.0.0.1:8081: connect: connection refused")
}

func (unavailableInitiateControlPlaneClient) Confirm(_ context.Context, _ string, _ controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (unavailableInitiateControlPlaneClient) Context(_ context.Context, _ string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (unavailableInitiateControlPlaneClient) SoRProtect(_ context.Context, _ string, _ controlplane.SoRProtectionRequest) (controlplane.SoRProtectionResponse, error) {
	return controlplane.SoRProtectionResponse{}, nil
}

func (unavailableInitiateControlPlaneClient) UPUProtect(_ context.Context, _ string, _ controlplane.UPUProtectionRequest) (controlplane.UPUProtectionResponse, error) {
	return controlplane.UPUProtectionResponse{}, nil
}

type failingRejectedConfirmControlPlaneClient struct{}

func (failingRejectedConfirmControlPlaneClient) Initiate(_ context.Context, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
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

func (failingRejectedConfirmControlPlaneClient) Confirm(_ context.Context, _ string, _ controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, controlplane.APIError{
		StatusCode: http.StatusUnauthorized,
		Message:    "RES* verification failed",
		ErrorCode:  "AUTHENTICATION_REJECTED",
	}
}

func (failingRejectedConfirmControlPlaneClient) Context(_ context.Context, _ string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (failingRejectedConfirmControlPlaneClient) SoRProtect(_ context.Context, _ string, _ controlplane.SoRProtectionRequest) (controlplane.SoRProtectionResponse, error) {
	return controlplane.SoRProtectionResponse{}, nil
}

func (failingRejectedConfirmControlPlaneClient) UPUProtect(_ context.Context, _ string, _ controlplane.UPUProtectionRequest) (controlplane.UPUProtectionResponse, error) {
	return controlplane.UPUProtectionResponse{}, nil
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

	_, err := authService.CreateUEAuthentication(context.Background(), "imsi-250010000000001", "5G:mnc001.mcc001.3gppnetwork.org", "5G_AKA", "")
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

type failingRejectedEapConfirmControlPlaneClient struct{}

func (failingRejectedEapConfirmControlPlaneClient) Initiate(_ context.Context, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{
		Success:            true,
		SUPI:               request.SUPI,
		AuthType:           request.AuthType,
		ServingNetworkName: request.ServingNetworkName,
		EapChallenge:       "EAP-Request/AKA'-Challenge token",
	}, nil
}

func (failingRejectedEapConfirmControlPlaneClient) Confirm(_ context.Context, _ string, _ controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, controlplane.APIError{
		StatusCode: http.StatusUnauthorized,
		Message:    "EAP-AKA' verification failed",
		ErrorCode:  "AUTHENTICATION_REJECTED",
		EapPayload: "EAP-Failure",
	}
}

func (failingRejectedEapConfirmControlPlaneClient) Context(_ context.Context, _ string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (failingRejectedEapConfirmControlPlaneClient) SoRProtect(_ context.Context, _ string, _ controlplane.SoRProtectionRequest) (controlplane.SoRProtectionResponse, error) {
	return controlplane.SoRProtectionResponse{}, nil
}

func (failingRejectedEapConfirmControlPlaneClient) UPUProtect(_ context.Context, _ string, _ controlplane.UPUProtectionRequest) (controlplane.UPUProtectionResponse, error) {
	return controlplane.UPUProtectionResponse{}, nil
}

func TestEapSessionShouldPropagateEapFailurePayload(t *testing.T) {
	authService := service.NewAuthService(failingRejectedEapConfirmControlPlaneClient{}, nil)
	handler := NewHandler(authService).Routes()

	_, err := authService.CreateUEAuthentication(context.Background(), "imsi-250010000000002", "5G:mnc001.mcc001.3gppnetwork.org", "EAP_AKA_PRIME", "")
	if err != nil {
		t.Fatalf("CreateUEAuthentication() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications/auth-1/eap-session", bytes.NewReader([]byte(`{"eapPayload":"EAP-Response/AKA'-Challenge bad"}`)))
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
	if problem.EapPayload != "EAP-Failure" {
		t.Fatalf("eap payload = %s, want EAP-Failure", problem.EapPayload)
	}
}

func TestFiveGAkaConfirmationShouldPropagateControlPlaneNotFoundCause(t *testing.T) {
	authService := service.NewAuthService(failingConfirmControlPlaneClient{}, nil)
	handler := NewHandler(authService).Routes()

	_, err := authService.CreateUEAuthentication(context.Background(), "imsi-250010000000001", "5G:mnc001.mcc001.3gppnetwork.org", "5G_AKA", "")
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
