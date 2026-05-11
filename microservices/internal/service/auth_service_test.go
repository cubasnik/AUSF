package service

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexey/ausf/microservices/internal/controlplane"
	"github.com/alexey/ausf/microservices/internal/namf"
)

type stubControlPlaneClient struct {
	initiateResponse controlplane.AuthenticationResponse
	initiateErr      error
	confirmResponse  controlplane.AuthenticationResponse
	confirmErr       error
}

func (client stubControlPlaneClient) Initiate(_ context.Context, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
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

func (client stubControlPlaneClient) Confirm(_ context.Context, supi string, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	if client.confirmErr != nil {
		return controlplane.AuthenticationResponse{}, client.confirmErr
	}
	if client.confirmResponse.Success || client.confirmResponse.KSEAF != "" || client.confirmResponse.Message != "" {
		return client.confirmResponse, nil
	}
	return controlplane.AuthenticationResponse{Success: true, Message: "authentication successful"}, nil
}

func (client stubControlPlaneClient) Context(_ context.Context, supi string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (client stubControlPlaneClient) SoRProtect(_ context.Context, _ string, _ controlplane.SoRProtectionRequest) (controlplane.SoRProtectionResponse, error) {
	return controlplane.SoRProtectionResponse{SoRMacIAUSF: "00000000000000000000000000000000", CounterSoR: "0000"}, nil
}

func (client stubControlPlaneClient) UPUProtect(_ context.Context, _ string, _ controlplane.UPUProtectionRequest) (controlplane.UPUProtectionResponse, error) {
	return controlplane.UPUProtectionResponse{UPUMacIAUSF: "00000000000000000000000000000000", CounterUPU: "0000"}, nil
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
	mustSaveContext(t, authService, AuthContext{
		AuthCtxID:          "auth-1",
		SUPI:               "imsi-250010000000001",
		AuthType:           "5G_AKA",
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		NotificationURI:    "http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify",
		Status:             "CHALLENGE_SENT",
	})

	result, err := authService.Confirm(context.Background(), "auth-1", "res-star", "", "")
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

func TestConfirmShouldNotifyNamfOnAuthenticationRejected(t *testing.T) {
	namfClient := &stubNamfClient{}
	authService := NewAuthService(stubControlPlaneClient{
		confirmErr: controlplane.APIError{
			StatusCode: http.StatusUnauthorized,
			Message:    "RES* verification failed",
			ErrorCode:  authenticationRejectedCause,
		},
	}, namfClient)
	mustSaveContext(t, authService, AuthContext{
		AuthCtxID:          "auth-1",
		SUPI:               "imsi-250010000000001",
		AuthType:           "5G_AKA",
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		NotificationURI:    "http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify",
		Status:             "CHALLENGE_SENT",
	})

	_, err := authService.Confirm(context.Background(), "auth-1", "deadbeef", "", "")
	if err == nil {
		t.Fatal("Confirm() error = nil, want AUTHENTICATION_REJECTED error")
	}
	if len(namfClient.notifications) != 1 {
		t.Fatalf("notifications = %d, want 1", len(namfClient.notifications))
	}
	if namfClient.notifications[0].AuthResult != "FAILURE" {
		t.Fatalf("notification authResult = %s, want FAILURE", namfClient.notifications[0].AuthResult)
	}
	if namfClient.notifications[0].AuthCtxID != "auth-1" {
		t.Fatalf("notification authCtxId = %s, want auth-1", namfClient.notifications[0].AuthCtxID)
	}
	if namfClient.notifications[0].KSEAF != "" {
		t.Fatalf("notification kseaf = %s, want empty for FAILURE", namfClient.notifications[0].KSEAF)
	}
	if namfClient.uris[0] != "http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify" {
		t.Fatalf("notification uri = %s, want callback template", namfClient.uris[0])
	}
}

func TestConfirmShouldNotSendNotificationOnTransientControlPlaneError(t *testing.T) {
	namfClient := &stubNamfClient{}
	authService := NewAuthService(stubControlPlaneClient{
		confirmErr: controlplane.APIError{
			StatusCode: http.StatusServiceUnavailable,
			Message:    "control plane unavailable",
			ErrorCode:  "CONTROL_PLANE_UNAVAILABLE",
		},
	}, namfClient)
	mustSaveContext(t, authService, AuthContext{
		AuthCtxID:          "auth-1",
		SUPI:               "imsi-250010000000001",
		AuthType:           "5G_AKA",
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		NotificationURI:    "http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify",
		Status:             "CHALLENGE_SENT",
	})

	_, err := authService.Confirm(context.Background(), "auth-1", "deadbeef", "", "")
	if err == nil {
		t.Fatal("Confirm() error = nil, want error")
	}
	if len(namfClient.notifications) != 0 {
		t.Fatalf("notifications = %d, want 0 for transient error", len(namfClient.notifications))
	}
}

func TestCreateUEAuthenticationShouldExposeFiveGAkaLinkForFiveGAka(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{}, nil)

	context, err := authService.CreateUEAuthentication(
		context.Background(),
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
		context.Background(),
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
		context.Background(),
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

	_, confirmErr := authService.Confirm(context.Background(), "missing", "deadbeef", "", "")
	assertAPIError(t, confirmErr, http.StatusNotFound, contextNotFoundCause, "authentication context not found")
}

func TestDeleteShouldReturnContextNotFoundAfterContextIsRemoved(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{}, nil)

	context, err := authService.CreateUEAuthentication(
		context.Background(),
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
	mustSaveContext(t, authService, AuthContext{
		AuthCtxID: "auth-1",
		SUPI:      "imsi-250010000000001",
	})

	_, err := authService.Confirm(context.Background(), "auth-1", "deadbeef", "", "")
	assertAPIError(t, err, http.StatusUnauthorized, "AUTHENTICATION_REJECTED", "RES* verification failed")
	storedContext, lookupErr := authService.Lookup("auth-1")
	if lookupErr != nil {
		t.Fatalf("Lookup() error = %v", lookupErr)
	}
	if storedContext.Status != "FAILED" {
		t.Fatalf("stored status = %s, want FAILED", storedContext.Status)
	}
}

func TestConfirmShouldPersistEapFailurePayloadOnRejectedEapConfirmation(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{
		confirmErr: controlplane.APIError{
			StatusCode: http.StatusUnauthorized,
			Message:    "EAP-AKA' verification failed",
			ErrorCode:  authenticationRejectedCause,
		},
	}, nil)
	mustSaveContext(t, authService, AuthContext{
		AuthCtxID:          "auth-eap-1",
		SUPI:               "imsi-250010000000002",
		AuthType:           authTypeEapAkaPrime,
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		Status:             authStatusChallengeSent,
		EapSession: &EapSession{
			Method:    "EAP-AKA'",
			Payload:   "EAP-Request/AKA'-Challenge token",
			SessionID: "auth-eap-1",
		},
	})

	_, err := authService.Confirm(context.Background(), "auth-eap-1", "", "", "EAP-Response/AKA'-Challenge bad")
	assertAPIError(t, err, http.StatusUnauthorized, authenticationRejectedCause, "EAP-AKA' verification failed")
	storedContext, lookupErr := authService.Lookup("auth-eap-1")
	if lookupErr != nil {
		t.Fatalf("Lookup() error = %v", lookupErr)
	}
	if storedContext.Status != "FAILED" {
		t.Fatalf("stored status = %s, want FAILED", storedContext.Status)
	}
	if storedContext.EapSession == nil || storedContext.EapSession.Payload != eapFailurePayload {
		t.Fatalf("stored eap session = %#v, want EAP-Failure payload", storedContext.EapSession)
	}
}

type staleEapAfterRefreshControlPlaneClient struct{}

func (staleEapAfterRefreshControlPlaneClient) Initiate(_ context.Context, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{
		Success:            true,
		SUPI:               request.SUPI,
		AuthType:           request.AuthType,
		ServingNetworkName: request.ServingNetworkName,
		EapChallenge:       "EAP-Request/AKA'-Challenge initial-token",
	}, nil
}

func (staleEapAfterRefreshControlPlaneClient) Confirm(_ context.Context, _ string, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	switch request.EapPayload {
	case "EAP-Response/AKA'-Reauthentication token":
		return controlplane.AuthenticationResponse{
			Success:      true,
			RAND:         "rand-2",
			AUTN:         "autn-2",
			HXRESStar:    "hxres-2",
			EapChallenge: "EAP-Request/AKA'-Challenge refreshed-token",
			Message:      "EAP-AKA' re-authentication challenge generated",
		}, nil
	case "EAP-Response/AKA'-Challenge RES*=stale-token":
		return controlplane.AuthenticationResponse{}, controlplane.APIError{
			StatusCode: http.StatusUnauthorized,
			Message:    "EAP-AKA' verification failed",
			ErrorCode:  authenticationRejectedCause,
			EapPayload: eapFailurePayload,
		}
	default:
		return controlplane.AuthenticationResponse{}, errors.New("unexpected eap payload")
	}
}

func (staleEapAfterRefreshControlPlaneClient) Context(_ context.Context, _ string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (staleEapAfterRefreshControlPlaneClient) SoRProtect(_ context.Context, _ string, _ controlplane.SoRProtectionRequest) (controlplane.SoRProtectionResponse, error) {
	return controlplane.SoRProtectionResponse{}, nil
}

func (staleEapAfterRefreshControlPlaneClient) UPUProtect(_ context.Context, _ string, _ controlplane.UPUProtectionRequest) (controlplane.UPUProtectionResponse, error) {
	return controlplane.UPUProtectionResponse{}, nil
}

func TestConfirmShouldRejectStaleEapResponseAfterReauthenticationRefresh(t *testing.T) {
	authService := NewAuthService(staleEapAfterRefreshControlPlaneClient{}, nil)

	_, err := authService.CreateUEAuthentication(
		context.Background(),
		"imsi-250010000000002",
		"5G:mnc001.mcc001.3gppnetwork.org",
		authTypeEapAkaPrime,
		"",
	)
	if err != nil {
		t.Fatalf("CreateUEAuthentication() error = %v", err)
	}

	refreshed, err := authService.Confirm(
		context.Background(),
		"auth-1",
		"",
		"",
		"EAP-Response/AKA'-Reauthentication token",
	)
	if err != nil {
		t.Fatalf("Confirm() refresh error = %v", err)
	}
	if refreshed.AuthResult != authResultOngoing {
		t.Fatalf("refresh auth result = %s, want %s", refreshed.AuthResult, authResultOngoing)
	}

	_, err = authService.Confirm(
		context.Background(),
		"auth-1",
		"",
		"",
		"EAP-Response/AKA'-Challenge RES*=stale-token",
	)
	assertAPIError(t, err, http.StatusUnauthorized, authenticationRejectedCause, "EAP-AKA' verification failed")

	storedContext, lookupErr := authService.Lookup("auth-1")
	if lookupErr != nil {
		t.Fatalf("Lookup() error = %v", lookupErr)
	}
	if storedContext.Status != "FAILED" {
		t.Fatalf("stored status = %s, want FAILED", storedContext.Status)
	}
	if storedContext.EapSession == nil || storedContext.EapSession.Payload != eapFailurePayload {
		t.Fatalf("stored eap session = %#v, want EAP-Failure payload", storedContext.EapSession)
	}
	if storedContext.KSEAF != "" {
		t.Fatalf("stored kseaf = %s, want empty", storedContext.KSEAF)
	}
}

func TestConfirmShouldReturnOngoingWithRefreshedEapChallengeOnReauthentication(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{
		confirmResponse: controlplane.AuthenticationResponse{
			Success:      true,
			RAND:         "rand-2",
			AUTN:         "autn-2",
			HXRESStar:    "hxres-2",
			EapChallenge: "EAP-Request/AKA'-Challenge refreshed-token",
			Message:      "EAP-AKA' re-authentication challenge generated",
		},
	}, nil)
	mustSaveContext(t, authService, AuthContext{
		AuthCtxID:          "auth-eap-1",
		SUPI:               "imsi-250010000000002",
		AuthType:           authTypeEapAkaPrime,
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		Status:             authStatusChallengeSent,
		EapSession: &EapSession{
			Method:    "EAP-AKA'",
			Payload:   "EAP-Request/AKA'-Challenge old-token",
			SessionID: "auth-eap-1",
		},
	})

	result, err := authService.Confirm(context.Background(), "auth-eap-1", "", "", "EAP-Response/AKA'-Reauthentication token")
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if result.AuthResult != authResultOngoing {
		t.Fatalf("Confirm() auth result = %s, want %s", result.AuthResult, authResultOngoing)
	}
	if result.EapSession == nil || result.EapSession.Payload != "EAP-Request/AKA'-Challenge refreshed-token" {
		t.Fatalf("Confirm() eap session = %#v, want refreshed challenge", result.EapSession)
	}
	storedContext, lookupErr := authService.Lookup("auth-eap-1")
	if lookupErr != nil {
		t.Fatalf("Lookup() error = %v", lookupErr)
	}
	if storedContext.Status != authStatusChallengeSent {
		t.Fatalf("stored status = %s, want %s", storedContext.Status, authStatusChallengeSent)
	}
	if storedContext.EapSession == nil || storedContext.EapSession.Payload != "EAP-Request/AKA'-Challenge refreshed-token" {
		t.Fatalf("stored eap session = %#v, want refreshed challenge", storedContext.EapSession)
	}
	if storedContext.KSEAF != "" {
		t.Fatalf("stored kseaf = %s, want empty", storedContext.KSEAF)
	}
}

func TestConfirmShouldReturnOngoingWithRefreshedEapChallengeOnFastReauthentication(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{
		confirmResponse: controlplane.AuthenticationResponse{
			Success:      true,
			RAND:         "rand-2",
			AUTN:         "autn-2",
			HXRESStar:    "hxres-2",
			EapChallenge: "EAP-Request/AKA'-Challenge refreshed-fast-token",
			Message:      "EAP-AKA' fast re-authentication challenge generated",
		},
	}, nil)
	mustSaveContext(t, authService, AuthContext{
		AuthCtxID:          "auth-eap-fast-1",
		SUPI:               "imsi-250010000000002",
		AuthType:           authTypeEapAkaPrime,
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		Status:             authStatusChallengeSent,
		EapSession: &EapSession{
			Method:    "EAP-AKA'",
			Payload:   "EAP-Request/AKA'-Challenge old-token",
			SessionID: "auth-eap-fast-1",
		},
	})

	result, err := authService.Confirm(context.Background(), "auth-eap-fast-1", "", "", "EAP-Response/AKA'-Fast-Reauthentication token")
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if result.AuthResult != authResultOngoing {
		t.Fatalf("Confirm() auth result = %s, want %s", result.AuthResult, authResultOngoing)
	}
	if result.EapSession == nil || result.EapSession.Payload != "EAP-Request/AKA'-Challenge refreshed-fast-token" {
		t.Fatalf("Confirm() eap session = %#v, want refreshed fast challenge", result.EapSession)
	}
	storedContext, lookupErr := authService.Lookup("auth-eap-fast-1")
	if lookupErr != nil {
		t.Fatalf("Lookup() error = %v", lookupErr)
	}
	if storedContext.Status != authStatusChallengeSent {
		t.Fatalf("stored status = %s, want %s", storedContext.Status, authStatusChallengeSent)
	}
	if storedContext.EapSession == nil || storedContext.EapSession.Payload != "EAP-Request/AKA'-Challenge refreshed-fast-token" {
		t.Fatalf("stored eap session = %#v, want refreshed fast challenge", storedContext.EapSession)
	}
	if storedContext.KSEAF != "" {
		t.Fatalf("stored kseaf = %s, want empty", storedContext.KSEAF)
	}
}

func TestConfirmShouldReturnOngoingWithRefreshedEapChallengeOnSynchronizationFailure(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{
		confirmResponse: controlplane.AuthenticationResponse{
			Success:      true,
			RAND:         "rand-2",
			AUTN:         "autn-2",
			HXRESStar:    "hxres-2",
			EapChallenge: "EAP-Request/AKA'-Challenge refreshed-sync-token",
			Message:      "EAP-AKA' synchronization-failure challenge generated",
		},
	}, nil)
	mustSaveContext(t, authService, AuthContext{
		AuthCtxID:          "auth-eap-sync-1",
		SUPI:               "imsi-250010000000002",
		AuthType:           authTypeEapAkaPrime,
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		Status:             authStatusChallengeSent,
		EapSession: &EapSession{
			Method:    "EAP-AKA'",
			Payload:   "EAP-Request/AKA'-Challenge old-token",
			SessionID: "auth-eap-sync-1",
		},
	})

	result, err := authService.Confirm(context.Background(), "auth-eap-sync-1", "", "", "EAP-Response/AKA'-Synchronization-Failure AUTS=auts-token")
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if result.AuthResult != authResultOngoing {
		t.Fatalf("Confirm() auth result = %s, want %s", result.AuthResult, authResultOngoing)
	}
	if result.EapSession == nil || result.EapSession.Payload != "EAP-Request/AKA'-Challenge refreshed-sync-token" {
		t.Fatalf("Confirm() eap session = %#v, want refreshed sync-failure challenge", result.EapSession)
	}
	storedContext, lookupErr := authService.Lookup("auth-eap-sync-1")
	if lookupErr != nil {
		t.Fatalf("Lookup() error = %v", lookupErr)
	}
	if storedContext.Status != authStatusChallengeSent {
		t.Fatalf("stored status = %s, want %s", storedContext.Status, authStatusChallengeSent)
	}
	if storedContext.EapSession == nil || storedContext.EapSession.Payload != "EAP-Request/AKA'-Challenge refreshed-sync-token" {
		t.Fatalf("stored eap session = %#v, want refreshed sync-failure challenge", storedContext.EapSession)
	}
	if storedContext.KSEAF != "" {
		t.Fatalf("stored kseaf = %s, want empty", storedContext.KSEAF)
	}
}

