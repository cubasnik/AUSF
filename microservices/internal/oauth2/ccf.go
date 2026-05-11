// Package oauth2 — outbound OAuth2 Client Credentials Flow (CCF).
//
// TokenProvider is the abstraction used by outbound SBI clients to attach a
// Bearer token to every request. Two implementations are provided:
//
//   - StaticToken   — wraps a pre-shared static string (backward-compat).
//   - CCFTokenProvider — fetches access tokens from the NRF token endpoint
//     (POST /oauth2/token, client_credentials grant, TS 29.500 §13 / RFC 6749),
//     caches the result, and refreshes automatically before expiry.
package oauth2

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TokenProvider returns a Bearer token for an outbound SBI call.
// Implementations must be safe for concurrent use.
type TokenProvider interface {
	GetToken() (string, error)
}

// StaticToken is a TokenProvider backed by a fixed pre-shared string.
type StaticToken struct{ token string }

// NewStaticToken wraps a static bearer token string as a TokenProvider.
// Returns nil when the trimmed token is empty so callers can use a nil check.
func NewStaticToken(token string) *StaticToken {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	return &StaticToken{token: token}
}

func (s *StaticToken) GetToken() (string, error) { return s.token, nil }

// tokenResponse is the RFC 6749 §5.1 / TS 29.500 §13 access-token response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"` // seconds; 0 means no expiry hint
	TokenType   string `json:"token_type"`
}

// tokenRefreshMargin is how early (before the stated expiry) a token is
// considered expired and a refresh is triggered.
const tokenRefreshMargin = 30 * time.Second

// defaultTokenTTL is used when the NRF does not provide an expires_in value.
const defaultTokenTTL = time.Hour

// CCFTokenProvider fetches and caches OAuth2 access tokens from an NRF token
// endpoint using the client_credentials grant (TS 29.500 §13 / RFC 6749).
// Cached tokens are reused until tokenRefreshMargin before stated expiry, then
// transparently refreshed. On a refresh error the last valid token is returned
// to avoid disrupting in-flight calls ("graceful degradation").
type CCFTokenProvider struct {
	tokenURL     string
	clientID     string
	clientSecret string
	scope        string
	httpClient   *http.Client

	mu     sync.Mutex
	cached string
	expiry time.Time
}

// NewCCFTokenProvider creates a CCFTokenProvider.
//   - tokenURL    — NRF token endpoint (e.g. "https://nrf:8080/oauth2/token").
//   - clientID    — AUSF NF instance ID used as OAuth2 client_id.
//   - clientSecret — optional; omit with "" if NRF authenticates via NF profile.
//   - scope       — TS 29.500 target-NF service scope (e.g. "namf-comm").
//   - httpClient  — nil uses http.DefaultClient.
func NewCCFTokenProvider(tokenURL, clientID, clientSecret, scope string, httpClient *http.Client) *CCFTokenProvider {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &CCFTokenProvider{
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		scope:        scope,
		httpClient:   httpClient,
	}
}

// GetToken returns a valid access token, fetching a fresh one when the cached
// token is absent or within tokenRefreshMargin of its stated expiry.
func (c *CCFTokenProvider) GetToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return cached token if still valid (with refresh margin).
	if c.cached != "" && time.Now().Before(c.expiry.Add(-tokenRefreshMargin)) {
		return c.cached, nil
	}

	tr, err := c.fetch()
	if err != nil {
		// Graceful degradation: return stale token if we have one.
		if c.cached != "" {
			return c.cached, nil
		}
		return "", err
	}

	c.cached = tr.AccessToken
	if tr.ExpiresIn > 0 {
		c.expiry = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	} else {
		c.expiry = time.Now().Add(defaultTokenTTL)
	}
	return c.cached, nil
}

// fetch performs the actual HTTP token request to the NRF.
func (c *CCFTokenProvider) fetch() (*tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", c.clientID)
	if c.clientSecret != "" {
		form.Set("client_secret", c.clientSecret)
	}
	if c.scope != "" {
		form.Set("scope", c.scope)
	}

	resp, err := c.httpClient.PostForm(c.tokenURL, form)
	if err != nil {
		return nil, fmt.Errorf("CCF token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CCF token endpoint returned HTTP %d", resp.StatusCode)
	}

	const maxBody = 64 << 10 // 64 KiB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("read CCF token response: %w", err)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parse CCF token response: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("CCF token response missing access_token")
	}
	return &tr, nil
}
