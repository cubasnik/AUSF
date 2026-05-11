package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/alexey/ausf/microservices/internal/service"
)

type Handler struct {
	authService       *service.AuthService
	authorization     AuthorizationConfig
	overloadThreshold int
}

type createAuthRequest struct {
	SUPI               string `json:"supiOrSuci"`
	ServingNetworkName string `json:"servingNetworkName"`
	AuthType           string `json:"authType"`
	NotificationURI    string `json:"notificationUri,omitempty"`
}

type confirmRequest struct {
	ResStar    string `json:"resStar"`
	Auts       string `json:"auts"`
	EapPayload string `json:"eapPayload"`
}

func NewHandler(authService *service.AuthService) Handler {
	return NewHandlerWithAuthorization(authService, AuthorizationConfig{})
}

func NewHandlerWithAuthorization(authService *service.AuthService, authorization AuthorizationConfig) Handler {
	return Handler{authService: authService, authorization: authorization}
}

// NewHandlerWithOptions is like NewHandlerWithAuthorization but also sets the
// overload-control threshold (AUSF_OVERLOAD_THRESHOLD).  threshold ≤ 0 uses
// the built-in default of 500 concurrent requests.
func NewHandlerWithOptions(authService *service.AuthService, authorization AuthorizationConfig, overloadThreshold int) Handler {
	return Handler{
		authService:       authService,
		authorization:     authorization,
		overloadThreshold: overloadThreshold,
	}
}

func (handler Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handler.health)
	mux.HandleFunc("/metrics", handler.metrics)
	mux.HandleFunc("/nausf-auth/v1/ue-authentications", handler.createUEAuthentication)
	mux.HandleFunc("/nausf-auth/v1/ue-authentications/", handler.authContextRoutes)
	return withObservability(withOverloadControl(withAuthorization(mux, handler.authorization), handler.overloadThreshold))
}

func (handler Handler) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (handler Handler) createUEAuthentication(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeProblem(writer, http.StatusMethodNotAllowed, "Method not allowed", "use POST for ue-authentications creation", "METHOD_NOT_ALLOWED", request.URL.Path)
		return
	}

	var payload createAuthRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeProblem(writer, http.StatusBadRequest, "Invalid request", "request body is invalid", "MALFORMED_REQUEST", request.URL.Path)
		return
	}
	if strings.TrimSpace(payload.SUPI) == "" || strings.TrimSpace(payload.ServingNetworkName) == "" {
		writeProblem(writer, http.StatusBadRequest, "Invalid request", "supiOrSuci and servingNetworkName are required", "MANDATORY_IE_MISSING", request.URL.Path)
		return
	}
	if !isValidNotificationURI(payload.NotificationURI) {
		writeProblem(writer, http.StatusBadRequest, "Invalid request", "notificationUri must be an absolute http or https URL", "INVALID_NOTIFICATION_URI", request.URL.Path)
		return
	}
	if !isSupportedAuthType(payload.AuthType) {
		writeProblem(writer, http.StatusBadRequest, "Invalid request", "authType must be 5G_AKA or EAP_AKA_PRIME when provided", "UNSUPPORTED_AUTH_TYPE", request.URL.Path)
		return
	}

	context, err := handler.authService.CreateUEAuthentication(request.Context(), payload.SUPI, payload.ServingNetworkName, payload.AuthType, payload.NotificationURI)
	if err != nil {
		apiErr := err.(service.APIError)
		writeProblem(writer, apiErr.StatusCode, "Authentication setup failed", apiErr.Message, apiErr.Cause, request.URL.Path)
		return
	}

	writer.Header().Set("Location", "/nausf-auth/v1/ue-authentications/"+context.AuthCtxID)
	writeJSON(writer, http.StatusCreated, context)
}

func (handler Handler) authContextRoutes(writer http.ResponseWriter, request *http.Request) {
	path := strings.TrimPrefix(request.URL.Path, "/nausf-auth/v1/ue-authentications/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeProblem(writer, http.StatusNotFound, "Resource not found", "authentication context path is invalid", "CONTEXT_NOT_FOUND", request.URL.Path)
		return
	}

	authCtxID := parts[0]
	if len(parts) == 1 && request.Method == http.MethodGet {
		handler.getContext(writer, authCtxID)
		return
	}

	if len(parts) == 1 && request.Method == http.MethodDelete {
		handler.deleteContext(writer, authCtxID)
		return
	}

	if len(parts) == 2 && parts[1] == "5g-aka-confirmation" && request.Method == http.MethodPost {
		handler.confirm(writer, request, authCtxID, false)
		return
	}
	if len(parts) == 2 && parts[1] == "eap-session" && request.Method == http.MethodPost {
		handler.confirm(writer, request, authCtxID, true)
		return
	}
	if len(parts) == 2 && parts[1] == "sor-protection" && request.Method == http.MethodPut {
		handler.sorProtection(writer, request, authCtxID)
		return
	}
	if len(parts) == 2 && parts[1] == "upu-protection" && request.Method == http.MethodPut {
		handler.upuProtection(writer, request, authCtxID)
		return
	}

	writeProblem(writer, http.StatusNotFound, "Resource not found", "requested AUSF sub-resource is not implemented", "RESOURCE_UNKNOWN", request.URL.Path)
}

