package oauth2

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// signRS256 builds and signs a compact JWT with the given RSA private key.
func signRS256(header, payload []byte, key *rsa.PrivateKey) string {
	h := base64.RawURLEncoding.EncodeToString(header)
	p := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := h + "." + p
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		panic(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// signPS256 builds and signs a compact JWT with RSA-PSS.
func signPS256(header, payload []byte, key *rsa.PrivateKey) string {
	h := base64.RawURLEncoding.EncodeToString(header)
	p := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := h + "." + p
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPSS(rand.Reader, key, crypto.SHA256, digest[:], nil)
	if err != nil {
		panic(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// signES256 builds and signs a compact JWT with ECDSA P-256.
// JWT EC signature format: r || s (each 32 bytes), not DER.
func signES256(header, payload []byte, key *ecdsa.PrivateKey) string {
	h := base64.RawURLEncoding.EncodeToString(header)
	p := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := h + "." + p
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		panic(err)
	}
	// Pad each component to 32 bytes.
	rb := r.Bytes()
	sb := s.Bytes()
	sig := make([]byte, 64)
	copy(sig[32-len(rb):32], rb)
	copy(sig[64-len(sb):64], sb)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// jwksFor builds a JWKS JSON body from a JWK slice.
func jwksFor(keys []JWK) []byte {
	body, _ := json.Marshal(JWKSet{Keys: keys})
	return body
}

// jwtHeader produces a base64url-decoded JWT header for the given alg/kid.
func jwtHeader(alg, kid string) []byte {
	m := map[string]string{"alg": alg, "typ": "JWT"}
	if kid != "" {
		m["kid"] = kid
	}
	b, _ := json.Marshal(m)
	return b
}

// jwtPayload produces a minimal JWT payload with exp 1 hour from now.
func jwtPayload() []byte {
	b, _ := json.Marshal(map[string]any{
		"iss": "https://nrf.example.com",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	return b
}

// jwkFromRSA builds a JWK for an RSA public key.
func jwkFromRSA(pub *rsa.PublicKey, kid, alg string) JWK {
	e := new(big.Int).SetInt64(int64(pub.E))
	return JWK{
		KeyType:   "RSA",
		Use:       "sig",
		KeyID:     kid,
		Algorithm: alg,
		N:         base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:         base64.RawURLEncoding.EncodeToString(e.Bytes()),
	}
}

// jwkFromEC builds a JWK for an EC public key.
func jwkFromEC(pub *ecdsa.PublicKey, kid string) JWK {
	return JWK{
		KeyType:   "EC",
		Use:       "sig",
		KeyID:     kid,
		Algorithm: "ES256",
		Curve:     "P-256",
		X:         base64.RawURLEncoding.EncodeToString(pub.X.Bytes()),
		Y:         base64.RawURLEncoding.EncodeToString(pub.Y.Bytes()),
	}
}

// mockJWKSServer starts a test server that serves the given JWKS body.
func mockJWKSServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

// ---- RS256 tests ----

func TestJWKSProviderVerifiesRS256Signature(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwk := jwkFromRSA(&priv.PublicKey, "rs256-key-1", "RS256")
	srv := mockJWKSServer(t, jwksFor([]JWK{jwk}))
	defer srv.Close()

	token := signRS256(jwtHeader("RS256", "rs256-key-1"), jwtPayload(), priv)
	provider := NewJWKSProvider(srv.URL, time.Minute, nil)

	if err := provider.VerifySignature(token); err != nil {
		t.Errorf("RS256 verify failed: %v", err)
	}
}

func TestJWKSProviderRejectsRS256TamperedPayload(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwk := jwkFromRSA(&priv.PublicKey, "rs256-key-1", "RS256")
	srv := mockJWKSServer(t, jwksFor([]JWK{jwk}))
	defer srv.Close()

	token := signRS256(jwtHeader("RS256", "rs256-key-1"), jwtPayload(), priv)
	// Tamper: replace payload with a different base64 value.
	parts := splitToken(token)
	parts[1] = base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"attacker","exp":9999999999}`))
	tampered := parts[0] + "." + parts[1] + "." + parts[2]

	provider := NewJWKSProvider(srv.URL, time.Minute, nil)
	if err := provider.VerifySignature(tampered); err == nil {
		t.Error("expected signature verification failure for tampered payload, got nil")
	}
}

// ---- PS256 tests ----

func TestJWKSProviderVerifiesPS256Signature(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwk := jwkFromRSA(&priv.PublicKey, "ps256-key-1", "PS256")
	srv := mockJWKSServer(t, jwksFor([]JWK{jwk}))
	defer srv.Close()

	token := signPS256(jwtHeader("PS256", "ps256-key-1"), jwtPayload(), priv)
	provider := NewJWKSProvider(srv.URL, time.Minute, nil)

	if err := provider.VerifySignature(token); err != nil {
		t.Errorf("PS256 verify failed: %v", err)
	}
}

// ---- ES256 tests ----

func TestJWKSProviderVerifiesES256Signature(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	jwk := jwkFromEC(&priv.PublicKey, "ec256-key-1")
	srv := mockJWKSServer(t, jwksFor([]JWK{jwk}))
	defer srv.Close()

	token := signES256(jwtHeader("ES256", "ec256-key-1"), jwtPayload(), priv)
	provider := NewJWKSProvider(srv.URL, time.Minute, nil)

	if err := provider.VerifySignature(token); err != nil {
		t.Errorf("ES256 verify failed: %v", err)
	}
}

func TestJWKSProviderRejectsES256TamperedPayload(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	jwk := jwkFromEC(&priv.PublicKey, "ec256-key-1")
	srv := mockJWKSServer(t, jwksFor([]JWK{jwk}))
	defer srv.Close()

	token := signES256(jwtHeader("ES256", "ec256-key-1"), jwtPayload(), priv)
	parts := splitToken(token)
	parts[1] = base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"attacker"}`))
	tampered := parts[0] + "." + parts[1] + "." + parts[2]

	provider := NewJWKSProvider(srv.URL, time.Minute, nil)
	if err := provider.VerifySignature(tampered); err == nil {
		t.Error("expected signature verification failure for tampered ES256 payload")
	}
}

// ---- Key selection tests ----

func TestJWKSProviderSelectsKeyByKid(t *testing.T) {
	priv1, _ := rsa.GenerateKey(rand.Reader, 2048)
	priv2, _ := rsa.GenerateKey(rand.Reader, 2048)
	keys := []JWK{
		jwkFromRSA(&priv1.PublicKey, "key-1", "RS256"),
		jwkFromRSA(&priv2.PublicKey, "key-2", "RS256"),
	}
	srv := mockJWKSServer(t, jwksFor(keys))
	defer srv.Close()

	// Token signed with priv2 but kid=key-2 — should pick priv2 key.
	token := signRS256(jwtHeader("RS256", "key-2"), jwtPayload(), priv2)
	provider := NewJWKSProvider(srv.URL, time.Minute, nil)

	if err := provider.VerifySignature(token); err != nil {
		t.Errorf("key selection by kid failed: %v", err)
	}
}

func TestJWKSProviderRejectsUnknownKid(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwk := jwkFromRSA(&priv.PublicKey, "known-kid", "RS256")
	srv := mockJWKSServer(t, jwksFor([]JWK{jwk}))
	defer srv.Close()

	token := signRS256(jwtHeader("RS256", "unknown-kid"), jwtPayload(), priv)
	provider := NewJWKSProvider(srv.URL, time.Minute, nil)

	if err := provider.VerifySignature(token); err == nil {
		t.Error("expected error for unknown kid, got nil")
	}
}

// ---- Cache TTL tests ----

func TestJWKSProviderCachesKeySet(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwk := jwkFromRSA(&priv.PublicKey, "cache-key", "RS256")
	fetchCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetchCount++
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(JWKSet{Keys: []JWK{jwk}})
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	provider := NewJWKSProvider(srv.URL, time.Minute, nil)
	token := signRS256(jwtHeader("RS256", "cache-key"), jwtPayload(), priv)

	for i := 0; i < 5; i++ {
		if err := provider.VerifySignature(token); err != nil {
			t.Fatalf("verify attempt %d: %v", i+1, err)
		}
	}

	if fetchCount != 1 {
		t.Errorf("JWKS fetched %d times in TTL window, want 1", fetchCount)
	}
}

func TestJWKSProviderRefetchesAfterTTLExpires(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwk := jwkFromRSA(&priv.PublicKey, "ttl-key", "RS256")
	fetchCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetchCount++
		body, _ := json.Marshal(JWKSet{Keys: []JWK{jwk}})
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	provider := NewJWKSProvider(srv.URL, 1*time.Millisecond, nil) // very short TTL
	token := signRS256(jwtHeader("RS256", "ttl-key"), jwtPayload(), priv)

	if err := provider.VerifySignature(token); err != nil {
		t.Fatalf("first verify: %v", err)
	}
	time.Sleep(5 * time.Millisecond) // let cache expire
	if err := provider.VerifySignature(token); err != nil {
		t.Fatalf("second verify: %v", err)
	}

	if fetchCount < 2 {
		t.Errorf("expected at least 2 JWKS fetches after TTL expiry, got %d", fetchCount)
	}
}

// ---- Error handling ----

func TestJWKSProviderRejectsUnsupportedAlgorithm(t *testing.T) {
	srv := mockJWKSServer(t, jwksFor([]JWK{{KeyType: "oct", KeyID: "hmac", Algorithm: "HS256"}}))
	defer srv.Close()

	header, _ := json.Marshal(map[string]string{"alg": "HS256", "kid": "hmac"})
	payload, _ := json.Marshal(map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	token := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload) + ".fakesig"

	provider := NewJWKSProvider(srv.URL, time.Minute, nil)
	if err := provider.VerifySignature(token); err == nil {
		t.Error("expected error for unsupported alg HS256, got nil")
	}
}

func TestJWKSProviderRejectsUnreachableEndpoint(t *testing.T) {
	provider := NewJWKSProvider("http://127.0.0.1:1", time.Minute, nil)
	token := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`)) +
		"." + base64.RawURLEncoding.EncodeToString([]byte(`{}`)) + ".sig"

	if err := provider.VerifySignature(token); err == nil {
		t.Error("expected error for unreachable JWKS endpoint, got nil")
	}
}

// splitToken splits a compact JWT into its 3 base64url segments.
func splitToken(token string) []string {
	parts := make([]string, 0, 3)
	start := 0
	dots := 0
	for i, c := range token {
		if c == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
			dots++
			if dots == 2 {
				parts = append(parts, token[start:])
				return parts
			}
		}
	}
	return parts
}
