package namf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	maxAttempts = 3
	baseBackoff = 200 * time.Millisecond
)

type Client struct {
	baseURL    string
	httpClient *http.Client
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
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
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