func TestConfirmShouldReturnSyncFailureWithRefreshedChallengeWhenAutsIsProvided(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{
		confirmResponse: controlplane.AuthenticationResponse{
			Success:   true,
			RAND:      "rand-2",
			AUTN:      "autn-2",
			HXRESStar: "hxres-2",
			Message:   "re-synchronization challenge generated",
		},
	}, nil)
	mustSaveContext(t, authService, AuthContext{
		AuthCtxID:          "auth-1",
		SUPI:               "imsi-250010000000001",
		AuthType:           authTypeFiveGAka,
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		Status:             authStatusChallengeSent,
	})

	result, err := authService.Confirm(context.Background(), "auth-1", "", "auts-token", "")
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if result.AuthResult != authResultSyncFailure {
		t.Fatalf("Confirm() auth result = %s, want %s", result.AuthResult, authResultSyncFailure)
	}
	if result.AuthData == nil || result.AuthData.RAND != "rand-2" {
		t.Fatalf("Confirm() auth data = %#v, want refreshed challenge", result.AuthData)
	}
	storedContext, err := authService.Lookup("auth-1")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if storedContext.Status != authStatusChallengeSent {
		t.Fatalf("stored status = %s, want %s", storedContext.Status, authStatusChallengeSent)
	}
	if storedContext.AuthData.RAND != "rand-2" {
		t.Fatalf("stored rand = %s, want rand-2", storedContext.AuthData.RAND)
	}
	if storedContext.KSEAF != "" {
		t.Fatalf("stored kseaf = %s, want empty", storedContext.KSEAF)
	}
}

