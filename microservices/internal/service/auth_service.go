package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

// eapFailurePayload is the fallback binary EAP-Failure (Code=4, ID=0, Len=4) in base64url, no padding.
// Used only if the control-plane response is missing the eapChallenge field.
const eapFailurePayload = "BAAABA"

type AuthLinks struct {
	FiveGAka   *Link `json:"5g-aka,omitempty"`
	EapSession *Link `json:"eap-session,omitempty"`
}

type Link struct {
	Href string `json:"href"`
}

type ConfirmationResult struct {
	AuthCtxID  string      `json:"authCtxId"`
	SUPI       string      `json:"supi"`
	AuthResult string      `json:"authResult"`
	AuthData   *AuthData   `json:"5gAuthData,omitempty"`
	EapSession *EapSession `json:"eapSession,omitempty"`
	KSEAF      string      `json:"kseaf,omitempty"`
	Message    string      `json:"message,omitempty"`
}

type APIError struct {
	StatusCode int
	Message    string
	Cause      string
	EapPayload string
}

func (error APIError) Error() string {
	return error.Message
}

type AuthService struct {
	mu                 sync.RWMutex
	store              AuthContextStore
	contextTTL         time.Duration
	controlPlaneClient controlPlaneAPI
	namfClient         namfNotifier
	recorder           authMetricsRecorder
}

// authMetricsRecorder is a local interface so the service package does not
// need to import the metrics package — any value satisfying these methods
// (including *metrics.Registry) will work.
type authMetricsRecorder interface {
	RecordAuthInitiated(authType string)
	RecordAuthConfirmed(authType string)
	RecordAuthFailed(cause string)
	RecordSyncFailure(authType string)
	RecordAuthDuration(authType string, durationSeconds float64)
}

type controlPlaneAPI interface {
	Initiate(ctx context.Context, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error)
	Confirm(ctx context.Context, authCtxID string, request controlplane.AuthenticationRequest) (controlplane.AuthenticationResponse, error)
	Context(ctx context.Context, supi string) (controlplane.AuthenticationResponse, error)
	SoRProtect(ctx context.Context, authCtxID string, req controlplane.SoRProtectionRequest) (controlplane.SoRProtectionResponse, error)
	UPUProtect(ctx context.Context, authCtxID string, req controlplane.UPUProtectionRequest) (controlplane.UPUProtectionResponse, error)
}

type namfNotifier interface {
	NotifyUEAuthenticationStatus(notification namf.UEAuthenticationStatusNotification, notificationURI string) error
}

const defaultAuthContextTTL = 15 * time.Minute

func NewAuthService(controlPlaneClient controlPlaneAPI, namfClient namfNotifier) *AuthService {
	return NewAuthServiceWithStoreAndTTL(controlPlaneClient, namfClient, NewInMemoryAuthContextStore(), defaultAuthContextTTL)
}

func NewAuthServiceWithStore(controlPlaneClient controlPlaneAPI, namfClient namfNotifier, store AuthContextStore) *AuthService {
	return NewAuthServiceWithStoreAndTTL(controlPlaneClient, namfClient, store, defaultAuthContextTTL)
}

func NewAuthServiceWithStoreAndTTL(controlPlaneClient controlPlaneAPI, namfClient namfNotifier, store AuthContextStore, contextTTL time.Duration) *AuthService {
	return &AuthService{
		store:              store,
		contextTTL:         contextTTL,
		controlPlaneClient: controlPlaneClient,
		namfClient:         namfClient,
	}
}

// SetMetricsRecorder attaches an auth-domain metrics recorder.
// It is safe to call before serving any requests.
func (service *AuthService) SetMetricsRecorder(r authMetricsRecorder) {
	service.recorder = r
}

