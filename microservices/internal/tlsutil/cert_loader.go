// Package tlsutil provides TLS certificate hot-reload and OCSP revocation
// checking for the AUSF microservice.
package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"time"
)

const defaultReloadInterval = 30 * time.Second

// CertLoader reloads a TLS certificate/key pair from disk at most once per
// reloadInterval so that certificate rotation takes effect without a process
// restart.
//
// On a reload error the previously loaded certificate is returned until the
// next successful reload, preventing a hard failure during a rolling cert
// rotation where the new file may not yet be fully written.
type CertLoader struct {
	certFile string
	keyFile  string
	interval time.Duration

	mu       sync.Mutex
	cached   *tls.Certificate
	loadedAt time.Time
}

// NewCertLoader returns a CertLoader that refreshes at most once per interval.
// If interval is ≤ 0 the default interval (30 s) is used.
func NewCertLoader(certFile, keyFile string, interval time.Duration) *CertLoader {
	if interval <= 0 {
		interval = defaultReloadInterval
	}
	return &CertLoader{certFile: certFile, keyFile: keyFile, interval: interval}
}

// Get returns the current certificate, reloading from disk when the cache TTL
// has expired.  Returns an error only if no certificate has ever been loaded
// successfully.
func (l *CertLoader) Get() (*tls.Certificate, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.cached != nil && time.Since(l.loadedAt) < l.interval {
		return l.cached, nil
	}

	cert, err := tls.LoadX509KeyPair(l.certFile, l.keyFile)
	if err != nil {
		if l.cached != nil {
			// Return stale cert rather than hard-failing during rotation while
			// the new cert file is being written.
			return l.cached, nil
		}
		return nil, fmt.Errorf("tlsutil: load cert %q / key %q: %w", l.certFile, l.keyFile, err)
	}

	l.cached = &cert
	l.loadedAt = time.Now()
	l.warnIfExpiringSoon(l.cached)
	return l.cached, nil
}

// GetCertificate implements tls.Config.GetCertificate for inbound TLS servers.
// It returns the current certificate, transparently hot-reloading when needed.
func (l *CertLoader) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return l.Get()
}

// GetClientCertificate implements tls.Config.GetClientCertificate for outbound
// mTLS clients.  It returns the current certificate, transparently hot-reloading
// when needed.
func (l *CertLoader) GetClientCertificate(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	return l.Get()
}

// ForceReload invalidates the cached certificate so that the next call to
// Get() reloads from disk unconditionally, regardless of the configured
// interval.  It is safe to call concurrently.
//
// Useful when an operator sends SIGHUP after deploying a new certificate and
// wants the process to pick it up immediately without waiting for the TTL.
func (l *CertLoader) ForceReload() {
	l.mu.Lock()
	l.loadedAt = time.Time{} // zero value → always older than interval
	l.mu.Unlock()
}

const expiryWarnThreshold = 7 * 24 * time.Hour

// warnIfExpiringSoon writes a JSON-formatted warning to stderr when the
// certificate leaf's NotAfter is within 7 days.  This gives operators advance
// notice to rotate the certificate before it expires.
func (l *CertLoader) warnIfExpiringSoon(cert *tls.Certificate) {
	var notAfter time.Time
	if cert.Leaf != nil {
		notAfter = cert.Leaf.NotAfter
	} else if len(cert.Certificate) > 0 {
		if leaf, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
			notAfter = leaf.NotAfter
		}
	}
	if notAfter.IsZero() {
		return
	}
	remaining := time.Until(notAfter)
	if remaining < expiryWarnThreshold {
		_, _ = fmt.Fprintf(os.Stderr,
			`{"level":"WARN","msg":"TLS certificate approaching expiry","cert_file":%q,"expires_at":%q,"remaining_hours":%.0f}`+"\n",
			l.certFile, notAfter.UTC().Format(time.RFC3339), remaining.Hours(),
		)
	}
}
