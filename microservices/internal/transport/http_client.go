package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

type TLSClientConfig struct {
	CACertFile         string
	InsecureSkipVerify bool
}

func NewHTTPClient(timeout time.Duration, tlsClientConfig TLSClientConfig) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()

	if tlsClientConfig.CACertFile != "" || tlsClientConfig.InsecureSkipVerify {
		tlsConfig := &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: tlsClientConfig.InsecureSkipVerify,
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

		transport.TLSClientConfig = tlsConfig
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}, nil
}
