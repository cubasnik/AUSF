// Package oauth2 — RFC 7662 token introspection.
//
// Introspector calls an NRF introspection endpoint to verify that a JWT
// presented to the AUSF is still active (i.e. not revoked). Results are
// cached by the SHA-256 of the raw token to avoid a round-trip on every
// request. Checking is best-effort: an unreachable endpoint does not cause
// authentication to fail.
package oauth2

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// introspectionResponse is the RFC 7662 §2.2 JSON response body.
type introspectionResponse struct {
	Active bool `json:"active"`
}

type introspectionCacheEntry struct {
	active    bool
	expiresAt time.Time
}

const defaultIntrospectionCacheTTL = 60 * time.Second

// Introspector calls an RFC 7662 token introspection endpoint to check
// whether a JWT is currently active. It authenticates to the endpoint using
// HTTP Basic Auth (client_id / client_secret).
//
// Positive results (active=true) are cached for cacheTTL to limit round-trips.
// Negative results (active=false, i.e. revoked) are also cached so that a
// revoked token continues to be rejected without hitting the endpoint again.
//
// On network error or unexpected response the call is treated as best-effort
// and Introspect returns nil (pass-through), consistent with the OCSP policy.
type Introspector struct {
	endpointURL  string
	clientID     string
	clientSecret string
	cacheTTL     time.Duration
	httpClient   *http.Client

	mu    sync.Mutex
	cache map[[32]byte]introspectionCacheEntry
}

// NewIntrospector creates an Introspector.
//   - endpointURL  — RFC 7662 introspection endpoint (e.g. "https://nrf/oauth2/introspect").
//   - clientID     — used for HTTP Basic Auth; empty = no auth header sent.
//   - clientSecret — may be empty when the NRF authenticates via NF profile only.
//   - cacheTTL     — positive-result cache lifetime; ≤ 0 uses 60 s default.
//   - httpClient   — nil uses http.DefaultClient.
func NewIntrospector(endpointURL, clientID, clientSecret string, cacheTTL time.Duration, httpClient *http.Client) *Introspector {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if cacheTTL <= 0 {
		cacheTTL = defaultIntrospectionCacheTTL
	}
	return &Introspector{
		endpointURL:  endpointURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		cacheTTL:     cacheTTL,
		httpClient:   httpClient,
		cache:        make(map[[32]byte]introspectionCacheEntry),
	}
}

// Introspect checks whether rawToken is currently active at the NRF.
// Returns nil when active, an error when revoked or inactive.
// On endpoint unreachability or unexpected response returns nil (best-effort).
func (intr *Introspector) Introspect(rawToken string) error {
	key := sha256.Sum256([]byte(rawToken))

	// Check cache under the lock, then release before the HTTP call.
	intr.mu.Lock()
	entry, hit := intr.cache[key]
	intr.mu.Unlock()

	if hit && time.Now().Before(entry.expiresAt) {
		if !entry.active {
			return fmt.Errorf("token has been revoked")
		}
		return nil
	}

	active, err := intr.callEndpoint(rawToken)
	if err != nil {
		// Best-effort: don't block auth on infrastructure issues.
		return nil
	}

	intr.mu.Lock()
	intr.cache[key] = introspectionCacheEntry{
		active:    active,
		expiresAt: time.Now().Add(intr.cacheTTL),
	}
	intr.mu.Unlock()

	if !active {
		return fmt.Errorf("token has been revoked")
	}
	return nil
}

// callEndpoint performs the RFC 7662 POST request and returns active status.
func (intr *Introspector) callEndpoint(rawToken string) (bool, error) {
	form := url.Values{}
	form.Set("token", rawToken)
	form.Set("token_type_hint", "access_token")

	req, err := http.NewRequest(http.MethodPost, intr.endpointURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, fmt.Errorf("build introspection request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if intr.clientID != "" {
		req.SetBasicAuth(intr.clientID, intr.clientSecret)
	}

	resp, err := intr.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("introspection request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("introspection endpoint returned HTTP %d", resp.StatusCode)
	}

	const maxBody = 64 << 10 // 64 KiB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return false, fmt.Errorf("read introspection response: %w", err)
	}

	var ir introspectionResponse
	if err := json.Unmarshal(body, &ir); err != nil {
		return false, fmt.Errorf("parse introspection response: %w", err)
	}
	return ir.Active, nil
}
