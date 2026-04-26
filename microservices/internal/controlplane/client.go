package controlplane

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
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
}

type APIError struct {
	StatusCode int
	Message    string
}

func (error APIError) Error() string {
	return error.Message
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
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
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return AuthenticationResponse{}, err
		}
	}

	httpRequest, err := http.NewRequest(method, client.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return AuthenticationResponse{}, err
	}
	if payload != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}

	response, err := client.httpClient.Do(httpRequest)
	if err != nil {
		return AuthenticationResponse{}, err
	}
	defer response.Body.Close()

	var result AuthenticationResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return AuthenticationResponse{}, err
	}
	if response.StatusCode >= 400 {
		message := result.Message
		if message == "" {
			message = fmt.Sprintf("control-plane request failed with status %d", response.StatusCode)
		}
		return AuthenticationResponse{}, APIError{StatusCode: response.StatusCode, Message: message}
	}
	return result, nil
}
