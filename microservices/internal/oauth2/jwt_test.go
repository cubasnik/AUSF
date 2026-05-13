package oauth2

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// buildJWT creates a minimal unsigned JWT with the given claims.
// The signature segment is a placeholder — this is sufficient for
// testing claim parsing, which is all our parser does.
func buildJWT(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, _ := json.Marshal(claims)
	payloadEnc := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + payloadEnc + ".fakesig"
}

func TestParseClaimsReturnsExpectedFields(t *testing.T) {
	exp := time.Now().Add(time.Hour).Unix()
	token := buildJWT(map[string]any{
		"iss":   "https://nrf.example.com",
		"sub":   "ausf-instance-1",
		"aud":   "nausf-auth",
		"exp":   exp,
		"scope": "nausf-auth nudm-ueau",
	})

	claims, err := ParseClaims(token)
	if err != nil {
		t.Fatalf("ParseClaims error: %v", err)
	}
	if claims.Issuer != "https://nrf.example.com" {
		t.Errorf("iss = %q, want https://nrf.example.com", claims.Issuer)
	}
	if claims.ExpiresAt != exp {
		t.Errorf("exp = %d, want %d", claims.ExpiresAt, exp)
	}
	if !claims.HasAudience("nausf-auth") {
		t.Errorf("HasAudience(nausf-auth) = false, want true")
	}
	if !claims.HasScope("nausf-auth") {
		t.Errorf("HasScope(nausf-auth) = false, want true")
	}
	if !claims.HasScope("nudm-ueau") {
		t.Errorf("HasScope(nudm-ueau) = false, want true")
	}
}

func TestParseClaimsHandlesArrayAudience(t *testing.T) {
	token := buildJWT(map[string]any{
		"aud": []string{"nausf-auth", "nudm-ueau"},
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	claims, err := ParseClaims(token)
	if err != nil {
		t.Fatalf("ParseClaims error: %v", err)
	}
	if !claims.HasAudience("nausf-auth") {
		t.Errorf("HasAudience(nausf-auth) = false, want true")
	}
	if !claims.HasAudience("nudm-ueau") {
		t.Errorf("HasAudience(nudm-ueau) = false, want true")
	}
	if claims.HasAudience("unknown") {
		t.Errorf("HasAudience(unknown) = true, want false")
	}
}

func TestParseClaimsRejectsMalformedToken(t *testing.T) {
	cases := []string{"", "only-two.parts", "bad.~~~.sig"}
	for _, raw := range cases {
		if _, err := ParseClaims(raw); err == nil {
			t.Errorf("ParseClaims(%q) expected error, got nil", raw)
		}
	}
}

func TestValidateClaimsRejectsExpiredToken(t *testing.T) {
	claims := &Claims{ExpiresAt: time.Now().Add(-time.Second).Unix()}
	err := ValidateClaims(claims, "", "", time.Now())
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected expired error, got %v", err)
	}
}

func TestValidateClaimsAcceptsNoExpiry(t *testing.T) {
	claims := &Claims{ExpiresAt: 0}
	if err := ValidateClaims(claims, "", "", time.Now()); err != nil {
		t.Errorf("expected no error for zero exp, got %v", err)
	}
}

func TestValidateClaimsRejectsWrongAudience(t *testing.T) {
	claims := &Claims{Audience: audience{"nausf-auth"}, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	err := ValidateClaims(claims, "nudm-ueau", "", time.Now())
	if err == nil || !strings.Contains(err.Error(), "audience") {
		t.Errorf("expected audience error, got %v", err)
	}
}

func TestValidateClaimsRejectsMissingScope(t *testing.T) {
	claims := &Claims{Scope: "nudm-ueau", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	err := ValidateClaims(claims, "", "nausf-auth", time.Now())
	if err == nil || !strings.Contains(err.Error(), "scope") {
		t.Errorf("expected scope error, got %v", err)
	}
}

func TestValidateClaimsPassesValidToken(t *testing.T) {
	claims := &Claims{
		Audience:  audience{"nausf-auth"},
		Scope:     "nausf-auth nudm-ueau",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	if err := ValidateClaims(claims, "nausf-auth", "nausf-auth", time.Now()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestSkipsAudienceAndScopeWhenEmpty(t *testing.T) {
	claims := &Claims{Scope: "", Audience: nil, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	if err := ValidateClaims(claims, "", "", time.Now()); err != nil {
		t.Errorf("expected no error when audience/scope empty, got %v", err)
	}
}