type staleFiveGAkaAfterResyncControlPlaneClient struct{}

func (staleFiveGAkaAfterResyncControlPlaneClient) Initiate(_ context.Context, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{
		Success:            true,
		SUPI:               request.SUPI,
		AuthType:           request.AuthType,
		ServingNetworkName: request.ServingNetworkName,
		RAND:               "rand-1",
		AUTN:               "autn-1",
		HXRESStar:          "hxres-1",
	}, nil
}

func (staleFiveGAkaAfterResyncControlPlaneClient) Confirm(_ context.Context, _ string, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	switch {
	case request.AUTS == "auts-token":
		return controlplane.AuthenticationResponse{
			Success:   true,
			RAND:      "rand-2",
			AUTN:      "autn-2",
			HXRESStar: "hxres-2",
			Message:   "re-synchronization challenge generated",
		}, nil
	case request.ResStar == "stale-token":
		return controlplane.AuthenticationResponse{}, controlplane.APIError{
			StatusCode: http.StatusUnauthorized,
			Message:    "RES* verification failed",
			ErrorCode:  authenticationRejectedCause,
		}
	default:
		return controlplane.AuthenticationResponse{}, errors.New("unexpected 5G_AKA confirmation payload")
	}
}

func (staleFiveGAkaAfterResyncControlPlaneClient) Context(_ context.Context, _ string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (staleFiveGAkaAfterResyncControlPlaneClient) SoRProtect(_ context.Context, _ string, _ controlplane.SoRProtectionRequest) (controlplane.SoRProtectionResponse, error) {
	return controlplane.SoRProtectionResponse{}, nil
}

func (staleFiveGAkaAfterResyncControlPlaneClient) UPUProtect(_ context.Context, _ string, _ controlplane.UPUProtectionRequest) (controlplane.UPUProtectionResponse, error) {
	return controlplane.UPUProtectionResponse{}, nil
}

type staleAutsAfterResyncControlPlaneClient struct {
	autsAttempts int
}

func (client *staleAutsAfterResyncControlPlaneClient) Initiate(_ context.Context, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{
		Success:            true,
		SUPI:               request.SUPI,
		AuthType:           request.AuthType,
		ServingNetworkName: request.ServingNetworkName,
		RAND:               "rand-1",
		AUTN:               "autn-1",
		HXRESStar:          "hxres-1",
	}, nil
}

func (client *staleAutsAfterResyncControlPlaneClient) Confirm(_ context.Context, _ string, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	switch {
	case request.AUTS == "auts-token":
		client.autsAttempts++
		if client.autsAttempts == 1 {
			return controlplane.AuthenticationResponse{
				Success:   true,
				RAND:      "rand-2",
				AUTN:      "autn-2",
				HXRESStar: "hxres-2",
				Message:   "re-synchronization challenge generated",
			}, nil
		}
		return controlplane.AuthenticationResponse{}, controlplane.APIError{
			StatusCode: http.StatusUnauthorized,
			Message:    "AUTS verification failed",
			ErrorCode:  authenticationRejectedCause,
		}
	default:
		return controlplane.AuthenticationResponse{}, errors.New("unexpected 5G_AKA confirmation payload")
	}
}

func (client *staleAutsAfterResyncControlPlaneClient) Context(_ context.Context, _ string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (client *staleAutsAfterResyncControlPlaneClient) SoRProtect(_ context.Context, _ string, _ controlplane.SoRProtectionRequest) (controlplane.SoRProtectionResponse, error) {
	return controlplane.SoRProtectionResponse{}, nil
}

func (client *staleAutsAfterResyncControlPlaneClient) UPUProtect(_ context.Context, _ string, _ controlplane.UPUProtectionRequest) (controlplane.UPUProtectionResponse, error) {
	return controlplane.UPUProtectionResponse{}, nil
}

func TestConfirmShouldRejectStaleResStarAfterSyncFailureRefresh(t *testing.T) {
	authService := NewAuthService(staleFiveGAkaAfterResyncControlPlaneClient{}, nil)

	_, err := authService.CreateUEAuthentication(
		context.Background(),
		"imsi-250010000000001",
		"5G:mnc001.mcc001.3gppnetwork.org",
		authTypeFiveGAka,
		"",
	)
	if err != nil {
		t.Fatalf("CreateUEAuthentication() error = %v", err)
	}

	refreshed, err := authService.Confirm(context.Background(), "auth-1", "", "auts-token", "")
	if err != nil {
		t.Fatalf("Confirm() resync error = %v", err)
	}
	if refreshed.AuthResult != authResultSyncFailure {
		t.Fatalf("resync auth result = %s, want %s", refreshed.AuthResult, authResultSyncFailure)
	}

	_, err = authService.Confirm(context.Background(), "auth-1", "stale-token", "", "")
	assertAPIError(t, err, http.StatusUnauthorized, authenticationRejectedCause, "RES* verification failed")

	storedContext, lookupErr := authService.Lookup("auth-1")
	if lookupErr != nil {
		t.Fatalf("Lookup() error = %v", lookupErr)
	}
	if storedContext.Status != "FAILED" {
		t.Fatalf("stored status = %s, want FAILED", storedContext.Status)
	}
	if storedContext.AuthData.RAND != "rand-2" {
		t.Fatalf("stored rand = %s, want rand-2", storedContext.AuthData.RAND)
	}
	if storedContext.KSEAF != "" {
		t.Fatalf("stored kseaf = %s, want empty", storedContext.KSEAF)
	}
}

func TestConfirmShouldRejectStaleAutsAfterSyncFailureRefresh(t *testing.T) {
	authService := NewAuthService(&staleAutsAfterResyncControlPlaneClient{}, nil)

	_, err := authService.CreateUEAuthentication(
		context.Background(),
		"imsi-250010000000001",
		"5G:mnc001.mcc001.3gppnetwork.org",
		authTypeFiveGAka,
		"",
	)
	if err != nil {
		t.Fatalf("CreateUEAuthentication() error = %v", err)
	}

	refreshed, err := authService.Confirm(context.Background(), "auth-1", "", "auts-token", "")
	if err != nil {
		t.Fatalf("Confirm() resync error = %v", err)
	}
	if refreshed.AuthResult != authResultSyncFailure {
		t.Fatalf("resync auth result = %s, want %s", refreshed.AuthResult, authResultSyncFailure)
	}

	_, err = authService.Confirm(context.Background(), "auth-1", "", "auts-token", "")
	assertAPIError(t, err, http.StatusUnauthorized, authenticationRejectedCause, "AUTS verification failed")

	storedContext, lookupErr := authService.Lookup("auth-1")
	if lookupErr != nil {
		t.Fatalf("Lookup() error = %v", lookupErr)
	}
	if storedContext.Status != "FAILED" {
		t.Fatalf("stored status = %s, want FAILED", storedContext.Status)
	}
	if storedContext.AuthData.RAND != "rand-2" {
		t.Fatalf("stored rand = %s, want rand-2", storedContext.AuthData.RAND)
	}
	if storedContext.KSEAF != "" {
		t.Fatalf("stored kseaf = %s, want empty", storedContext.KSEAF)
	}
}

func TestConfirmShouldRejectNonPendingContext(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{
		confirmResponse: controlplane.AuthenticationResponse{
			Success: true,
			KSEAF:   "kseaf-1",
			Message: "authentication successful",
		},
	}, nil)
	mustSaveContext(t, authService, AuthContext{
		AuthCtxID:          "auth-1",
		SUPI:               "imsi-250010000000001",
		AuthType:           authTypeFiveGAka,
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		Status:             authStatusAuthenticated,
	})

	_, err := authService.Confirm(context.Background(), "auth-1", "res-star", "", "")
	assertAPIError(t, err, http.StatusUnauthorized, authenticationRejectedCause, "authentication context is no longer pending")
}

func TestConfirmShouldRejectAlreadyFailedFiveGAkaContext(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{}, nil)
	mustSaveContext(t, authService, AuthContext{
		AuthCtxID:          "auth-1",
		SUPI:               "imsi-250010000000001",
		AuthType:           authTypeFiveGAka,
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		Status:             "FAILED",
		AuthData: AuthData{
			RAND:      "rand-1",
			AUTN:      "autn-1",
			HXRESStar: "hxres-1",
		},
	})

	_, err := authService.Confirm(context.Background(), "auth-1", "res-star", "", "")
	assertAPIError(t, err, http.StatusUnauthorized, authenticationRejectedCause, "authentication context is no longer pending")
}

