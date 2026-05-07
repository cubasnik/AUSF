package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

type AuthorizationConfig struct {
	BearerToken string
}

func withAuthorization(next http.Handler, config AuthorizationConfig) http.Handler {
	expectedToken := strings.TrimSpace(config.BearerToken)
	if expectedToken == "" {
		return next
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !requiresAuthorization(request.URL.Path) {
			next.ServeHTTP(writer, request)
			return
		}

		providedToken, ok := parseBearerToken(request.Header.Get("Authorization"))
		if !ok || subtle.ConstantTimeCompare([]byte(providedToken), []byte(expectedToken)) != 1 {
			writer.Header().Set("WWW-Authenticate", "Bearer")
			writeProblem(writer, http.StatusUnauthorized, "Unauthorized", "missing or invalid bearer token", "UNAUTHORIZED", request.URL.Path)
			return
		}

		next.ServeHTTP(writer, request)
	})
}

func requiresAuthorization(path string) bool {
	return path == "/nausf-auth/v1/ue-authentications" || strings.HasPrefix(path, "/nausf-auth/v1/ue-authentications/")
}

func parseBearerToken(header string) (string, bool) {
	trimmed := strings.TrimSpace(header)
	if trimmed == "" || !strings.HasPrefix(trimmed, "Bearer ") {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(trimmed, "Bearer "))
	return token, token != ""
}