func (handler Handler) getContext(writer http.ResponseWriter, authCtxID string) {
	context, err := handler.authService.Lookup(authCtxID)
	if err != nil {
		apiErr := err.(service.APIError)
		writeProblem(writer, apiErr.StatusCode, "Context not found", apiErr.Message, apiErr.Cause, "/nausf-auth/v1/ue-authentications/"+authCtxID)
		return
	}
	writeJSON(writer, http.StatusOK, context)
}

func (handler Handler) confirm(writer http.ResponseWriter, request *http.Request, authCtxID string, expectsEapPayload bool) {
	var payload confirmRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeProblem(writer, http.StatusBadRequest, "Invalid request", "request body is invalid", "MALFORMED_REQUEST", request.URL.Path)
		return
	}
	if expectsEapPayload {
		if strings.TrimSpace(payload.EapPayload) == "" || strings.TrimSpace(payload.ResStar) != "" || strings.TrimSpace(payload.Auts) != "" {
			writeProblem(writer, http.StatusBadRequest, "Invalid request", "eapPayload is required for eap-session and resStar must be omitted", "INVALID_CONFIRMATION_PAYLOAD", request.URL.Path)
			return
		}
	} else {
		if strings.TrimSpace(payload.ResStar) == "" && strings.TrimSpace(payload.Auts) == "" && strings.TrimSpace(payload.EapPayload) == "" {
			writeProblem(writer, http.StatusBadRequest, "Invalid request", "resStar or auts is required", "MANDATORY_IE_MISSING", request.URL.Path)
			return
		}

		hasResStar := strings.TrimSpace(payload.ResStar) != ""
		hasAuts := strings.TrimSpace(payload.Auts) != ""
		if hasResStar == hasAuts || strings.TrimSpace(payload.EapPayload) != "" {
			writeProblem(writer, http.StatusBadRequest, "Invalid request", "exactly one of resStar or auts is required for 5g-aka-confirmation and eapPayload must be omitted", "INVALID_CONFIRMATION_PAYLOAD", request.URL.Path)
			return
		}
	}

	result, err := handler.authService.Confirm(request.Context(), authCtxID, payload.ResStar, payload.Auts, payload.EapPayload)
	if err != nil {
		apiErr := err.(service.APIError)
		writeProblemWithEapPayload(writer, apiErr.StatusCode, "Authentication rejected", apiErr.Message, apiErr.Cause, request.URL.Path, apiErr.EapPayload)
		return
	}

	writeJSON(writer, http.StatusOK, result)
}

func (handler Handler) deleteContext(writer http.ResponseWriter, authCtxID string) {
	if err := handler.authService.Delete(authCtxID); err != nil {
		apiErr := err.(service.APIError)
		writeProblem(writer, apiErr.StatusCode, "Context not found", apiErr.Message, apiErr.Cause, "/nausf-auth/v1/ue-authentications/"+authCtxID)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (handler Handler) sorProtection(writer http.ResponseWriter, request *http.Request, authCtxID string) {
	var info service.SoRInfo
	if err := json.NewDecoder(request.Body).Decode(&info); err != nil {
		writeProblem(writer, http.StatusBadRequest, "Invalid request", "request body is invalid", "MALFORMED_REQUEST", request.URL.Path)
		return
	}
	result, err := handler.authService.SoRProtect(request.Context(), authCtxID, info)
	if err != nil {
		apiErr := err.(service.APIError)
		writeProblem(writer, apiErr.StatusCode, "SoR protection failed", apiErr.Message, apiErr.Cause, request.URL.Path)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler Handler) upuProtection(writer http.ResponseWriter, request *http.Request, authCtxID string) {
	var info service.UPUInfo
	if err := json.NewDecoder(request.Body).Decode(&info); err != nil {
		writeProblem(writer, http.StatusBadRequest, "Invalid request", "request body is invalid", "MALFORMED_REQUEST", request.URL.Path)
		return
	}
	result, err := handler.authService.UPUProtect(request.Context(), authCtxID, info)
	if err != nil {
		apiErr := err.(service.APIError)
		writeProblem(writer, apiErr.StatusCode, "UPU protection failed", apiErr.Message, apiErr.Cause, request.URL.Path)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func isValidNotificationURI(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}

	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return false
	}

	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func isSupportedAuthType(raw string) bool {
	raw = strings.TrimSpace(raw)
	return raw == "" || raw == "5G_AKA" || raw == "EAP_AKA_PRIME"
}
