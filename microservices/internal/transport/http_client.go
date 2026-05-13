package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/alexey/ausf/microservices/internal/tlsutil"
	"golang.org/x/net/http2"
)

// TLSClientConfig configures TLS and HTTP/2 for outbound HTTP clients.
type TLSClientConfig struct {
	CACertFile         string
	InsecureSkipVerify bool
	// ForceH2C forces cleartext HTTP/2 (h2c) when no TLS is configured.
	// When TLS is configured, HTTP/2 is always enabled via ALPN.
	ForceH2C bool
	// ClientCertFile and ClientKeyFile enable mTLS: the client presents its own
	// certificate to the server. Both must be set together.
	ClientCertFile string
	ClientKeyFile  string
	// CertReloadInterval, when > 0, enables hot-reload of the client certificate
	// from disk at most once per interval without recreating the HTTP client.
	// When ≤ 0 the certificate is loaded once at client creation time.
	CertReloadInterval time.Duration
	// OCSPEnabled enables OCSP revocation checking for the peer certificate on
	// every TLS handshake. Checking is best-effort: an unreachable OCSP
	// responder does not cause the connection to fail.
	OCSPEnabled bool
}

// NewHTTPClient returns an HTTP client configured for TLS and/or HTTP/2.
// When TLS is configured, HTTP/2 is negotiated via ALPN (RFC 7301).
// When ForceH2C is true and no TLS is configured, HTTP/2 cleartext (h2c) is used.
func NewHTTPClient(timeout time.Duration, tlsClientConfig TLSClientConfig) (*http.Client, error) {
	// H2C path: cleartext HTTP/2 without TLS.
	if tlsClientConfig.ForceH2C && tlsClientConfig.CACertFile == "" && !tlsClientConfig.InsecureSkipVerify {
		return &http.Client{
			Timeout: timeout,
			Transport: &http2.Transport{
				AllowHTTP: true,
				DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, network, addr)
				},
			},
		}, nil
	}

	tr := http.DefaultTransport.(*http.Transport).Clone()

	needsTLS := tlsClientConfig.CACertFile != "" || tlsClientConfig.InsecureSkipVerify ||
		(tlsClientConfig.ClientCertFile != "" && tlsClientConfig.ClientKeyFile != "")

	if needsTLS {
		tlsConfig := &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: tlsClientConfig.InsecureSkipVerify, //nolint:gosec // opt-in, env-controlled
		}

		if tlsClientConfig.CACertFile != "" {
			caBytes, err := os.ReadFile(tlsClientConfig.CACertFile)
			if err != nil {
				return nil, fmt.Errorf("read CA cert file: %w", err)
			}

			rootCAs, err := x509.SystemCertPool()
			if err != nil || rootCAs == nil {
				rootCAs = x509.NewCertPool()
			}
			if !rootCAs.AppendCertsFromPEM(caBytes) {
				return nil, fmt.Errorf("parse CA cert file: no PEM certificates found")
			}
			tlsConfig.RootCAs = rootCAs
		}

		if tlsClientConfig.ClientCertFile != "" && tlsClientConfig.ClientKeyFile != "" {
			if tlsClientConfig.CertReloadInterval > 0 {
				// Hot-reload path: certificate is re-read from disk on each
				// handshake when the TTL has expired.
				loader := tlsutil.NewCertLoader(
					tlsClientConfig.ClientCertFile,
					tlsClientConfig.ClientKeyFile,
					tlsClientConfig.CertReloadInterval,
				)
				// Eagerly load once to surface misconfiguration at startup.
				if _, err := loader.Get(); err != nil {
					return nil, fmt.Errorf("load client cert/key for mTLS: %w", err)
				}
				tlsConfig.GetClientCertificate = loader.GetClientCertificate
			} else {
				// Static path: load once at client creation time.
				cert, err := tls.LoadX509KeyPair(tlsClientConfig.ClientCertFile, tlsClientConfig.ClientKeyFile)
				if err != nil {
					return nil, fmt.Errorf("load client cert/key for mTLS: %w", err)
				}
				tlsConfig.Certificates = []tls.Certificate{cert}
			}
		}

		if tlsClientConfig.OCSPEnabled {
			checker := tlsutil.NewOCSPChecker()
			tlsConfig.VerifyPeerCertificate = checker.VerifyPeerCertificate
		}

		tr.TLSClientConfig = tlsConfig
	}

	// Always configure HTTP/2 via ALPN for TLS connections; also enables
	// HTTP/2 for non-TLS transports that negotiate upgrade (RFC 9113).
	tr.ForceAttemptHTTP2 = true
	if err := http2.ConfigureTransport(tr); err != nil {
		return nil, fmt.Errorf("configure HTTP/2 transport: %w", err)
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
	}, nil
}
