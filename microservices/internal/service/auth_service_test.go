package service

import (
	"testing"

	"github.com/alexey/ausf/microservices/internal/controlplane"
	"github.com/alexey/ausf/microservices/internal/namf"
)

type stubControlPlaneClient struct {
	confirmResponse controlplane.AuthenticationResponse
}

func (client stubControlPlaneClient) Initiate(request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

func (client stubControlPlaneClient) Confirm(supi string, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error) {
	return client.confirmResponse, nil
}

func (client stubControlPlaneClient) Context(supi string) (controlplane.AuthenticationResponse, error) {
	return controlplane.AuthenticationResponse{}, nil
}

type stubNamfClient struct {
	notifications []namf.UEAuthenticationStatusNotification
}

func (client *stubNamfClient) NotifyUEAuthenticationStatus(notification namf.UEAuthenticationStatusNotification) error {
	client.notifications = append(client.notifications, notification)
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
		SUPI:               "imsi-001010000000001",
		AuthType:           "5G_AKA",
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
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
}