func TestConfirmShouldRejectAutsForNonFiveGAkaContext(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{
		confirmResponse: controlplane.AuthenticationResponse{
			Success:   true,
			RAND:      "rand-2",
			AUTN:      "autn-2",
			HXRESStar: "hxres-2",
			Message:   "re-synchronization challenge generated",
		},
	}, nil)
	mustSaveContext(t, authService, AuthContext{
		AuthCtxID:          "auth-1",
		SUPI:               "imsi-250010000000002",
		AuthType:           authTypeEapAkaPrime,
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		Status:             authStatusChallengeSent,
	})

	_, err := authService.Confirm(context.Background(), "auth-1", "", "auts-token", "")
	assertAPIError(t, err, http.StatusUnauthorized, authenticationRejectedCause, "AUTS re-synchronization is only valid for 5G_AKA")
}

func TestConfirmShouldRejectAmbiguousFiveGAkaPayload(t *testing.T) {
	authService := NewAuthService(stubControlPlaneClient{
		confirmResponse: controlplane.AuthenticationResponse{
			Success: true,
			KSEAF:   "kseaf-1",
			Message: "authentication successful",
		},
	}, nil)
	mustSaveContext(t, authService, AuthContext{
		AuthCtxID:          "auth-1",
		SUPI:               "imsi-250010000000001",
		AuthType:           authTypeFiveGAka,
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		Status:             authStatusChallengeSent,
	})

	_, err := authService.Confirm(context.Background(), "auth-1", "res-star", "auts-token", "")
	assertAPIError(t, err, http.StatusUnauthorized, authenticationRejectedCause, "5G_AKA confirmation must provide exactly one of RES* or AUTS")
}

