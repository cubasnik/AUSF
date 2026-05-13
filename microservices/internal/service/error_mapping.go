package service

import (
	"strings"

	"github.com/alexey/ausf/microservices/internal/controlplane"
)

const (
	contextNotFoundCause            = "CONTEXT_NOT_FOUND"
	contextStoreFailureCause        = "AUTH_CONTEXT_STORE_FAILURE"
	controlPlaneUnavailableCause    = "CONTROL_PLANE_UNAVAILABLE"
	controlPlaneInitiateFailedCause = "CONTROL_PLANE_INITIATE_FAILED"
	controlPlaneConfirmFailedCause  = "CONTROL_PLANE_CONFIRMATION_FAILED"
	authenticationRejectedCause     = "AUTHENTICATION_REJECTED"
	unsupportedAuthTypeCause        = "UNSUPPORTED_AUTH_TYPE"
	authStatusChallengeSent         = "CHALLENGE_SENT"
	authStatusAuthenticated         = "AUTHENTICATED"
	authResultOngoing               = "ONGOING"
	authResultSyncFailure           = "SYNC_FAILURE"
	authTypeFiveGAka                = "5G_AKA"
	authTypeEapAkaPrime             = "EAP_AKA_PRIME"
)

func validateRequestedAuthType(authType string) error {
	authType = strings.TrimSpace(authType)
	if authType == "" || authType == authTypeFiveGAka || authType == authTypeEapAkaPrime {
		return nil
	}

	return APIError{
		StatusCode: 400,
		Message:    "authType must be 5G_AKA or EAP_AKA_PRIME when provided",
		Cause:      unsupportedAuthTypeCause,
	}
}

func contextNotFoundError() APIError {
	return APIError{StatusCode: 404, Message: "authentication context not found", Cause: contextNotFoundCause}
}

func contextStoreError(message string, err error) APIError {
	return APIError{StatusCode: 500, Message: message + ": " + err.Error(), Cause: contextStoreFailureCause}
}

func contextNotPendingError() APIError {
	return APIError{StatusCode: 401, Message: "authentication context is no longer pending", Cause: authenticationRejectedCause}
}

func invalidAutsForAuthTypeError() APIError {
	return APIError{StatusCode: 401, Message: "AUTS re-synchronization is only valid for 5G_AKA", Cause: authenticationRejectedCause}
}

func invalidFiveGAkaConfirmationError() APIError {
	return APIError{StatusCode: 401, Message: "5G_AKA confirmation must provide exactly one of RES* or AUTS", Cause: authenticationRejectedCause}
}

func mapInitiateError(err error) APIError {
	return mapControlPlaneError(err, controlPlaneInitiateFailedCause, "control-plane initiate failed")
}

func mapConfirmError(err error) APIError {
	return mapControlPlaneError(err, controlPlaneConfirmFailedCause, "control-plane confirmation failed")
}

func mapControlPlaneError(err error, fallbackCause string, operation string) APIError {
	if apiErr, ok := err.(controlplane.APIError); ok {
		cause := fallbackCause
		if apiErr.ErrorCode != "" {
			cause = apiErr.ErrorCode
		}
		return APIError{StatusCode: apiErr.StatusCode, Message: apiErr.Message, Cause: cause, EapPayload: apiErr.EapPayload}
	}
	return APIError{StatusCode: 502, Message: operation + ": " + err.Error(), Cause: controlPlaneUnavailableCause}
}
