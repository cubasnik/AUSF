// Package tlsutil provides TLS certificate hot-reload and OCSP revocation
// checking for the AUSF microservice.
package tlsutil

import (
	"crypto/tls"
	"fmt"
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
