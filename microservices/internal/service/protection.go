package service

import (
	"context"
	"net/http"

	"github.com/alexey/ausf/microservices/internal/controlplane"
)

// SoRInfo is the request body from AMF/UDM to AUSF for SoR protection
// per TS 29.509 §6.2.6.2.1.
type SoRInfo struct {
	SteeringContainer string `json:"steeringContainer"`
	AckIndication     bool   `json:"ackIndication"`
	// Optional fields per TS 29.509 OAS3 schema (Table 6.2.6.2.2-1).
	SorHeader           *string `json:"sorHeader,omitempty"`
	StorageIndicator    *string `json:"storageIndicator,omitempty"`
	Provisioning3gppInd *bool   `json:"provisioning3gppInd,omitempty"`
}

// SoRSecurityInfo is the response from AUSF per TS 29.509 §6.2.6.2.2.
type SoRSecurityInfo struct {
	SoRMacIAUSF      string  `json:"sorMacIausf"`
	CounterSoR       string  `json:"counterSor"`
	StorageIndicator *string `json:"storageIndicator,omitempty"`
}

// UPUInfo is the request body for UPU protection per TS 29.509 §6.3.6.2.1.
type UPUInfo struct {
	UPUData       string `json:"upuData"`
	AckIndication bool   `json:"ackIndication"`
	// Optional fields per TS 29.509 OAS3 schema (Table 6.3.6.2.2-1).
	UpuHeader           *string `json:"upuHeader,omitempty"`
	Provisioning3gppInd *bool   `json:"provisioning3gppInd,omitempty"`
}

// UPUSecurityInfo is the response from AUSF per TS 29.509 §6.3.6.2.2.
type UPUSecurityInfo struct {
	UPUMacIAUSF string `json:"upuMacIausf"`
	CounterUPU  string `json:"counterUpu"`
}

// SoRProtect computes the SoR MAC for an authenticated context and returns
// the SoR security information to the caller (UDM/AMF).
//
// The caller must supply the authCtxId of a successfully authenticated context.
// The KAUSF for that context is held by the Java control-plane.
func (service *AuthService) SoRProtect(ctx context.Context, authCtxID string, info SoRInfo) (SoRSecurityInfo, error) {
	service.mu.RLock()
	authCtx, err := service.lookupContextLocked(authCtxID)
	service.mu.RUnlock()
	if err != nil {
		return SoRSecurityInfo{}, err
	}

	if authCtx.Status != authStatusAuthenticated {
		return SoRSecurityInfo{}, APIError{
			StatusCode: http.StatusConflict,
			Message:    "SoR protection requires an AUTHENTICATED context",
			Cause:      "CONTEXT_NOT_AUTHENTICATED",
		}
	}

	resp, err := service.controlPlaneClient.SoRProtect(ctx, authCtxID, controlplane.SoRProtectionRequest{
		SteeringContainer:   info.SteeringContainer,
		AckIndication:       info.AckIndication,
		SorHeader:           info.SorHeader,
		StorageIndicator:    info.StorageIndicator,
		Provisioning3gppInd: info.Provisioning3gppInd,
	})
	if err != nil {
		if apiErr, ok := err.(controlplane.APIError); ok {
			return SoRSecurityInfo{}, APIError{
				StatusCode: apiErr.StatusCode,
				Message:    apiErr.Message,
				Cause:      apiErr.ErrorCode,
			}
		}
		return SoRSecurityInfo{}, APIError{
			StatusCode: http.StatusServiceUnavailable,
			Message:    err.Error(),
			Cause:      "CONTROL_PLANE_UNAVAILABLE",
		}
	}

	return SoRSecurityInfo{
		SoRMacIAUSF:      resp.SoRMacIAUSF,
		CounterSoR:       resp.CounterSoR,
		StorageIndicator: resp.StorageIndicator,
	}, nil
}

// UPUProtect computes the UPU MAC for an authenticated context.
func (service *AuthService) UPUProtect(ctx context.Context, authCtxID string, info UPUInfo) (UPUSecurityInfo, error) {
	service.mu.RLock()
	authCtx, err := service.lookupContextLocked(authCtxID)
	service.mu.RUnlock()
	if err != nil {
		return UPUSecurityInfo{}, err
	}

	if authCtx.Status != authStatusAuthenticated {
		return UPUSecurityInfo{}, APIError{
			StatusCode: http.StatusConflict,
			Message:    "UPU protection requires an AUTHENTICATED context",
			Cause:      "CONTEXT_NOT_AUTHENTICATED",
		}
	}

	resp, err := service.controlPlaneClient.UPUProtect(ctx, authCtxID, controlplane.UPUProtectionRequest{
		UPUData:             info.UPUData,
		AckIndication:       info.AckIndication,
		UpuHeader:           info.UpuHeader,
		Provisioning3gppInd: info.Provisioning3gppInd,
	})
	if err != nil {
		if apiErr, ok := err.(controlplane.APIError); ok {
			return UPUSecurityInfo{}, APIError{
				StatusCode: apiErr.StatusCode,
				Message:    apiErr.Message,
				Cause:      apiErr.ErrorCode,
			}
		}
		return UPUSecurityInfo{}, APIError{
			StatusCode: http.StatusServiceUnavailable,
			Message:    err.Error(),
			Cause:      "CONTROL_PLANE_UNAVAILABLE",
		}
	}

	return UPUSecurityInfo{
		UPUMacIAUSF: resp.UPUMacIAUSF,
		CounterUPU:  resp.CounterUPU,
	}, nil
}