func (service *AuthService) CreateUEAuthentication(ctx context.Context, supi string, servingNetworkName string, authType string, notificationURI string) (AuthContext, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	if err := validateRequestedAuthType(authType); err != nil {
		if service.recorder != nil {
			if apiErr, ok := err.(APIError); ok {
				service.recorder.RecordAuthFailed(apiErr.Cause)
			}
		}
		return AuthContext{}, err
	}

	authCtxID, err := service.store.NextAuthCtxID()
	if err != nil {
		return AuthContext{}, contextStoreError("auth context allocation failed", err)
	}

	initiateStart := time.Now()
	response, err := service.controlPlaneClient.Initiate(ctx, controlplane.AuthenticationRequest{
		AuthCtxID:          authCtxID,
		SUPI:               supi,
		ServingNetworkName: servingNetworkName,
		AuthType:           authType,
	})
	if err != nil {
		mappedErr := mapInitiateError(err)
		if service.recorder != nil {
			service.recorder.RecordAuthFailed(mappedErr.Cause)
		}
		return AuthContext{}, mappedErr
	}

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

	if err := service.store.Save(context); err != nil {
		return AuthContext{}, contextStoreError("auth context persist failed", err)
	}
	if service.recorder != nil {
		service.recorder.RecordAuthInitiated(context.AuthType)
		service.recorder.RecordAuthDuration(context.AuthType, time.Since(initiateStart).Seconds())
	}
	return context, nil
}

func (service *AuthService) Confirm(ctx context.Context, authCtxID string, resStar string, auts string, eapPayload string) (ConfirmationResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	context, err := service.lookupContextLocked(authCtxID)
	if err != nil {
		return ConfirmationResult{}, err
	}
	if context.Status != authStatusChallengeSent {
		return ConfirmationResult{}, contextNotPendingError()
	}
	if context.AuthType == authTypeFiveGAka && !isValidFiveGAkaConfirmationPayload(resStar, auts) {
		return ConfirmationResult{}, invalidFiveGAkaConfirmationError()
	}
	if auts != "" && context.AuthType != authTypeFiveGAka {
		return ConfirmationResult{}, invalidAutsForAuthTypeError()
	}

	confirmStart := time.Now()
	response, err := service.controlPlaneClient.Confirm(ctx, authCtxID, controlplane.AuthenticationRequest{
		ResStar:    resStar,
		AUTS:       auts,
		EapPayload: eapPayload,
	})
	if err != nil {
		mappedErr := mapConfirmError(err)
		if mappedErr.Cause == authenticationRejectedCause {
			context.Status = "FAILED"
			if context.AuthType == authTypeEapAkaPrime && context.EapSession != nil {
				if mappedErr.EapPayload != "" {
					context.EapSession.Payload = mappedErr.EapPayload
				} else {
					context.EapSession.Payload = eapFailurePayload
				}
			}
			if saveErr := service.store.Save(context); saveErr != nil {
				return ConfirmationResult{}, contextStoreError("auth context update failed", saveErr)
			}
		}
		if service.recorder != nil {
			service.recorder.RecordAuthFailed(mappedErr.Cause)
		}
		return ConfirmationResult{}, mappedErr
	}

	if shouldReissueFiveGAkaChallenge(context, response) {
		context.AuthData = AuthData{
			RAND:      response.RAND,
			AUTN:      response.AUTN,
			HXRESStar: response.HXRESStar,
		}
		context.KSEAF = ""
		context.Status = authStatusChallengeSent
		if err := service.store.Save(context); err != nil {
			return ConfirmationResult{}, contextStoreError("auth context update failed", err)
		}

		if service.recorder != nil {
			service.recorder.RecordSyncFailure(context.AuthType)
		}
		return ConfirmationResult{
			AuthCtxID:  authCtxID,
			SUPI:       context.SUPI,
			AuthResult: authResultSyncFailure,
			AuthData:   &context.AuthData,
			Message:    response.Message,
		}, nil
	}

	if shouldContinueEapSession(context, response) {
		context = initializeEapAkaPrimeContext(context, response.EapChallenge)
		context.KSEAF = ""
		context.Status = authStatusChallengeSent
		if err := service.store.Save(context); err != nil {
			return ConfirmationResult{}, contextStoreError("auth context update failed", err)
		}

		return ConfirmationResult{
			AuthCtxID:  authCtxID,
			SUPI:       context.SUPI,
			AuthResult: authResultOngoing,
			EapSession: context.EapSession,
			Message:    response.Message,
		}, nil
	}

	context.KSEAF = response.KSEAF
	context.Status = authStatusAuthenticated
	if err := service.store.Save(context); err != nil {
		return ConfirmationResult{}, contextStoreError("auth context update failed", err)
	}

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
			logServiceJSON("WARN", "namf notification failed", map[string]any{
				"auth_ctx_id": authCtxID,
				"error":       err.Error(),
			})
		}
	}
	if service.recorder != nil {
		service.recorder.RecordAuthConfirmed(context.AuthType)
		service.recorder.RecordAuthDuration(context.AuthType, time.Since(confirmStart).Seconds())
	}

	return ConfirmationResult{
		AuthCtxID:  authCtxID,
		SUPI:       context.SUPI,
		AuthResult: "SUCCESS",
		KSEAF:      context.KSEAF,
		Message:    response.Message,
	}, nil
}

