package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/alexey/ausf/microservices/internal/controlplane"
	"github.com/alexey/ausf/microservices/internal/namf"
)

type stubControlPlaneClient struct {
	initiateResponse controlplane.AuthenticationResponse
	initiateErr      error
	confirmResponse  controlplane.AuthenticationResponse
	confirmErr       error
}

func (client stubControlPlaneClient) Initiate(request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	if client.initiateErr != nil {
		return controlplane.AuthenticationResponse{}, client.initiateErr
	}
	if client.initiateResponse.SUPI != "" {
		return client.initiateResponse, nil
	}
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

func (client stubControlPlaneClient) Confirm(supi string, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	if client.confirmErr != nil {
		return controlplane.AuthenticationResponse{}, client.confirmErr
	}
	if client.confirmResponse.Success || client.confirmResponse.KSEAF != "" || client.confirmResponse.Message != "" {
		return client.confirmResponse, nil
	}
	return controlplane.AuthenticationResponse{Success: true, Message: "authentication successful"}, nil
}

func (client stubControlPlaneClient) Context(supi string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

type stubNamfClient struct {
	notifications []namf.UEAuthenticationStatusNotification
	uris          []string
}

func (client *stubNamfClient) NotifyUEAuthenticationStatus(notification namf.UEAuthenticationStatusNotification, notificationURI string) error {
	client.notifications = append(client.notifications, notification)
	client.uris = append(client.uris, notificationURI)
	return nil
}

func TestConfirmShouldNotifyNamfOnSuccessfulAuthentication(t *testing.T) {
	namfClient := &stubNamfClient{}
	authService := NewAuthService(stubControlPlaneClient{
		confirmResponse: controlplane.AuthenticationResponse{
			Success: true,
			KSEAF:   "kseaf-1",
			Message: "authentication successful",
		},
	}, namfClient)
	authService.contexts["auth-1"] = AuthContext{
		AuthCtxID:          "auth-1",
		SUPI:               "imsi-250010000000001",
		AuthType:           "5G_AKA",
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		NotificationURI:    "http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify",
		Status:             "CHALLENGE_SENT",
	}

	result, err := authService.Confirm("auth-1", "res-star", "")
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}

	if result.AuthResult != "SUCCESS" {
		t.Fatalf("Confirm() auth result = %s, want SUCCESS", result.AuthResult)
	}
	storedContext, err := authService.Lookup("auth-1")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if storedContext.Status != authStatusAuthenticated {
		t.Fatalf("stored status = %s, want %s", storedContext.Status, authStatusAuthenticated)
	}
	if storedContext.KSEAF != "kseaf-1" {
		t.Fatalf("stored kseaf = %s, want kseaf-1", storedContext.KSEAF)
	}
	if len(namfClient.notifications) != 1 {
		t.Fatalf("notifications = %d, want 1", len(namfClient.notifications))
	}
	if namfClient.notifications[0].AuthCtxID != "auth-1" {
		t.Fatalf("notification authCtxId = %s, want auth-1", namfClient.notifications[0].AuthCtxID)
	}
	if namfClient.notifications[0].KSEAF != "kseaf-1" {
		t.Fatalf("notification kseaf = %s, want kseaf-1", namfClient.notifications[0].KSEAF)
	}
	if namfClient.uris[0] != "http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify" {
		t.Fatalf("notification uri = %s, want callback template", namfClient.uris[0])
	}
}

func TestCreateUEAuthenticationShouldExposeFiveGAkaLinkForFiveGAka(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{}, nil)

	context, err := authService.CreateUEAuthentication(
		"imsi-250010000000001",
		"5G:mnc001.mcc001.3gppnetwork.org",
		authTypeFiveGAka,
		"",
	)
	if err != nil {
		t.Fatalf("CreateUEAuthentication() error = %v", err)
	}
	if context.Links.FiveGAka == nil {
		t.Fatal("5g-aka link = nil, want link")
	}
	if context.Links.EapSession != nil {
		t.Fatalf("eap-session link = %#v, want nil", context.Links.EapSession)
	}
	if context.AuthData.RAND == "" || context.AuthData.AUTN == "" || context.AuthData.HXRESStar == "" {
		t.Fatalf("auth data = %#v, want populated 5G AKA challenge", context.AuthData)
	}
	if context.Status != authStatusChallengeSent {
		t.Fatalf("status = %s, want %s", context.Status, authStatusChallengeSent)
	}
}

func TestCreateUEAuthenticationShouldRejectUnsupportedAuthType(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{}, nil)

	_, err := authService.CreateUEAuthentication(
		"imsi-250010000000001",
		"5G:mnc001.mcc001.3gppnetwork.org",
		"AKA_TLS",
		"",
	)

	assertAPIError(t, err, http.StatusBadRequest, unsupportedAuthTypeCause, "authType must be 5G_AKA or EAP_AKA_PRIME when provided")
}

