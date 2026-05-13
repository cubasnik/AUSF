package oauth2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- helpers -----------------------------------------------------------------

func startIntrospectionServer(t *testing.T, active bool, statusCode int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if statusCode != http.StatusOK {
			http.Error(w, "error", statusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"active": active})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// --- tests -------------------------------------------------------------------

func TestIntrospectorActiveToken(t *testing.T) {
	srv := startIntrospectionServer(t, true, http.StatusOK)
	intr := NewIntrospector(srv.URL, "", "", 0, nil)
	if err := intr.Introspect("some.jwt.token"); err != nil {
		t.Fatalf("expected nil for active token, got: %v", err)
	}
}

func TestIntrospectorRevokedToken(t *testing.T) {
	srv := startIntrospectionServer(t, false, http.StatusOK)
	intr := NewIntrospector(srv.URL, "", "", 0, nil)
	if err := intr.Introspect("some.jwt.token"); err == nil {
		t.Fatal("expected error for inactive token, got nil")
	}
}

func TestIntrospectorCachesActiveResult(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"active": true})
	}))
	t.Cleanup(srv.Close)

	intr := NewIntrospector(srv.URL, "", "", time.Minute, nil)
	_ = intr.Introspect("cached.jwt.token")
	_ = intr.Introspect("cached.jwt.token")

	if calls != 1 {
		t.Fatalf("expected 1 HTTP call due to cache, got %d", calls)
	}
}

func TestIntrospectorCachesRevokedResult(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"active": false})
	}))
	t.Cleanup(srv.Close)

	intr := NewIntrospector(srv.URL, "", "", time.Minute, nil)
	_ = intr.Introspect("revoked.jwt.token")
	_ = intr.Introspect("revoked.jwt.token")

	if calls != 1 {
		t.Fatalf("expected 1 HTTP call due to cache, got %d", calls)
	}
}

func TestIntrospectorExpiredCacheEntryRefetched(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"active": true})
	}))
	t.Cleanup(srv.Close)

	// Very short TTL so the cache entry expires immediately.
	intr := NewIntrospector(srv.URL, "", "", time.Nanosecond, nil)
	_ = intr.Introspect("short-ttl.jwt.token")
	time.Sleep(time.Millisecond)
	_ = intr.Introspect("short-ttl.jwt.token")

	if calls != 2 {
		t.Fatalf("expected 2 HTTP calls after cache expiry, got %d", calls)
	}
}

func TestIntrospectorToleratesUnreachableEndpoint(t *testing.T) {
	intr := NewIntrospector("http://127.0.0.1:1/introspect", "", "", 0, nil)
	if err := intr.Introspect("some.jwt.token"); err != nil {
		t.Fatalf("expected nil (best-effort) for unreachable endpoint, got: %v", err)
	}
}

func TestIntrospectorToleratesBadResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json{{"))
	}))
	t.Cleanup(srv.Close)

	intr := NewIntrospector(srv.URL, "", "", 0, nil)
	if err := intr.Introspect("some.jwt.token"); err != nil {
		t.Fatalf("expected nil (best-effort) for invalid response, got: %v", err)
	}
}

func TestIntrospectorToleratesNon200Response(t *testing.T) {
	srv := startIntrospectionServer(t, false, http.StatusInternalServerError)
	intr := NewIntrospector(srv.URL, "", "", 0, nil)
	if err := intr.Introspect("some.jwt.token"); err != nil {
		t.Fatalf("expected nil (best-effort) for non-200, got: %v", err)
	}
}

func TestIntrospectorUsesBasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "client1" || pass != "s3cr3t" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"active": true})
	}))
	t.Cleanup(srv.Close)

	intr := NewIntrospector(srv.URL, "client1", "s3cr3t", 0, nil)
	if err := intr.Introspect("some.jwt.token"); err != nil {
		t.Fatalf("expected nil with correct Basic Auth, got: %v", err)
	}
}

func TestIntrospectorDifferentTokensNotCrossed(t *testing.T) {
	revokedToken := "revoked.jwt"
	activeToken := "active.jwt"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		token := r.FormValue("token")
		active := token == activeToken
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"active": active})
	}))
	t.Cleanup(srv.Close)

	intr := NewIntrospector(srv.URL, "", "", time.Minute, nil)
	if err := intr.Introspect(activeToken); err != nil {
		t.Fatalf("active token: expected nil, got: %v", err)
	}
	if err := intr.Introspect(revokedToken); err == nil {
		t.Fatal("revoked token: expected error, got nil")
	}
	// Second call — must hit cache, not cross-pollinate.
	if err := intr.Introspect(activeToken); err != nil {
		t.Fatalf("active token (cached): expected nil, got: %v", err)
	}
}
