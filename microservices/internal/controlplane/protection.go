package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/alexey/ausf/microservices/internal/tracing"
)

// SoRProtectionRequest is the body sent to the Java control-plane for
// Nausf_SoRProtection (TS 29.509 §6.2). The control-plane computes
// MAC-SoR using KAUSF of the authenticated context.
type SoRProtectionRequest struct {
	SteeringContainer string `json:"steeringContainer"`
	AckIndication     bool   `json:"ackIndication"`
	// Optional TS 29.509 §6.2.6.2.2 / OAS3 schema fields.
	SorHeader           *string `json:"sorHeader,omitempty"`
	StorageIndicator    *string `json:"storageIndicator,omitempty"`
	Provisioning3gppInd *bool   `json:"provisioning3gppInd,omitempty"`
}

// SoRProtectionResponse is the body returned by the Java control-plane.
// SoRMacIAUSF is 32 hex chars (16-byte MAC), CounterSoR is 4 hex chars.
// StorageIndicator is echoed back when present in the request (TS 29.509 §6.2.6.2.2).
type SoRProtectionResponse struct {
	SoRMacIAUSF      string  `json:"sorMacIausf"`
	CounterSoR       string  `json:"counterSor"`
	StorageIndicator *string `json:"storageIndicator,omitempty"`
}

// UPUProtectionRequest is the body for Nausf_UPUProtection (TS 29.509 §6.3).
type UPUProtectionRequest struct {
	UPUData       string `json:"upuData"`
	AckIndication bool   `json:"ackIndication"`
	// Optional TS 29.509 §6.3.6.2.2 / OAS3 schema fields.
	UpuHeader           *string `json:"upuHeader,omitempty"`
	Provisioning3gppInd *bool   `json:"provisioning3gppInd,omitempty"`
}

// UPUProtectionResponse is the body returned for UPU protection.
type UPUProtectionResponse struct {
	UPUMacIAUSF string `json:"upuMacIausf"`
	CounterUPU  string `json:"counterUpu"`
}

// SoRProtect computes the SoR MAC for an authenticated context.
// Calls POST /control-plane/v1/auth/{authCtxId}/sor-protection.
func (client *Client) SoRProtect(ctx context.Context, authCtxID string, req SoRProtectionRequest) (SoRProtectionResponse, error) {
	var result SoRProtectionResponse
	if err := client.doProtectionRequest(ctx, authCtxID, "sor-protection", req, &result); err != nil {
		return SoRProtectionResponse{}, err
	}
	return result, nil
}

// UPUProtect computes the UPU MAC for an authenticated context.
// Calls POST /control-plane/v1/auth/{authCtxId}/upu-protection.
func (client *Client) UPUProtect(ctx context.Context, authCtxID string, req UPUProtectionRequest) (UPUProtectionResponse, error) {
	var result UPUProtectionResponse
	if err := client.doProtectionRequest(ctx, authCtxID, "upu-protection", req, &result); err != nil {
		return UPUProtectionResponse{}, err
	}
	return result, nil
}

// doProtectionRequest is a single-attempt POST helper used for SoR/UPU protection
// operations. These calls are idempotent per context so no retry is applied.
func (client *Client) doProtectionRequest(ctx context.Context, authCtxID, sub string, payload any, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/control-plane/v1/auth/%s/%s", authCtxID, sub)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if client.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+client.bearerToken)
	}
	if sc := tracing.SpanContextFromContext(ctx); sc.IsValid() {
		req.Header.Set("traceparent", sc.Traceparent())
	}

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return APIError{StatusCode: http.StatusServiceUnavailable, Message: err.Error(), ErrorCode: "CONTROL_PLANE_UNAVAILABLE"}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		// Try to extract a message from the body.
		var errResp struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(respBody, &errResp)
		msg := errResp.Message
		if msg == "" {
			msg = fmt.Sprintf("control-plane %s failed with status %d", sub, resp.StatusCode)
		}
		code := "CONTROL_PLANE_UNAVAILABLE"
		if resp.StatusCode == http.StatusNotFound {
			code = "CONTEXT_NOT_FOUND"
		} else if resp.StatusCode == http.StatusBadRequest {
			code = "INVALID_PROTECTION_REQUEST"
		}
		return APIError{StatusCode: resp.StatusCode, Message: msg, ErrorCode: code}
	}

	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return err
		}
	}
	return nil
}
