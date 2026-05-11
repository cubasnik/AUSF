package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestWithOverloadControl_AdvertisesHeaderOnNormalRequest(t *testing.T) {
	handler := withOverloadControl(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), 500)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if v := w.Header().Get("3gpp-Sbi-Overload-Control"); v == "" {
		t.Fatal("expected 3gpp-Sbi-Overload-Control header on normal response")
	}
	if w.Header().Get("Retry-After") != "" {
		t.Fatal("Retry-After must not appear on non-overload responses")
	}
}

func TestWithOverloadControl_Sheds503WhenThresholdExceeded(t *testing.T) {
	// threshold=1: the first in-flight request fills capacity; a second
	// concurrent request must receive 503.
	const threshold = 1
	started := make(chan struct{})
	release := make(chan struct{})

	blocking := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	})
	handler := withOverloadControl(blocking, threshold)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		req := httptest.NewRequest(http.MethodGet, "/first", nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}()

	// Wait until the first goroutine is inside the handler (inflight == 1).
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first request to start")
	}

	// Second request: inflight becomes 2, threshold is 1 → 503.
	req := httptest.NewRequest(http.MethodGet, "/second", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After on 503")
	}
	if w.Header().Get("3gpp-Sbi-Max-Rsp-Time") == "" {
		t.Fatal("expected 3gpp-Sbi-Max-Rsp-Time on 503")
	}

	close(release)
	wg.Wait()
}

func TestWithOverloadControl_DefaultThreshold(t *testing.T) {
	// threshold=0 must use the built-in default (500) rather than reject everything.
	handler := withOverloadControl(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), 0)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with default threshold, got %d", w.Code)
	}
}
