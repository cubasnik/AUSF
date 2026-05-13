// Package oauth2 provides lightweight JWT Bearer token claim parsing and
// validation for 5G SBI OAuth2 per TS 29.500 §13.
//
// NOTE: This implementation parses and validates JWT claims (exp, aud, scope)
// but does NOT verify the token signature. Signature verification requires
// fetching the JWKS from NRF — a future enhancement tracked separately.
// In a production deployment, this should be combined with JWKS validation.
package oauth2

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Claims holds the standard JWT claims relevant to 5G SBI access tokens.
type Claims struct {
	Issuer    string   `json:"iss"`
	Subject   string   `json:"sub"`
	Audience  audience `json:"aud"` // RFC 7519: string or array
	ExpiresAt int64    `json:"exp"`
	Scope     string   `json:"scope"` // TS 29.500 §13: space-separated scope string
}

// HasScope reports whether the token grants the given scope string.
func (c *Claims) HasScope(scope string) bool {
	if scope == "" {
		return true
	}
	for _, s := range strings.Fields(c.Scope) {
		if s == scope {
			return true
		}
	}
	return false
}

// HasAudience reports whether the token targets the given audience.
func (c *Claims) HasAudience(aud string) bool {
	if aud == "" {
		return true
	}
	for _, a := range c.Audience {
		if a == aud {
			return true
		}
	}
	return false
}

// ParseClaims decodes a compact JWT and returns its payload claims.
// It does NOT verify the signature — see package-level doc.
func ParseClaims(rawToken string) (*Claims, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT: expected 3 parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid JWT payload encoding: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("invalid JWT payload JSON: %w", err)
	}

	return &claims, nil
}

// ValidateClaims checks expiry, audience, and scope against the expected values.
// Pass empty strings to skip audience/scope checks.
func ValidateClaims(claims *Claims, expectedAudience, expectedScope string, now time.Time) error {
	if claims.ExpiresAt > 0 && now.Unix() >= claims.ExpiresAt {
		return fmt.Errorf("token is expired")
	}

	if !claims.HasAudience(expectedAudience) {
		return fmt.Errorf("token audience does not include %q", expectedAudience)
	}

	if !claims.HasScope(expectedScope) {
		return fmt.Errorf("token does not grant required scope %q", expectedScope)
	}

	return nil
}

// audience is a JSON union type: either a single string or an array of strings
// (both are valid per RFC 7519 §4.1.3).
type audience []string

func (a *audience) UnmarshalJSON(b []byte) error {
	// Try array first
	var arr []string
	if err := json.Unmarshal(b, &arr); err == nil {
		*a = arr
		return nil
	}
	// Fall back to single string
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("aud must be a string or array of strings: %w", err)
	}
	*a = []string{s}
	return nil
}
