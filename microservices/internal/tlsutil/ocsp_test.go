package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/ocsp"
)

// --- helpers -----------------------------------------------------------------

func generateTestCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generateTestCA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("generateTestCA cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("generateTestCA parse: %v", err)
	}
	return cert, key
}

func generateTestLeaf(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, ocspURLs []string) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generateTestLeaf key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "Test Leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		OCSPServer:   ocspURLs,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("generateTestLeaf cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("generateTestLeaf parse: %v", err)
	}
	return cert
}

// startMockOCSPServer starts an httptest server that returns OCSP responses
// with the given status for any leaf cert signed by ca/caKey.
// leaf is captured by pointer and may be set after server creation.
func startMockOCSPServer(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, leafPtr **x509.Certificate, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaf := *leafPtr
		if leaf == nil {
			http.Error(w, "leaf not set", http.StatusInternalServerError)
			return
		}
		tmpl := ocsp.Response{
			Status:       status,
			SerialNumber: leaf.SerialNumber,
			ThisUpdate:   time.Now(),
			NextUpdate:   time.Now().Add(time.Hour),
		}
		respBytes, err := ocsp.CreateResponse(ca, ca, tmpl, caKey)
		if err != nil {
			http.Error(w, "create response error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/ocsp-response")
		_, _ = w.Write(respBytes)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// --- tests -------------------------------------------------------------------

func TestOCSPCheckerSkipsWhenNoOCSPURLs(t *testing.T) {
	ca, caKey := generateTestCA(t)
	leaf := generateTestLeaf(t, ca, caKey, nil) // no OCSP URLs

	checker := NewOCSPChecker()
	if err := checker.Check(leaf, ca); err != nil {
		t.Fatalf("expected nil when no OCSP URLs, got: %v", err)
	}
}

func TestOCSPCheckerGoodCert(t *testing.T) {
	ca, caKey := generateTestCA(t)

	var leaf *x509.Certificate
	srv := startMockOCSPServer(t, ca, caKey, &leaf, ocsp.Good)

	leaf = generateTestLeaf(t, ca, caKey, []string{srv.URL})

	checker := NewOCSPChecker()
	if err := checker.Check(leaf, ca); err != nil {
		t.Fatalf("expected nil for Good OCSP status, got: %v", err)
	}
}

func TestOCSPCheckerRevokedCert(t *testing.T) {
	ca, caKey := generateTestCA(t)

	var leaf *x509.Certificate
	srv := startMockOCSPServer(t, ca, caKey, &leaf, ocsp.Revoked)

	leaf = generateTestLeaf(t, ca, caKey, []string{srv.URL})

	checker := NewOCSPChecker()
	err := checker.Check(leaf, ca)
	if err == nil {
		t.Fatal("expected error for Revoked OCSP status, got nil")
	}
}

func TestOCSPCheckerToleratesUnreachableServer(t *testing.T) {
	ca, caKey := generateTestCA(t)
	leaf := generateTestLeaf(t, ca, caKey, []string{"http://127.0.0.1:1"}) // nothing listening

	checker := NewOCSPChecker()
	if err := checker.Check(leaf, ca); err != nil {
		t.Fatalf("expected nil (best-effort) for unreachable server, got: %v", err)
	}
}

func TestOCSPCheckerToleratesBadResponse(t *testing.T) {
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/ocsp-response")
		_, _ = w.Write([]byte("garbage"))
	}))
	t.Cleanup(badSrv.Close)

	ca, caKey := generateTestCA(t)
	leaf := generateTestLeaf(t, ca, caKey, []string{badSrv.URL})

	checker := NewOCSPChecker()
	if err := checker.Check(leaf, ca); err != nil {
		t.Fatalf("expected nil (best-effort) for invalid OCSP response, got: %v", err)
	}
}

func TestVerifyPeerCertificateWithEmptyChain(t *testing.T) {
	checker := NewOCSPChecker()
	if err := checker.VerifyPeerCertificate(nil, nil); err != nil {
		t.Fatalf("expected nil for empty chain, got: %v", err)
	}
}

func TestVerifyPeerCertificateWithSingleCert(t *testing.T) {
	ca, _ := generateTestCA(t)
	checker := NewOCSPChecker()
	// Single cert in chain (no issuer) — should skip OCSP.
	chain := [][]*x509.Certificate{{ca}}
	if err := checker.VerifyPeerCertificate(nil, chain); err != nil {
		t.Fatalf("expected nil for single-cert chain, got: %v", err)
	}
}

func TestVerifyPeerCertificateRevokedCert(t *testing.T) {
	ca, caKey := generateTestCA(t)

	var leaf *x509.Certificate
	srv := startMockOCSPServer(t, ca, caKey, &leaf, ocsp.Revoked)
	leaf = generateTestLeaf(t, ca, caKey, []string{srv.URL})

	checker := NewOCSPChecker()
	chain := [][]*x509.Certificate{{leaf, ca}}
	err := checker.VerifyPeerCertificate(nil, chain)
	if err == nil {
		t.Fatal("expected error for revoked cert in VerifyPeerCertificate")
	}
}
