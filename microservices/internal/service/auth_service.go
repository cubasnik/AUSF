package service

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/alexey/ausf/microservices/internal/controlplane"
	"github.com/alexey/ausf/microservices/internal/namf"
)

type AuthContext struct {
	AuthCtxID          string      `json:"authCtxId"`
	SUPI               string      `json:"supi"`
	ServingNetworkName string      `json:"servingNetworkName"`
	AuthType           string      `json:"authType"`
	NotificationURI    string      `json:"notificationUri,omitempty"`
	AuthData           AuthData    `json:"5gAuthData"`
	EapSession         *EapSession `json:"eapSession,omitempty"`
	KSEAF              string      `json:"kseaf,omitempty"`
	Status             string      `json:"status"`
	Links              AuthLinks   `json:"_links"`
	CreatedAt          time.Time   `json:"createdAt"`
}

type AuthData struct {
	RAND      string `json:"rand,omitempty"`
	AUTN      string `json:"autn,omitempty"`
	HXRESStar string `json:"hxresStar,omitempty"`
}

type EapSession struct {
	Method    string `json:"method"`
	Payload   string `json:"payload"`
	SessionID string `json:"sessionId"`
}

type AuthLinks struct {
	FiveGAka Link `json:"5g-aka,omitempty"`
}

type Link struct {
	Href string `json:"href"`
}

type ConfirmationResult struct {
	AuthCtxID  string `json:"authCtxId"`
	SUPI       string `json:"supi"`
	AuthResult string `json:"authResult"`
	KSEAF      string `json:"kseaf,omitempty"`
	Message    string `json:"message,omitempty"`
}

type APIError struct {
	StatusCode int
	Message    string
	Cause      string
}

func (error APIError) Error() string {
	return error.Message
}

type AuthService struct {
	mu                 sync.RWMutex
	contexts           map[string]AuthContext
	serial             uint64
	controlPlaneClient controlPlaneAPI
	namfClient         namfNotifier
}

type controlPlaneAPI interface {
	Initiate(request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error)
	Confirm(supi string, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error)
	Context(supi string) (controlplane.AuthenticationResponse, error)
}

type namfNotifier interface {
	NotifyUEAuthenticationStatus(notification namf.UEAuthenticationStatusNotification, notificationURI string) error
}

func NewAuthService(controlPlaneClient controlPlaneAPI, namfClient namfNotifier) *AuthService {
	return &AuthService{
		contexts:           make(map[string]AuthContext),
		controlPlaneClient: controlPlaneClient,
		namfClient:         namfClient,
	}
}

func (service *AuthService) CreateUEAuthentication(supi string, servingNetworkName string, authType string, notificationURI string) (AuthContext, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	response, err := service.controlPlaneClient.Initiate(controlplane.AuthenticationRequest{
		SUPI:               supi,
		ServingNetworkName: servingNetworkName,
		AuthType:           authType,
	})
	if err != nil {
		return AuthContext{}, mapControlPlaneError(err, "control-plane initiate failed")
	}

	service.serial++
	authCtxID := fmt.Sprintf("auth-%d", service.serial)
	context := AuthContext{
		AuthCtxID:          authCtxID,
		SUPI:               response.SUPI,
		ServingNetworkName: response.ServingNetworkName,
		AuthType:           response.AuthType,
		NotificationURI:    notificationURI,
		AuthData: AuthData{
			RAND:      response.RAND,
			AUTN:      response.AUTN,
			HXRESStar: response.HXRESStar,
		},
		Status: "CHALLENGE_SENT",
		Links: AuthLinks{
			FiveGAka: Link{Href: "/nausf-auth/v1/ue-authentications/" + authCtxID + "/5g-aka-confirmation"},
		},
		CreatedAt: time.Now().UTC(),
	}
	if context.AuthType == "EAP_AKA_PRIME" {
		context.EapSession = &EapSession{
			Method:    "EAP-AKA'",
			Payload:   response.EapChallenge,
			SessionID: authCtxID,
		}
		context.AuthData = AuthData{}
	}

	service.contexts[authCtxID] = context
	return context, nil
}

func (service *AuthService) Confirm(authCtxID string, resStar string, eapPayload string) (ConfirmationResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	context, ok := service.contexts[authCtxID]
	if !ok {
		return ConfirmationResult{}, APIError{StatusCode: 404, Message: "authentication context not found", Cause: "CONTEXT_NOT_FOUND"}
	}

	response, err := service.controlPlaneClient.Confirm(context.SUPI, controlplane.AuthenticationRequest{
		ResStar:    resStar,
		EapPayload: eapPayload,
	})
	if err != nil {
		return ConfirmationResult{}, mapControlPlaneError(err, "control-plane confirmation failed")
	}

	context.KSEAF = response.KSEAF
	context.Status = "AUTHENTICATED"
	service.contexts[authCtxID] = context

	if service.namfClient != nil {
		notification := namf.UEAuthenticationStatusNotification{
			AuthCtxID:          authCtxID,
			SUPI:               context.SUPI,
			AuthType:           context.AuthType,
			ServingNetworkName: context.ServingNetworkName,
			AuthResult:         "SUCCESS",
			KSEAF:              context.KSEAF,
		}
		if err := service.namfClient.NotifyUEAuthenticationStatus(notification, context.NotificationURI); err != nil {
			log.Printf("namf notification failed for %s: %v", authCtxID, err)
		}
	}

	return ConfirmationResult{
		AuthCtxID:  authCtxID,
		SUPI:       context.SUPI,
		AuthResult: "SUCCESS",
		KSEAF:      context.KSEAF,
		Message:    response.Message,
	}, nil
}

func (service *AuthService) Lookup(authCtxID string) (AuthContext, bool) {
	service.mu.RLock()
	defer service.mu.RUnlock()

	context, ok := service.contexts[authCtxID]
	return context, ok
}

func (service *AuthService) Delete(authCtxID string) bool {
	service.mu.Lock()
	defer service.mu.Unlock()

	if _, ok := service.contexts[authCtxID]; !ok {
		return false
	}
	delete(service.contexts, authCtxID)
	return true
}

func mapControlPlaneError(err error, fallbackMessage string) error {
	if apiErr, ok := err.(controlplane.APIError); ok {
		return APIError{StatusCode: apiErr.StatusCode, Message: apiErr.Message, Cause: fallbackMessage}
	}
	return APIError{StatusCode: 502, Message: fallbackMessage + ": " + err.Error(), Cause: "CONTROL_PLANE_UNAVAILABLE"}
}
