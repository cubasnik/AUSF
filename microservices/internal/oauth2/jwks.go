// Package oauth2 — JWKS-based JWT signature verification (RFC 7517 / TS 29.500 §13).
//
// JWKSProvider fetches a JSON Web Key Set from an NRF endpoint, caches it with
// a configurable TTL, and verifies JWT signatures using the matching key.
// Supported algorithms: RS256 (PKCS#1v1.5), PS256 (RSA-PSS), ES256 (ECDSA P-256).
package oauth2

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// JWK is a single JSON Web Key (RFC 7517 / RFC 7518).
type JWK struct {
	KeyType   string `json:"kty"` // "RSA" or "EC"
	Use       string `json:"use"` // "sig"
	KeyID     string `json:"kid"` // key identifier matched against JWT header "kid"
	Algorithm string `json:"alg"` // "RS256", "PS256", "ES256"
	// RSA fields
	N string `json:"n"` // base64url-encoded modulus
	E string `json:"e"` // base64url-encoded public exponent
	// EC fields
	Curve string `json:"crv"` // "P-256", "P-384", "P-521"
	X     string `json:"x"`   // base64url-encoded x-coordinate
	Y     string `json:"y"`   // base64url-encoded y-coordinate
}

// JWKSet is a JSON Web Key Set.
type JWKSet struct {
	Keys []JWK `json:"keys"`
}

type cachedSet struct {
	set       *JWKSet
	fetchedAt time.Time
}

// JWKSProvider fetches and caches a JWKS from an NRF endpoint and verifies
// JWT signatures. It is safe for concurrent use.
type JWKSProvider struct {
	url        string
	ttl        time.Duration
	httpClient *http.Client

	mu    sync.RWMutex
	cache *cachedSet
}

// NewJWKSProvider creates a JWKSProvider that fetches JWKS from url and caches
// the result for ttl. If client is nil, http.DefaultClient is used.
func NewJWKSProvider(url string, ttl time.Duration, client *http.Client) *JWKSProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return &JWKSProvider{url: url, ttl: ttl, httpClient: client}
}

// VerifySignature verifies the cryptographic signature of a compact JWT.
// It reads the "kid" and "alg" from the JWT header, fetches (or serves from
// cache) the matching public key from the JWKS endpoint, and verifies the
// signature. Returns nil on success.
func (p *JWKSProvider) VerifySignature(rawToken string) error {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return fmt.Errorf("invalid JWT: expected 3 parts")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("invalid JWT header encoding: %w", err)
	}

	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return fmt.Errorf("invalid JWT header JSON: %w", err)
	}

	set, err := p.getOrFetch()
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}

	key, err := findKey(set, header.KeyID, header.Algorithm)
	if err != nil {
		return err
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("invalid JWT signature encoding: %w", err)
	}

	signingInput := []byte(parts[0] + "." + parts[1])
	return verifySignature(header.Algorithm, key, signingInput, sig)
}

// getOrFetch returns a cached JWKSet if still valid, or fetches a fresh one.
func (p *JWKSProvider) getOrFetch() (*JWKSet, error) {
	p.mu.RLock()
	if p.cache != nil && time.Since(p.cache.fetchedAt) < p.ttl {
		set := p.cache.set
		p.mu.RUnlock()
		return set, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()
	// Double-check after acquiring write lock.
	if p.cache != nil && time.Since(p.cache.fetchedAt) < p.ttl {
		return p.cache.set, nil
	}

	set, err := p.fetch()
	if err != nil {
		return nil, err
	}
	p.cache = &cachedSet{set: set, fetchedAt: time.Now()}
	return set, nil
}

func (p *JWKSProvider) fetch() (*JWKSet, error) {
	resp, err := p.httpClient.Get(p.url) //nolint:noctx // JWKS fetch is background, no request context
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", p.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned HTTP %d", resp.StatusCode)
	}

	const maxBody = 1 << 20 // 1 MiB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("read JWKS response: %w", err)
	}

	var set JWKSet
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, fmt.Errorf("parse JWKS JSON: %w", err)
	}
	return &set, nil
}

// findKey looks up the signing key in set.
// When kid is non-empty, it must match exactly — no alg fallback is attempted.
// When kid is empty, the first key whose alg matches is used.
func findKey(set *JWKSet, kid, alg string) (*JWK, error) {
	if kid != "" {
		for i := range set.Keys {
			k := &set.Keys[i]
			if k.KeyID == kid {
				return k, nil
			}
		}
		return nil, fmt.Errorf("no JWKS key found for kid=%q", kid)
	}
	// No kid in JWT header — match by alg.
	for i := range set.Keys {
		k := &set.Keys[i]
		if k.Algorithm == alg {
			return k, nil
		}
	}
	return nil, fmt.Errorf("no JWKS key found for alg=%q", alg)
}

// verifySignature dispatches to the correct algorithm-specific verifier.
func verifySignature(alg string, jwk *JWK, signingInput, sig []byte) error {
	digest := sha256.Sum256(signingInput)

	switch alg {
	case "RS256":
		pub, err := rsaPublicKey(jwk)
		if err != nil {
			return err
		}
		return rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig)

	case "PS256":
		pub, err := rsaPublicKey(jwk)
		if err != nil {
			return err
		}
		return rsa.VerifyPSS(pub, crypto.SHA256, digest[:], sig, &rsa.PSSOptions{
			SaltLength: rsa.PSSSaltLengthAuto,
		})

	case "ES256":
		pub, err := ecPublicKey(jwk)
		if err != nil {
			return err
		}
		// JWT EC signature encoding: r || s, each component is exactly 32 bytes for P-256.
		if len(sig) != 64 {
			return fmt.Errorf("ES256 signature must be 64 bytes, got %d", len(sig))
		}
		r := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		if !ecdsa.Verify(pub, digest[:], r, s) {
			return fmt.Errorf("ES256 signature verification failed")
		}
		return nil

	default:
		return fmt.Errorf("unsupported JWT algorithm %q (supported: RS256, PS256, ES256)", alg)
	}
}

func rsaPublicKey(jwk *JWK) (*rsa.PublicKey, error) {
	if jwk.KeyType != "RSA" {
		return nil, fmt.Errorf("expected RSA key, got kty=%q", jwk.KeyType)
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("decode JWK 'n': %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("decode JWK 'e': %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	e := int(new(big.Int).SetBytes(eBytes).Int64())
	if e == 0 {
		return nil, fmt.Errorf("invalid RSA public exponent 0")
	}
	return &rsa.PublicKey{N: n, E: e}, nil
}

func ecPublicKey(jwk *JWK) (*ecdsa.PublicKey, error) {
	if jwk.KeyType != "EC" {
		return nil, fmt.Errorf("expected EC key, got kty=%q", jwk.KeyType)
	}
	var curve elliptic.Curve
	switch jwk.Curve {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("unsupported EC curve %q (supported: P-256, P-384, P-521)", jwk.Curve)
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(jwk.X)
	if err != nil {
		return nil, fmt.Errorf("decode JWK 'x': %w", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(jwk.Y)
	if err != nil {
		return nil, fmt.Errorf("decode JWK 'y': %w", err)
	}
	return &ecdsa.PublicKey{
		Curve: curve,
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}, nil
}
