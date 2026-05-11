package tlsutil

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/crypto/ocsp"
)

// OCSPChecker verifies the revocation status of a TLS peer certificate via
// OCSP (RFC 6960).  Checking is best-effort: if all OCSP responders are
// unreachable the connection is allowed through and the failure is not treated
// as fatal.  Use it only when the CA issues certificates with OCSP URLs.
type OCSPChecker struct {
	httpClient *http.Client
}

// NewOCSPChecker returns an OCSPChecker with a 5-second request timeout.
func NewOCSPChecker() *OCSPChecker {
	return &OCSPChecker{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// NewOCSPCheckerWithClient returns an OCSPChecker that uses the given HTTP
// client.  Useful in tests to inject a controlled transport.
func NewOCSPCheckerWithClient(client *http.Client) *OCSPChecker {
	return &OCSPChecker{httpClient: client}
}

// Check verifies the revocation status of leaf against the given issuer.
//
//   - If leaf has no OCSP URLs, Check returns nil (check is skipped).
//   - If all configured OCSP responders are unreachable, Check returns nil
//     (best-effort: prefer availability over hard-fail on transient outages).
//   - If any responder returns a Revoked status, Check returns a non-nil error.
func (c *OCSPChecker) Check(leaf, issuer *x509.Certificate) error {
	if len(leaf.OCSPServer) == 0 {
		return nil
	}

	reqBytes, err := ocsp.CreateRequest(leaf, issuer, &ocsp.RequestOptions{Hash: crypto.SHA256})
	if err != nil {
		return fmt.Errorf("ocsp: create request: %w", err)
	}

	for _, server := range leaf.OCSPServer {
		resp, err := c.httpClient.Post(server, "application/ocsp-request", bytes.NewReader(reqBytes))
		if err != nil {
			// Responder unreachable — try next URL.
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		parsed, err := ocsp.ParseResponseForCert(body, leaf, issuer)
		if err != nil {
			// Unparseable response — try next URL.
			continue
		}

		if parsed.Status == ocsp.Revoked {
			return fmt.Errorf("ocsp: certificate serial %s is revoked (reason %d)",
				leaf.SerialNumber, parsed.RevocationReason)
		}

		// Good or Unknown — revocation check passed.
		return nil
	}

	// All responders were unreachable or returned unusable responses.
	// Allow the connection (best-effort behaviour).
	return nil
}

// VerifyPeerCertificate is a tls.Config.VerifyPeerCertificate callback.
// It checks OCSP revocation for the peer leaf certificate using the verified
// chain supplied by the TLS handshake.
//
// When verifiedChains is empty or contains fewer than two certificates (leaf +
// issuer), the check is skipped and nil is returned.
func (c *OCSPChecker) VerifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	if len(verifiedChains) == 0 || len(verifiedChains[0]) < 2 {
		// No verified chain or self-signed cert — skip OCSP.
		return nil
	}
	leaf := verifiedChains[0][0]
	issuer := verifiedChains[0][1]
	return c.Check(leaf, issuer)
}
