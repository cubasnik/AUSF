package api

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// TestH2cServerAcceptsHTTP2CleartextRequest verifies that the handler can be
// wrapped with h2c and served over a plain TCP connection using HTTP/2 (no TLS).
// This is required for 5G SBI compliance per TS 29.500.
func TestH2cServerAcceptsHTTP2CleartextRequest(t *testing.T) {
	underlying := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Errorf("ProtoMajor = %d, want 2", r.ProtoMajor)
		}
		w.WriteHeader(http.StatusOK)
	})

	h2cHandler := h2c.NewHandler(underlying, &http2.Server{})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}

	server := &http.Server{Handler: h2cHandler}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	// Use an HTTP/2 transport that allows cleartext (h2c).
	transport := &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(_ context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return net.Dial(network, addr)
		},
	}

	client := &http.Client{Transport: transport}
	resp, err := client.Get("http://" + listener.Addr().String() + "/healthz")
	if err != nil {
		t.Fatalf("h2c GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.Proto != "HTTP/2.0" {
		t.Errorf("proto = %q, want HTTP/2.0", resp.Proto)
	}
}

// TestH2cHandlerPreservesHTTP1Compatibility verifies that a plain HTTP/1.1
// request is still served correctly when the handler is wrapped with h2c.
func TestH2cHandlerPreservesHTTP1Compatibility(t *testing.T) {
	underlying := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	h2cHandler := h2c.NewHandler(underlying, &http2.Server{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h2cHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
}
