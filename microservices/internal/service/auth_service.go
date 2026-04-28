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
	FiveGAka   *Link `json:"5g-aka,omitempty"`
	EapSession *Link `json:"eap-session,omitempty"`
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

	if err := validateRequestedAuthType(authType); err != nil {
		return AuthContext{}, err
	}

	response, err := service.controlPlaneClient.Initiate(controlplane.AuthenticationRequest{
		SUPI:               supi,
		ServingNetworkName: servingNetworkName,
		AuthType:           authType,
	})
	if err != nil {
		return AuthContext{}, mapInitiateError(err)
	}

	service.serial++
	authCtxID := fmt.Sprintf("auth-%d", service.serial)
	context := AuthContext{
		AuthCtxID:          authCtxID,
		SUPI:               response.SUPI,
		ServingNetworkName: response.ServingNetworkName,
		AuthType:           response.AuthType,
		NotificationURI:    notificationURI,
		Status:             authStatusChallengeSent,
		Links:              AuthLinks{},
		CreatedAt:          time.Now().UTC(),
	}
	context = initializeAuthContext(context, response)

	service.contexts[authCtxID] = context
	return context, nil
}

func (service *AuthService) Confirm(authCtxID string, resStar string, eapPayload string) (ConfirmationResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	context, err := service.lookupContextLocked(authCtxID)
	if err != nil {
		return ConfirmationResult{}, err
	}

	response, err := service.controlPlaneClient.Confirm(context.SUPI, controlplane.AuthenticationRequest{
		ResStar:    resStar,
		EapPayload: eapPayload,
	})
	if err != nil {
		return ConfirmationResult{}, mapConfirmError(err)
	}

	context.KSEAF = response.KSEAF
	context.Status = authStatusAuthenticated
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

func (service *AuthService) Lookup(authCtxID string) (AuthContext, error) {
	service.mu.RLock()
	defer service.mu.RUnlock()

	return service.lookupContextLocked(authCtxID)
}

func (service *AuthService) Delete(authCtxID string) error {
	service.mu.Lock()
	defer service.mu.Unlock()

	if _, err := service.lookupContextLocked(authCtxID); err != nil {
		return err
	}
	delete(service.contexts, authCtxID)
	return nil
}

func (service *AuthService) lookupContextLocked(authCtxID string) (AuthContext, error) {
	context, ok := service.contexts[authCtxID]
	if !ok {
		return AuthContext{}, contextNotFoundError()
	}
	return context, nil
}

func initializeAuthContext(context AuthContext, response controlplane.AuthenticationResponse) AuthContext {
	switch context.AuthType {
	case authTypeEapAkaPrime:
		return initializeEapAkaPrimeContext(context, response.EapChallenge)
	case authTypeFiveGAka:
		fallthrough
	default:
		return initializeFiveGAkaContext(context, response)
	}
}

func initializeFiveGAkaContext(context AuthContext, response controlplane.AuthenticationResponse) AuthContext {
	context.AuthData = AuthData{
		RAND:      response.RAND,
		AUTN:      response.AUTN,
		HXRESStar: response.HXRESStar,
	}
	context.Links.FiveGAka = &Link{Href: "/nausf-auth/v1/ue-authentications/" + context.AuthCtxID + "/5g-aka-confirmation"}
	return context
}

func initializeEapAkaPrimeContext(context AuthContext, eapChallenge string) AuthContext {
	context.AuthData = AuthData{}
	context.EapSession = &EapSession{
		Method:    "EAP-AKA'",
		Payload:   eapChallenge,
		SessionID: context.AuthCtxID,
	}
	context.Links.EapSession = &Link{Href: "/nausf-auth/v1/ue-authentications/" + context.AuthCtxID + "/eap-session"}
	return context
}
