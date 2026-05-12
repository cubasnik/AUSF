package namf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	internaloauth2 "github.com/alexey/ausf/microservices/internal/oauth2"
	"github.com/alexey/ausf/microservices/internal/transport"
)

const (
	maxAttempts = 1
	baseBackoff = 200 * time.Millisecond
)

type Client struct {
	baseURL     string
	httpClient  *http.Client
	bearerToken string
	tokenSource internaloauth2.TokenProvider
}

// WithTokenSource sets a dynamic token source for outbound Bearer tokens.
// It takes precedence over the static bearerToken if both are set.
func (client *Client) WithTokenSource(ts internaloauth2.TokenProvider) *Client {
	client.tokenSource = ts
	return client
}

func (client *Client) getToken() string {
	if client.tokenSource != nil {
		if tok, err := client.tokenSource.GetToken(); err == nil {
			return tok
		}
	}
	return client.bearerToken
}

type UEAuthenticationStatusNotification struct {
	AuthCtxID          string `json:"authCtxId"`
	SUPI               string `json:"supi"`
	AuthType           string `json:"authType"`
	ServingNetworkName string `json:"servingNetworkName"`
	AuthResult         string `json:"authResult"`
	KSEAF              string `json:"kseaf,omitempty"`
}

func NewClient(baseURL string) *Client {
	client, err := NewClientWithTLS(baseURL, transport.TLSClientConfig{}, "")
	if err != nil {
		panic(err)
	}
	return client
}

func NewClientWithTLS(baseURL string, tlsClientConfig transport.TLSClientConfig, bearerToken string) (*Client, error) {
	httpClient, err := transport.NewHTTPClient(2*time.Second, tlsClientConfig)
	if err != nil {
		return nil, err
	}

	return &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		httpClient:  httpClient,
		bearerToken: strings.TrimSpace(bearerToken),
	}, nil
}

func (client *Client) NotifyUEAuthenticationStatus(notification UEAuthenticationStatusNotification, notificationURI string) error {
	endpoint, ok := client.resolveEndpoint(notification.AuthCtxID, notificationURI)
	if !ok {
		return nil
	}

	body, err := json.Marshal(notification)
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		request, requestErr := http.NewRequest(
			http.MethodPost,
			endpoint,
			bytes.NewReader(body),
		)
		if requestErr != nil {
			return requestErr
		}
		request.Header.Set("Content-Type", "application/json")
		if tok := client.getToken(); tok != "" {
			request.Header.Set("Authorization", "Bearer "+tok)
		}

		response, requestErr := client.httpClient.Do(request)
		if requestErr != nil {
			lastErr = requestErr
			if attempt == maxAttempts {
				return requestErr
			}
			time.Sleep(backoffDuration(attempt))
			continue
		}

		_ = response.Body.Close()
		if response.StatusCode < 500 {
			if response.StatusCode >= 400 {
				return fmt.Errorf("namf notification failed with status %d", response.StatusCode)
			}
			return nil
		}

		lastErr = fmt.Errorf("namf notification failed with status %d", response.StatusCode)
		if attempt == maxAttempts {
			return lastErr
		}
		time.Sleep(backoffDuration(attempt))
	}

	return lastErr
}

func (client *Client) resolveEndpoint(authCtxID string, notificationURI string) (string, bool) {
	if strings.TrimSpace(notificationURI) != "" {
		return strings.ReplaceAll(strings.TrimSpace(notificationURI), "{authCtxId}", authCtxID), true
	}
	if client.baseURL == "" {
		return "", false
	}
	return client.baseURL + "/namf-comm/v1/ue-authentications/" + authCtxID + "/status-notify", true
}

func backoffDuration(attempt int) time.Duration {
	return time.Duration(attempt) * baseBackoff
}
