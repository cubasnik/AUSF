package service

import (
	"testing"

	"github.com/alexey/ausf/microservices/internal/controlplane"
	"github.com/alexey/ausf/microservices/internal/namf"
)

type stubControlPlaneClient struct {
	initiateResponse controlplane.AuthenticationResponse
	confirmResponse controlplane.AuthenticationResponse
}

func (client stubControlPlaneClient) Initiate(request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	if client.initiateResponse.SUPI != "" {
		return client.initiateResponse, nil
	}
	return controlplane.AuthenticationResponse{
		Success:            true,
		SUPI:               request.SUPI,
		AuthType:           request.AuthType,
		ServingNetworkName: request.ServingNetworkName,
	}, nil
}

func (client stubControlPlaneClient) Confirm(supi string, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return client.confirmResponse, nil
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
}
