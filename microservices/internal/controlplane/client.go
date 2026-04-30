package controlplane

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	maxAttempts = 3
	baseBackoff = 200 * time.Millisecond

	defaultBreakerTimeout             = 10 * time.Second
	defaultBreakerConsecutiveFailures = 5
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	breaker    *circuitBreaker
}

type AuthenticationRequest struct {
	SUPI               string `json:"supi"`
	ServingNetworkName string `json:"servingNetworkName,omitempty"`
	AuthType           string `json:"authType,omitempty"`
	ResStar            string `json:"resStar,omitempty"`
	EapPayload         string `json:"eapPayload,omitempty"`
}

type AuthenticationResponse struct {
	Success            bool   `json:"success"`
	SUPI               string `json:"supi"`
	AuthType           string `json:"authType"`
	ServingNetworkName string `json:"servingNetworkName"`
	RAND               string `json:"rand"`
	AUTN               string `json:"autn"`
	HXRESStar          string `json:"hxresStar"`
	EapChallenge       string `json:"eapChallenge"`
	KSEAF              string `json:"kseaf"`
	Message            string `json:"message"`
	ErrorCode          string `json:"errorCode"`
}

type APIError struct {
	StatusCode int
	Message    string
	ErrorCode  string
}

func (error APIError) Error() string {
	return error.Message
}

func NewClient(baseURL string) *Client {
	return NewClientWithBreaker(baseURL, defaultBreakerConsecutiveFailures, defaultBreakerTimeout)
}

func NewClientWithBreaker(baseURL string, failures int, timeout time.Duration) *Client {
	if failures <= 0 {
		failures = defaultBreakerConsecutiveFailures
	}
	if timeout <= 0 {
		timeout = defaultBreakerTimeout
	}

	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		breaker: newCircuitBreaker(failures, timeout),
	}
}

func (client *Client) Initiate(request AuthenticationRequest) (AuthenticationResponse, error) {
	return client.doJSON(http.MethodPost, "/control-plane/v1/auth/initiate", request)
}

func (client *Client) Confirm(supi string, request AuthenticationRequest) (AuthenticationResponse, error) {
	return client.doJSON(http.MethodPost, "/control-plane/v1/auth/"+supi+"/confirm", request)
}

func (client *Client) Context(supi string) (AuthenticationResponse, error) {
	return client.doJSON(http.MethodGet, "/control-plane/v1/auth/"+supi, nil)
}

func (client *Client) doJSON(method string, path string, payload any) (AuthenticationResponse, error) {
	if !client.breaker.allow() {
		return AuthenticationResponse{}, APIError{
			StatusCode: http.StatusServiceUnavailable,
			Message:    "control-plane circuit breaker is open",
			ErrorCode:  "CONTROL_PLANE_UNAVAILABLE",
		}
	}

	response, err := client.doJSONWithRetries(method, path, payload)
	if err != nil {
		var apiErr APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode < http.StatusInternalServerError {
			client.breaker.onSuccess()
			return AuthenticationResponse{}, err
		}

		client.breaker.onFailure()
		return AuthenticationResponse{}, err
	}

	client.breaker.onSuccess()
	return response, nil
}

func (client *Client) doJSONWithRetries(method string, path string, payload any) (AuthenticationResponse, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return AuthenticationResponse{}, err
		}
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		httpRequest, requestErr := http.NewRequest(method, client.baseURL+path, bytes.NewReader(body))
		if requestErr != nil {
			return AuthenticationResponse{}, requestErr
		}
		if payload != nil {
			httpRequest.Header.Set("Content-Type", "application/json")
		}

		response, requestErr := client.httpClient.Do(httpRequest)
		if requestErr != nil {
			lastErr = requestErr
			if attempt == maxAttempts {
				return AuthenticationResponse{}, requestErr
			}
			time.Sleep(backoffDuration(attempt))
			continue
		}

		result, responseErr := decodeResponse(response)
		if responseErr != nil {
			lastErr = responseErr
			if attempt == maxAttempts || response.StatusCode < 500 {
				return AuthenticationResponse{}, responseErr
			}
			time.Sleep(backoffDuration(attempt))
			continue
		}

		return result, nil
	}

	return AuthenticationResponse{}, lastErr
}

func decodeResponse(response *http.Response) (AuthenticationResponse, error) {
	defer response.Body.Close()

	var result AuthenticationResponse
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return AuthenticationResponse{}, err
	}

	if len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, &result); err != nil {
			return AuthenticationResponse{}, err
		}
	}

	if response.StatusCode >= 400 {
		message := result.Message
		if message == "" {
			message = fmt.Sprintf("control-plane request failed with status %d", response.StatusCode)
		}
		return AuthenticationResponse{}, APIError{StatusCode: response.StatusCode, Message: message, ErrorCode: result.ErrorCode}
	}

	return result, nil
}

func backoffDuration(attempt int) time.Duration {
	return time.Duration(attempt) * baseBackoff
}

type circuitBreaker struct {
	mu                  sync.Mutex
	consecutiveFailures int
	openedAt            time.Time
	failureThreshold    int
	openTimeout         time.Duration
}

func newCircuitBreaker(failureThreshold int, openTimeout time.Duration) *circuitBreaker {
	return &circuitBreaker{
		failureThreshold: failureThreshold,
		openTimeout:      openTimeout,
	}
}

func (breaker *circuitBreaker) allow() bool {
	breaker.mu.Lock()
	defer breaker.mu.Unlock()

	if breaker.openedAt.IsZero() {
		return true
	}
	if time.Since(breaker.openedAt) >= breaker.openTimeout {
		breaker.openedAt = time.Time{}
		breaker.consecutiveFailures = 0
		return true
	}
	return false
}

func (breaker *circuitBreaker) onSuccess() {
	breaker.mu.Lock()
	defer breaker.mu.Unlock()

	breaker.consecutiveFailures = 0
	breaker.openedAt = time.Time{}
}

func (breaker *circuitBreaker) onFailure() {
	breaker.mu.Lock()
	defer breaker.mu.Unlock()

	breaker.consecutiveFailures++
	if breaker.consecutiveFailures >= breaker.failureThreshold {
		breaker.openedAt = time.Now()
	}
}
