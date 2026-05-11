package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeCertKeyPair generates a self-signed ECDSA certificate with the given
// serial number and writes it to certFile / keyFile (PEM).
func writeCertKeyPair(t *testing.T, certFile, keyFile string, serial int64) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

func setupCertFiles(t *testing.T, serial int64) (certFile, keyFile string) {
	t.Helper()
	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	writeCertKeyPair(t, certFile, keyFile, serial)
	return certFile, keyFile
}

// leafSerial parses the SerialNumber of the first certificate in a tls.Certificate.
func leafSerial(t *testing.T, cert *tls.Certificate) *big.Int {
	t.Helper()
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	return parsed.SerialNumber
}

func TestCertLoaderLoadsCert(t *testing.T) {
	certFile, keyFile := setupCertFiles(t, 42)
	loader := NewCertLoader(certFile, keyFile, time.Hour)

	cert, err := loader.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cert == nil {
		t.Fatal("Get returned nil cert")
	}
	if leafSerial(t, cert).Int64() != 42 {
		t.Fatalf("serial = %v, want 42", leafSerial(t, cert))
	}
}

func TestCertLoaderCachesWithinTTL(t *testing.T) {
	certFile, keyFile := setupCertFiles(t, 1)
	loader := NewCertLoader(certFile, keyFile, time.Hour)

	cert1, _ := loader.Get()

	// Overwrite the files — but the cache TTL hasn't expired.
	writeCertKeyPair(t, certFile, keyFile, 2)

	cert2, _ := loader.Get()

	// Should still be the same pointer (cached).
	if cert1 != cert2 {
		t.Fatal("expected cached cert to be returned within TTL")
	}
}

func TestCertLoaderReloadsAfterTTL(t *testing.T) {
	certFile, keyFile := setupCertFiles(t, 10)
	// Use a very short TTL so we can expire it in the test without sleeping long.
	loader := NewCertLoader(certFile, keyFile, time.Millisecond)

	cert1, _ := loader.Get()
	if leafSerial(t, cert1).Int64() != 10 {
		t.Fatalf("initial serial = %v, want 10", leafSerial(t, cert1))
	}

	// Wait for the TTL to expire, then overwrite the cert files.
	time.Sleep(5 * time.Millisecond)
	writeCertKeyPair(t, certFile, keyFile, 20)

	cert2, err := loader.Get()
	if err != nil {
		t.Fatalf("Get after reload: %v", err)
	}
	if leafSerial(t, cert2).Int64() != 20 {
		t.Fatalf("reloaded serial = %v, want 20", leafSerial(t, cert2))
	}
}

func TestCertLoaderReturnsStaleCertOnReloadError(t *testing.T) {
	certFile, keyFile := setupCertFiles(t, 5)
	loader := NewCertLoader(certFile, keyFile, time.Millisecond)

	cert1, _ := loader.Get()

	// Wait for TTL, then corrupt the cert file.
	time.Sleep(5 * time.Millisecond)
	if err := os.WriteFile(certFile, []byte("not a cert"), 0o600); err != nil {
		t.Fatalf("corrupt cert file: %v", err)
	}

	// Should return stale cert, not an error.
	cert2, err := loader.Get()
	if err != nil {
		t.Fatalf("expected stale cert on reload error, got error: %v", err)
	}
	if cert2 != cert1 {
		t.Fatal("expected stale cert pointer to be returned on reload error")
	}
}

func TestCertLoaderReturnsErrorWhenNeverLoaded(t *testing.T) {
	loader := NewCertLoader("/nonexistent/cert.pem", "/nonexistent/key.pem", time.Hour)
	_, err := loader.Get()
	if err == nil {
		t.Fatal("expected error when cert file does not exist")
	}
}

func TestCertLoaderGetCertificate(t *testing.T) {
	certFile, keyFile := setupCertFiles(t, 7)
	loader := NewCertLoader(certFile, keyFile, time.Hour)
	cert, err := loader.GetCertificate(nil)
	if err != nil || cert == nil {
		t.Fatalf("GetCertificate: cert=%v err=%v", cert, err)
	}
}

func TestCertLoaderGetClientCertificate(t *testing.T) {
	certFile, keyFile := setupCertFiles(t, 8)
	loader := NewCertLoader(certFile, keyFile, time.Hour)
	cert, err := loader.GetClientCertificate(nil)
	if err != nil || cert == nil {
		t.Fatalf("GetClientCertificate: cert=%v err=%v", cert, err)
	}
}
