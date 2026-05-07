package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alexey/ausf/microservices/internal/tracing"
	"github.com/alexey/ausf/microservices/internal/transport"
)

const (
	maxAttempts = 3
	baseBackoff = 200 * time.Millisecond

	defaultBreakerTimeout             = 10 * time.Second
	defaultBreakerConsecutiveFailures = 5
)

type Client struct {
	baseURL     string
	httpClient  *http.Client
	breaker     *circuitBreaker
	bearerToken string
}

type TLSClientConfig = transport.TLSClientConfig

type AuthenticationRequest struct {
	AuthCtxID          string `json:"authCtxId,omitempty"`
	SUPI               string `json:"supi"`
	ServingNetworkName string `json:"servingNetworkName,omitempty"`
	AuthType           string `json:"authType,omitempty"`
	ResStar            string `json:"resStar,omitempty"`
	AUTS               string `json:"auts,omitempty"`
	EapPayload         string `json:"eapPayload,omitempty"`
}

type AuthenticationResponse struct {
	Success            bool   `json:"success"`
	AuthCtxID          string `json:"authCtxId"`
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
	EapPayload string
}

func (error APIError) Error() string {
	return error.Message
}

func NewClient(baseURL string) *Client {
	client, err := NewClientWithTLSAndBreaker(baseURL, TLSClientConfig{}, defaultBreakerConsecutiveFailures, defaultBreakerTimeout, "")
	if err != nil {
		panic(err)
	}
	return client
}

func NewClientWithBreaker(baseURL string, failures int, timeout time.Duration) *Client {
	client, err := NewClientWithTLSAndBreaker(baseURL, TLSClientConfig{}, failures, timeout, "")
	if err != nil {
		panic(err)
	}
	return client
}

func NewClientWithTLSAndBreaker(baseURL string, tlsClientConfig TLSClientConfig, failures int, timeout time.Duration, bearerToken string) (*Client, error) {
	if failures <= 0 {
		failures = defaultBreakerConsecutiveFailures
	}
	if timeout <= 0 {
		timeout = defaultBreakerTimeout
	}

	httpClient, err := transport.NewHTTPClient(5*time.Second, transport.TLSClientConfig(tlsClientConfig))
	if err != nil {
		return nil, err
	}

	return &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		httpClient:  httpClient,
		breaker:     newCircuitBreaker(failures, timeout),
		bearerToken: strings.TrimSpace(bearerToken),
	}, nil
}

func (client *Client) Initiate(ctx context.Context, request AuthenticationRequest) (AuthenticationResponse, error) {
	return client.doJSON(ctx, http.MethodPost, "/control-plane/v1/auth/initiate", request)
}

func (client *Client) Confirm(ctx context.Context, authCtxID string, request AuthenticationRequest) (AuthenticationResponse, error) {
	return client.doJSON(ctx, http.MethodPost, "/control-plane/v1/auth/"+authCtxID+"/confirm", request)
}

func (client *Client) Context(ctx context.Context, supi string) (AuthenticationResponse, error) {
	return client.doJSON(ctx, http.MethodGet, "/control-plane/v1/auth/"+supi, nil)
}

func (client *Client) doJSON(ctx context.Context, method string, path string, payload any) (AuthenticationResponse, error) {
	if !client.breaker.allow() {
		return AuthenticationResponse{}, APIError{
			StatusCode: http.StatusServiceUnavailable,
			Message:    "control-plane circuit breaker is open",
			ErrorCode:  "CONTROL_PLANE_UNAVAILABLE",
		}
	}

	response, err := client.doJSONWithRetries(ctx, method, path, payload)
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

func (client *Client) doJSONWithRetries(ctx context.Context, method string, path string, payload any) (AuthenticationResponse, error) {
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
		httpRequest, requestErr := http.NewRequestWithContext(ctx, method, client.baseURL+path, bytes.NewReader(body))
		if requestErr != nil {
			return AuthenticationResponse{}, requestErr
		}
		if payload != nil {
			httpRequest.Header.Set("Content-Type", "application/json")
		}
		if client.bearerToken != "" {
			httpRequest.Header.Set("Authorization", "Bearer "+client.bearerToken)
		}
		if sc := tracing.SpanContextFromContext(ctx); sc.IsValid() {
			httpRequest.Header.Set("traceparent", sc.Traceparent())
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
		return AuthenticationResponse{}, APIError{StatusCode: response.StatusCode, Message: message, ErrorCode: result.ErrorCode, EapPayload: result.EapChallenge}
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