func TestLookupShouldReturnPersistedContextAcrossServiceRecreation(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "auth-contexts.json")
	store, err := NewFileAuthContextStore(storePath)
	if err != nil {
		t.Fatalf("NewFileAuthContextStore() error = %v", err)
	}

	firstService := NewAuthServiceWithStore(stubControlPlaneClient{}, nil, store)
	created, err := firstService.CreateUEAuthentication(
		context.Background(),
		"imsi-250010000000001",
		"5G:mnc001.mcc001.3gppnetwork.org",
		authTypeFiveGAka,
		"",
	)
	if err != nil {
		t.Fatalf("CreateUEAuthentication() error = %v", err)
	}

	reloadedStore, err := NewFileAuthContextStore(storePath)
	if err != nil {
		t.Fatalf("NewFileAuthContextStore() reload error = %v", err)
	}

	secondService := NewAuthServiceWithStore(stubControlPlaneClient{}, nil, reloadedStore)
	loaded, err := secondService.Lookup(created.AuthCtxID)
	if err != nil {
		t.Fatalf("Lookup() error after service recreation = %v", err)
	}
	if loaded.AuthCtxID != created.AuthCtxID {
		t.Fatalf("AuthCtxID = %s, want %s", loaded.AuthCtxID, created.AuthCtxID)
	}
	if loaded.SUPI != created.SUPI {
		t.Fatalf("SUPI = %s, want %s", loaded.SUPI, created.SUPI)
	}
	if loaded.Status != created.Status {
		t.Fatalf("status = %s, want %s", loaded.Status, created.Status)
	}
}

