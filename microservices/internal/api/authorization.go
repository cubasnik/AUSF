package api

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alexey/ausf/microservices/internal/oauth2"
)

// OAuth2Config enables JWT Bearer token validation per TS 29.500 §13.
// When Enabled is true, incoming SBI Bearer tokens are parsed as JWTs and
// validated for expiry, audience, and scope.
// When JWKSProvider is set, the JWT signature is also verified against keys
// fetched from the NRF JWKS endpoint (RS256, PS256, ES256 supported).
// When IntrospectionProvider is set, the token is also checked against the
// NRF introspection endpoint (RFC 7662) to detect revoked tokens.
type OAuth2Config struct {
	Enabled               bool
	ExpectedAudience      string               // e.g. "nausf-auth"; empty = skip audience check
	ExpectedScope         string               // e.g. "nausf-auth"; empty = skip scope check
	JWKSProvider          *oauth2.JWKSProvider // nil = skip signature verification
	IntrospectionProvider *oauth2.Introspector // nil = skip revocation check
}

type AuthorizationConfig struct {
	BearerToken string
	OAuth2      OAuth2Config
}

func withAuthorization(next http.Handler, config AuthorizationConfig) http.Handler {
	// Fast path: no auth configured — skip middleware entirely (opt-in model).
	if !config.OAuth2.Enabled && strings.TrimSpace(config.BearerToken) == "" {
		return next
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !requiresAuthorization(request.URL.Path) {
			next.ServeHTTP(writer, request)
			return
		}

		providedToken, ok := parseBearerToken(request.Header.Get("Authorization"))
		if !ok {
			writer.Header().Set("WWW-Authenticate", "Bearer")
			writeProblem(writer, http.StatusUnauthorized, "Unauthorized", "missing or invalid bearer token", "UNAUTHORIZED", request.URL.Path)
			return
		}

		if config.OAuth2.Enabled {
			if err := validateJWT(providedToken, config.OAuth2); err != nil {
				writer.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
				writeProblem(writer, http.StatusUnauthorized, "Unauthorized", err.Error(), "UNAUTHORIZED", request.URL.Path)
				return
			}
			next.ServeHTTP(writer, request)
			return
		}

		// Static bearer token comparison.
		if subtle.ConstantTimeCompare([]byte(providedToken), []byte(strings.TrimSpace(config.BearerToken))) != 1 {
			writer.Header().Set("WWW-Authenticate", "Bearer")
			writeProblem(writer, http.StatusUnauthorized, "Unauthorized", "missing or invalid bearer token", "UNAUTHORIZED", request.URL.Path)
			return
		}

		next.ServeHTTP(writer, request)
	})
}

func validateJWT(rawToken string, cfg OAuth2Config) error {
	claims, err := oauth2.ParseClaims(rawToken)
	if err != nil {
		return err
	}
	if err := oauth2.ValidateClaims(claims, cfg.ExpectedAudience, cfg.ExpectedScope, time.Now()); err != nil {
		return err
	}
	if cfg.JWKSProvider != nil {
		if err := cfg.JWKSProvider.VerifySignature(rawToken); err != nil {
			return fmt.Errorf("JWT signature verification failed: %w", err)
		}
	}
	if cfg.IntrospectionProvider != nil {
		if err := cfg.IntrospectionProvider.Introspect(rawToken); err != nil {
			return err
		}
	}
	return nil
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
