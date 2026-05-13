package oauth2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// ---- StaticToken tests ----

func TestStaticTokenReturnsConfiguredValue(t *testing.T) {
	ts := NewStaticToken("my-secret-token")
	if ts == nil {
		t.Fatal("NewStaticToken returned nil for non-empty token")
	}
	tok, err := ts.GetToken()
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if tok != "my-secret-token" {
		t.Errorf("GetToken() = %q, want %q", tok, "my-secret-token")
	}
}

func TestStaticTokenTrimsWhitespace(t *testing.T) {
	ts := NewStaticToken("  padded  ")
	tok, _ := ts.GetToken()
	if tok != "padded" {
		t.Errorf("GetToken() = %q, want %q", tok, "padded")
	}
}

func TestStaticTokenNilForEmpty(t *testing.T) {
	if NewStaticToken("") != nil {
		t.Error("NewStaticToken(\"\") should return nil")
	}
	if NewStaticToken("   ") != nil {
		t.Error("NewStaticToken(\"   \") should return nil")
	}
}

// ---- CCFTokenProvider helper ----

func newTokenServer(t *testing.T, accessToken string, expiresIn int, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if got := r.FormValue("grant_type"); got != "client_credentials" {
			t.Errorf("grant_type = %q, want client_credentials", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			body, _ := json.Marshal(tokenResponse{
				AccessToken: accessToken,
				ExpiresIn:   expiresIn,
				TokenType:   "Bearer",
			})
			_, _ = w.Write(body)
		}
	}))
}

// ---- CCFTokenProvider tests ----

func TestCCFTokenProviderFetchesToken(t *testing.T) {
	srv := newTokenServer(t, "fetched-token", 3600, http.StatusOK)
	defer srv.Close()

	provider := NewCCFTokenProvider(srv.URL, "ausf-instance-1", "", "namf-comm", nil)
	tok, err := provider.GetToken()
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if tok != "fetched-token" {
		t.Errorf("GetToken() = %q, want fetched-token", tok)
	}
}

func TestCCFTokenProviderSendsClientIDAndScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if got := r.FormValue("client_id"); got != "ausf-uuid-123" {
			t.Errorf("client_id = %q, want ausf-uuid-123", got)
		}
		if got := r.FormValue("client_secret"); got != "supersecret" {
			t.Errorf("client_secret = %q, want supersecret", got)
		}
		if got := r.FormValue("scope"); got != "namf-comm" {
			t.Errorf("scope = %q, want namf-comm", got)
		}
		body, _ := json.Marshal(tokenResponse{AccessToken: "t", ExpiresIn: 60, TokenType: "Bearer"})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	provider := NewCCFTokenProvider(srv.URL, "ausf-uuid-123", "supersecret", "namf-comm", nil)
	if _, err := provider.GetToken(); err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
}

func TestCCFTokenProviderCachesWithinTTL(t *testing.T) {
	var fetchCount int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&fetchCount, 1)
		body, _ := json.Marshal(tokenResponse{AccessToken: "cached-token", ExpiresIn: 3600, TokenType: "Bearer"})
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	provider := NewCCFTokenProvider(srv.URL, "c", "", "s", nil)
	for i := 0; i < 10; i++ {
		if _, err := provider.GetToken(); err != nil {
			t.Fatalf("GetToken() attempt %d: %v", i+1, err)
		}
	}

	if n := atomic.LoadInt64(&fetchCount); n != 1 {
		t.Errorf("JWKS fetched %d times within TTL, want 1", n)
	}
}

func TestCCFTokenProviderRefreshesAfterExpiry(t *testing.T) {
	var fetchCount int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&fetchCount, 1)
		body, _ := json.Marshal(tokenResponse{AccessToken: "token", ExpiresIn: 0, TokenType: "Bearer"})
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	provider := NewCCFTokenProvider(srv.URL, "c", "", "s", nil)
	// Override cached expiry to past so next call triggers a refresh.
	if _, err := provider.GetToken(); err != nil {
		t.Fatalf("first GetToken(): %v", err)
	}
	provider.mu.Lock()
	provider.expiry = time.Now().Add(-time.Minute) // force expiry
	provider.mu.Unlock()

	if _, err := provider.GetToken(); err != nil {
		t.Fatalf("second GetToken(): %v", err)
	}
	if n := atomic.LoadInt64(&fetchCount); n < 2 {
		t.Errorf("expected at least 2 fetches after expiry, got %d", n)
	}
}

func TestCCFTokenProviderReturnsStaledTokenOnFetchError(t *testing.T) {
	var fetchCount int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt64(&fetchCount, 1)
		if n == 1 {
			body, _ := json.Marshal(tokenResponse{AccessToken: "stale-ok", ExpiresIn: 1, TokenType: "Bearer"})
			_, _ = w.Write(body)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	provider := NewCCFTokenProvider(srv.URL, "c", "", "s", nil)
	// First fetch — succeeds.
	if _, err := provider.GetToken(); err != nil {
		t.Fatalf("first GetToken(): %v", err)
	}
	// Force expiry.
	provider.mu.Lock()
	provider.expiry = time.Now().Add(-time.Minute)
	provider.mu.Unlock()

	// Second fetch fails — should return stale token.
	tok, err := provider.GetToken()
	if err != nil {
		t.Fatalf("expected stale token on error, got error: %v", err)
	}
	if tok != "stale-ok" {
		t.Errorf("stale token = %q, want stale-ok", tok)
	}
}

func TestCCFTokenProviderReturnsErrorWhenNoCache(t *testing.T) {
	provider := NewCCFTokenProvider("http://127.0.0.1:1", "c", "", "s", nil)
	_, err := provider.GetToken()
	if err == nil {
		t.Error("expected error for unreachable token endpoint, got nil")
	}
}

func TestCCFTokenProviderRejectsNonOKResponse(t *testing.T) {
	srv := newTokenServer(t, "", 0, http.StatusUnauthorized)
	defer srv.Close()

	provider := NewCCFTokenProvider(srv.URL, "c", "", "s", nil)
	_, err := provider.GetToken()
	if err == nil {
		t.Error("expected error for HTTP 401 response, got nil")
	}
}

func TestCCFTokenProviderRejectsEmptyAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body, _ := json.Marshal(map[string]string{"token_type": "Bearer"}) // no access_token
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	provider := NewCCFTokenProvider(srv.URL, "c", "", "s", nil)
	_, err := provider.GetToken()
	if err == nil {
		t.Error("expected error for empty access_token, got nil")
	}
}
