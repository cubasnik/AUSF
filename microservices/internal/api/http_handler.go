package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/alexey/ausf/microservices/internal/service"
)

type Handler struct {
	authService *service.AuthService
}

type createAuthRequest struct {
	SUPI               string `json:"supiOrSuci"`
	ServingNetworkName string `json:"servingNetworkName"`
	AuthType           string `json:"authType"`
	NotificationURI    string `json:"notificationUri,omitempty"`
}

type confirmRequest struct {
	ResStar    string `json:"resStar"`
	EapPayload string `json:"eapPayload"`
}

func NewHandler(authService *service.AuthService) Handler {
	return Handler{authService: authService}
}

func (handler Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handler.health)
	mux.HandleFunc("/nausf-auth/v1/ue-authentications", handler.createUEAuthentication)
	mux.HandleFunc("/nausf-auth/v1/ue-authentications/", handler.authContextRoutes)
	return mux
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
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || strings.TrimSpace(payload.SUPI) == "" || strings.TrimSpace(payload.ServingNetworkName) == "" {
		writeProblem(writer, http.StatusBadRequest, "Invalid request", "supiOrSuci and servingNetworkName are required", "MANDATORY_IE_MISSING", request.URL.Path)
		return
	}
	if !isValidNotificationURI(payload.NotificationURI) {
		writeProblem(writer, http.StatusBadRequest, "Invalid request", "notificationUri must be an absolute http or https URL", "INVALID_NOTIFICATION_URI", request.URL.Path)
		return
	}

	context, err := handler.authService.CreateUEAuthentication(payload.SUPI, payload.ServingNetworkName, payload.AuthType, payload.NotificationURI)
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
		handler.confirm(writer, request, authCtxID)
		return
	}
	if len(parts) == 2 && parts[1] == "eap-session" && request.Method == http.MethodPost {
		handler.confirm(writer, request, authCtxID)
		return
	}

	writeProblem(writer, http.StatusNotFound, "Resource not found", "requested AUSF sub-resource is not implemented", "RESOURCE_UNKNOWN", request.URL.Path)
}

func (handler Handler) getContext(writer http.ResponseWriter, authCtxID string) {
	context, ok := handler.authService.Lookup(authCtxID)
	if !ok {
		writeProblem(writer, http.StatusNotFound, "Context not found", "authentication context not found", "CONTEXT_NOT_FOUND", "/nausf-auth/v1/ue-authentications/"+authCtxID)
		return
	}
	writeJSON(writer, http.StatusOK, context)
}

func (handler Handler) confirm(writer http.ResponseWriter, request *http.Request, authCtxID string) {
	var payload confirmRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || (payload.ResStar == "" && payload.EapPayload == "") {
		writeProblem(writer, http.StatusBadRequest, "Invalid request", "resStar or eapPayload is required", "MANDATORY_IE_MISSING", request.URL.Path)
		return
	}

	result, err := handler.authService.Confirm(authCtxID, payload.ResStar, payload.EapPayload)
	if err != nil {
		apiErr := err.(service.APIError)
		writeProblem(writer, apiErr.StatusCode, "Authentication rejected", apiErr.Message, apiErr.Cause, request.URL.Path)
		return
	}

	writeJSON(writer, http.StatusOK, result)
}

func (handler Handler) deleteContext(writer http.ResponseWriter, authCtxID string) {
	if !handler.authService.Delete(authCtxID) {
		writeProblem(writer, http.StatusNotFound, "Context not found", "authentication context not found", "CONTEXT_NOT_FOUND", "/nausf-auth/v1/ue-authentications/"+authCtxID)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
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
