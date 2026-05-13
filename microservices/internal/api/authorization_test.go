package api

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alexey/ausf/microservices/internal/oauth2"
)

// buildTestJWT creates a minimal unsigned JWT for testing authorization middleware.
func buildTestJWT(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, _ := json.Marshal(claims)
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".fakesig"
}

func makeOAuth2Handler(cfg OAuth2Config) http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return withAuthorization(ok, AuthorizationConfig{OAuth2: cfg})
}

func TestOAuth2WithValidTokenAndMatchingAudienceAndScopeAllows(t *testing.T) {
	token := buildTestJWT(map[string]any{
		"aud":   "nausf-auth",
		"scope": "nausf-auth",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	makeOAuth2Handler(OAuth2Config{Enabled: true, ExpectedAudience: "nausf-auth", ExpectedScope: "nausf-auth"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestOAuth2WithExpiredTokenReturns401(t *testing.T) {
	token := buildTestJWT(map[string]any{
		"aud":   "nausf-auth",
		"scope": "nausf-auth",
		"exp":   time.Now().Add(-time.Second).Unix(),
	})

	req := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	makeOAuth2Handler(OAuth2Config{Enabled: true, ExpectedAudience: "nausf-auth", ExpectedScope: "nausf-auth"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestOAuth2WithWrongAudienceReturns401(t *testing.T) {
	token := buildTestJWT(map[string]any{
		"aud":   "nudm-ueau",
		"scope": "nausf-auth",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	makeOAuth2Handler(OAuth2Config{Enabled: true, ExpectedAudience: "nausf-auth"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestOAuth2WithMissingScopeReturns401(t *testing.T) {
	token := buildTestJWT(map[string]any{
		"aud":   "nausf-auth",
		"scope": "nudm-ueau",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	makeOAuth2Handler(OAuth2Config{Enabled: true, ExpectedScope: "nausf-auth"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestOAuth2MissingAuthorizationHeaderReturns401(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", nil)
	rec := httptest.NewRecorder()

	makeOAuth2Handler(OAuth2Config{Enabled: true, ExpectedScope: "nausf-auth"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestOAuth2DisabledFallsBackToStaticToken(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := withAuthorization(ok, AuthorizationConfig{BearerToken: "secret", OAuth2: OAuth2Config{Enabled: false}})

	req := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", nil)
	req2.Header.Set("Authorization", "Bearer wrong")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for wrong static token", rec2.Code)
	}
}

func TestNonSBIPathsAreNotProtectedByOAuth2(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	makeOAuth2Handler(OAuth2Config{Enabled: true, ExpectedScope: "nausf-auth"}).ServeHTTP(rec, req)

	// /healthz is not an SBI path — should pass through without auth
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for non-SBI path", rec.Code)
	}
}

// ---- JWKS signature verification middleware tests ----

// buildSignedES256JWT builds a compact JWT signed with the given EC key.
func buildSignedES256JWT(claims map[string]any, key *ecdsa.PrivateKey, kid string) string {
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": kid, "typ": "JWT"})
	payload, _ := json.Marshal(claims)
	h := base64.RawURLEncoding.EncodeToString(header)
	p := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := h + "." + p
	digest := sha256.Sum256([]byte(signingInput))
	r, s, _ := ecdsa.Sign(rand.Reader, key, digest[:])
	rb, sb := r.Bytes(), s.Bytes()
	sig := make([]byte, 64)
	copy(sig[32-len(rb):32], rb)
	copy(sig[64-len(sb):64], sb)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func startECJWKSServer(t *testing.T, pub *ecdsa.PublicKey, kid string) *httptest.Server {
	t.Helper()
	jwk := map[string]string{
		"kty": "EC", "use": "sig", "alg": "ES256", "kid": kid,
		"crv": "P-256",
		"x":   base64.RawURLEncoding.EncodeToString(pub.X.Bytes()),
		"y":   base64.RawURLEncoding.EncodeToString(pub.Y.Bytes()),
	}
	body, _ := json.Marshal(map[string]any{"keys": []any{jwk}})
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

func makeOAuth2HandlerWithJWKS(cfg OAuth2Config) http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return withAuthorization(ok, AuthorizationConfig{OAuth2: cfg})
}

func TestOAuth2JWKSVerifiesValidSignature(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	srv := startECJWKSServer(t, &priv.PublicKey, "test-kid")
	defer srv.Close()

	provider := oauth2.NewJWKSProvider(srv.URL, time.Minute, nil)
	token := buildSignedES256JWT(map[string]any{
		"aud":   "nausf-auth",
		"scope": "nausf-auth",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}, priv, "test-kid")

	req := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	makeOAuth2HandlerWithJWKS(OAuth2Config{
		Enabled:          true,
		ExpectedAudience: "nausf-auth",
		ExpectedScope:    "nausf-auth",
		JWKSProvider:     provider,
	}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for valid JWKS-signed token", rec.Code)
	}
}

func TestOAuth2JWKSRejectsTamperedSignature(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	otherPriv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader) // different key
	srv := startECJWKSServer(t, &priv.PublicKey, "test-kid")
	defer srv.Close()

	provider := oauth2.NewJWKSProvider(srv.URL, time.Minute, nil)
	// Sign with a different key — JWKS has only priv.PublicKey.
	token := buildSignedES256JWT(map[string]any{
		"aud": "nausf-auth", "scope": "nausf-auth",
		"exp": time.Now().Add(time.Hour).Unix(),
	}, otherPriv, "test-kid")

	req := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	makeOAuth2HandlerWithJWKS(OAuth2Config{
		Enabled:      true,
		JWKSProvider: provider,
	}).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for wrong-key signature", rec.Code)
	}
}

// Ensure unused imports (crypto, big) referenced in helpers are not flagged.
var _ = crypto.SHA256
var _ = new(big.Int)