func TestCreateUEAuthenticationShouldExposeEapSessionLinkForEapAkaPrime(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{
		initiateResponse: controlplane.AuthenticationResponse{
			Success:            true,
			SUPI:               "imsi-250010000000002",
			AuthType:           "EAP_AKA_PRIME",
			ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
			EapChallenge:       "EAP-Request/AKA'-Challenge token",
		},
	}, nil)

	context, err := authService.CreateUEAuthentication(
		"imsi-250010000000002",
		"5G:mnc001.mcc001.3gppnetwork.org",
		"EAP_AKA_PRIME",
		"",
	)
	if err != nil {
		t.Fatalf("CreateUEAuthentication() error = %v", err)
	}
	if context.Links.EapSession == nil {
		t.Fatal("eap-session link = nil, want link")
	}
	if context.Links.EapSession.Href != "/nausf-auth/v1/ue-authentications/auth-1/eap-session" {
		t.Fatalf("eap-session link = %s, want /nausf-auth/v1/ue-authentications/auth-1/eap-session", context.Links.EapSession.Href)
	}
	if context.Links.FiveGAka != nil {
		t.Fatalf("5g-aka link = %#v, want nil", context.Links.FiveGAka)
	}
	if context.AuthData != (AuthData{}) {
		t.Fatalf("auth data = %#v, want empty for EAP", context.AuthData)
	}
}

func TestLookupDeleteAndConfirmShouldShareContextNotFoundError(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{}, nil)

	_, lookupErr := authService.Lookup("missing")
	assertAPIError(t, lookupErr, http.StatusNotFound, contextNotFoundCause, "authentication context not found")

	deleteErr := authService.Delete("missing")
	assertAPIError(t, deleteErr, http.StatusNotFound, contextNotFoundCause, "authentication context not found")

	_, confirmErr := authService.Confirm("missing", "deadbeef", "")
	assertAPIError(t, confirmErr, http.StatusNotFound, contextNotFoundCause, "authentication context not found")
}

func TestDeleteShouldReturnContextNotFoundAfterContextIsRemoved(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{}, nil)

	context, err := authService.CreateUEAuthentication(
		"imsi-250010000000001",
		"5G:mnc001.mcc001.3gppnetwork.org",
		authTypeFiveGAka,
		"",
	)
	if err != nil {
		t.Fatalf("CreateUEAuthentication() error = %v", err)
	}

	if err := authService.Delete(context.AuthCtxID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	err = authService.Delete(context.AuthCtxID)
	assertAPIError(t, err, http.StatusNotFound, contextNotFoundCause, "authentication context not found")
}

func TestConfirmShouldMapControlPlaneAuthenticationRejectedError(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{
		confirmErr: controlplane.APIError{
			StatusCode: http.StatusUnauthorized,
			Message:    "RES* verification failed",
			ErrorCode:  "AUTHENTICATION_REJECTED",
		},
	}, nil)
	authService.contexts["auth-1"] = AuthContext{
		AuthCtxID: "auth-1",
		SUPI:      "imsi-250010000000001",
	}

	_, err := authService.Confirm("auth-1", "deadbeef", "")
	assertAPIError(t, err, http.StatusUnauthorized, "AUTHENTICATION_REJECTED", "RES* verification failed")
}

func TestMapControlPlaneErrorShouldUseFallbackCauseWhenErrorCodeIsMissing(t *testing.T) {
	err := mapControlPlaneError(controlplane.APIError{
		StatusCode: http.StatusBadGateway,
		Message:    "upstream returned malformed error payload",
	}, controlPlaneInitiateFailedCause, "control-plane initiate failed")

	assertAPIError(t, err, http.StatusBadGateway, controlPlaneInitiateFailedCause, "upstream returned malformed error payload")
}

func TestMapControlPlaneErrorShouldReturnUnavailableForTransportFailure(t *testing.T) {
	err := mapControlPlaneError(errors.New("dial tcp 127.0.0.1:8081: connect: connection refused"), controlPlaneConfirmFailedCause, "control-plane confirmation failed")

	assertAPIError(t, err, http.StatusBadGateway, controlPlaneUnavailableCause, "control-plane confirmation failed: dial tcp 127.0.0.1:8081: connect: connection refused")
}

func assertAPIError(t *testing.T, err error, wantStatus int, wantCause string, wantMessage string) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want APIError")
	}
	apiErr, ok := err.(APIError)
	if !ok {
		t.Fatalf("error type = %T, want APIError", err)
	}
	if apiErr.StatusCode != wantStatus {
		t.Fatalf("status = %d, want %d", apiErr.StatusCode, wantStatus)
	}
	if apiErr.Cause != wantCause {
		t.Fatalf("cause = %s, want %s", apiErr.Cause, wantCause)
	}
	if apiErr.Message != wantMessage {
		t.Fatalf("message = %s, want %s", apiErr.Message, wantMessage)
	}
}