func isValidFiveGAkaConfirmationPayload(resStar string, auts string) bool {
	hasResStar := resStar != ""
	hasAuts := auts != ""
	return hasResStar != hasAuts
}

func (service *AuthService) Lookup(authCtxID string) (AuthContext, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	return service.lookupContextLocked(authCtxID)
}

func (service *AuthService) Delete(authCtxID string) error {
	service.mu.Lock()
	defer service.mu.Unlock()

	context, err := service.lookupContextLocked(authCtxID)
	if err != nil {
		return err
	}

	deleted, err := service.store.Delete(authCtxID)
	if err != nil {
		return contextStoreError("auth context delete failed", err)
	}
	if !deleted {
		return contextNotFoundError()
	}

	if context.Status == "FAILED" && service.namfClient != nil && context.NotificationURI != "" {
		notification := namf.UEAuthenticationStatusNotification{
			AuthCtxID:          authCtxID,
			SUPI:               context.SUPI,
			AuthType:           context.AuthType,
			ServingNetworkName: context.ServingNetworkName,
			AuthResult:         "FAILURE",
		}
		if notifyErr := service.namfClient.NotifyUEAuthenticationStatus(notification, context.NotificationURI); notifyErr != nil {
			logServiceJSON("WARN", "namf failure notification failed", map[string]any{
				"auth_ctx_id": authCtxID,
				"error":       notifyErr.Error(),
			})
		}
	}
	return nil
}

func (service *AuthService) lookupContextLocked(authCtxID string) (AuthContext, error) {
	context, ok, err := service.store.Get(authCtxID)
	if err != nil {
		return AuthContext{}, contextStoreError("auth context lookup failed", err)
	}
	if !ok {
		return AuthContext{}, contextNotFoundError()
	}
	if service.contextExpired(context) {
		if _, err := service.store.Delete(authCtxID); err != nil {
			return AuthContext{}, contextStoreError("auth context expiry cleanup failed", err)
		}
		return AuthContext{}, contextNotFoundError()
	}
	return context, nil
}

func (service *AuthService) contextExpired(context AuthContext) bool {
	if service.contextTTL <= 0 {
		return false
	}
	if context.CreatedAt.IsZero() {
		return true
	}
	return time.Since(context.CreatedAt) > service.contextTTL
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

func logServiceJSON(level, msg string, fields map[string]any) {
	fields["time"] = time.Now().UTC().Format(time.RFC3339Nano)
	fields["level"] = level
	fields["msg"] = msg
	data, _ := json.Marshal(fields)
	_, _ = fmt.Fprintln(os.Stderr, string(data))
}

func shouldReissueFiveGAkaChallenge(context AuthContext, response controlplane.AuthenticationResponse) bool {
	return context.AuthType == authTypeFiveGAka && response.KSEAF == "" && response.RAND != "" && response.AUTN != "" && response.HXRESStar != ""
}

func shouldContinueEapSession(context AuthContext, response controlplane.AuthenticationResponse) bool {
	return context.AuthType == authTypeEapAkaPrime && response.KSEAF == "" && response.EapChallenge != ""
}
