package service

import "github.com/alexey/ausf/microservices/internal/controlplane"

const (
	contextNotFoundCause            = "CONTEXT_NOT_FOUND"
	controlPlaneUnavailableCause    = "CONTROL_PLANE_UNAVAILABLE"
	controlPlaneInitiateFailedCause = "CONTROL_PLANE_INITIATE_FAILED"
	controlPlaneConfirmFailedCause  = "CONTROL_PLANE_CONFIRMATION_FAILED"
	authStatusChallengeSent         = "CHALLENGE_SENT"
	authStatusAuthenticated         = "AUTHENTICATED"
	authTypeFiveGAka                = "5G_AKA"
	authTypeEapAkaPrime             = "EAP_AKA_PRIME"
)

func contextNotFoundError() APIError {
	return APIError{StatusCode: 404, Message: "authentication context not found", Cause: contextNotFoundCause}
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
		return APIError{StatusCode: apiErr.StatusCode, Message: apiErr.Message, Cause: cause}
	}
	return APIError{StatusCode: 502, Message: operation + ": " + err.Error(), Cause: controlPlaneUnavailableCause}
}