func TestLookupShouldReturnContextNotFoundAfterTTLExpiration(t *testing.T) {
	authService := NewAuthServiceWithStoreAndTTL(stubControlPlaneClient{}, nil, NewInMemoryAuthContextStore(), 20*time.Millisecond)

	created, err := authService.CreateUEAuthentication(
		context.Background(),
		"imsi-250010000000001",
		"5G:mnc001.mcc001.3gppnetwork.org",
		authTypeFiveGAka,
		"",
	)
	if err != nil {
		t.Fatalf("CreateUEAuthentication() error = %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	_, lookupErr := authService.Lookup(created.AuthCtxID)
	assertAPIError(t, lookupErr, http.StatusNotFound, contextNotFoundCause, "authentication context not found")

	_, ok, getErr := authService.store.Get(created.AuthCtxID)
	if getErr != nil {
		t.Fatalf("store.Get() error = %v", getErr)
	}
	if ok {
		t.Fatalf("store still contains expired auth context %s", created.AuthCtxID)
	}
}

func TestConfirmShouldReturnContextNotFoundAfterTTLExpiration(t *testing.T) {
	authService := NewAuthServiceWithStoreAndTTL(stubControlPlaneClient{}, nil, NewInMemoryAuthContextStore(), 20*time.Millisecond)

	created, err := authService.CreateUEAuthentication(
		context.Background(),
		"imsi-250010000000001",
		"5G:mnc001.mcc001.3gppnetwork.org",
		authTypeFiveGAka,
		"",
	)
	if err != nil {
		t.Fatalf("CreateUEAuthentication() error = %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	_, confirmErr := authService.Confirm(context.Background(), created.AuthCtxID, "deadbeef", "", "")
	assertAPIError(t, confirmErr, http.StatusNotFound, contextNotFoundCause, "authentication context not found")
}

func TestLookupShouldExpireLegacyContextWithoutCreatedAtWhenTTLIsEnabled(t *testing.T) {
	authService := NewAuthServiceWithStoreAndTTL(stubControlPlaneClient{}, nil, NewInMemoryAuthContextStore(), time.Minute)

	legacyContext := AuthContext{
		AuthCtxID:          "auth-legacy",
		SUPI:               "imsi-250010000000001",
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		AuthType:           authTypeFiveGAka,
		Status:             authStatusChallengeSent,
		CreatedAt:          time.Time{},
	}
	if err := authService.store.Save(legacyContext); err != nil {
		t.Fatalf("save context error = %v", err)
	}

	_, err := authService.Lookup(legacyContext.AuthCtxID)
	assertAPIError(t, err, http.StatusNotFound, contextNotFoundCause, "authentication context not found")

	_, ok, getErr := authService.store.Get(legacyContext.AuthCtxID)
	if getErr != nil {
		t.Fatalf("store.Get() error = %v", getErr)
	}
	if ok {
		t.Fatalf("store still contains expired legacy auth context %s", legacyContext.AuthCtxID)
	}
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

func mustSaveContext(t *testing.T, authService *AuthService, context AuthContext) {
	t.Helper()
	if context.CreatedAt.IsZero() {
		context.CreatedAt = time.Now().UTC()
	}
	if context.Status == "" {
		context.Status = authStatusChallengeSent
	}
	if err := authService.store.Save(context); err != nil {
		t.Fatalf("save context error = %v", err)
	}
}
