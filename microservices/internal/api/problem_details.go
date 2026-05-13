package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// InvalidParam describes a single request parameter that failed schema validation
// per TS 29.500 §6.6.4.
type InvalidParam struct {
	Param  string `json:"param"`
	Reason string `json:"reason,omitempty"`
}

type ProblemDetails struct {
	Type          string         `json:"type"`
	Title         string         `json:"title"`
	Status        int            `json:"status"`
	Detail        string         `json:"detail,omitempty"`
	Cause         string         `json:"cause,omitempty"`
	Instance      string         `json:"instance,omitempty"`
	EapPayload    string         `json:"eapPayload,omitempty"`
	InvalidParams []InvalidParam `json:"invalidParams,omitempty"`
}

func writeProblem(writer http.ResponseWriter, status int, title string, detail string, cause string, instance string) {
	writeProblemWithEapPayload(writer, status, title, detail, cause, instance, "")
}

func writeProblemWithEapPayload(writer http.ResponseWriter, status int, title string, detail string, cause string, instance string, eapPayload string) {
	writer.Header().Set("Content-Type", "application/problem+json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(ProblemDetails{
		Type:       fmt.Sprintf("https://example.com/problem/%d", status),
		Title:      title,
		Status:     status,
		Detail:     detail,
		Cause:      cause,
		Instance:   instance,
		EapPayload: eapPayload,
	})
}

func writeProblemWithInvalidParams(writer http.ResponseWriter, status int, title string, detail string, cause string, instance string, params []InvalidParam) {
	writer.Header().Set("Content-Type", "application/problem+json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(ProblemDetails{
		Type:          fmt.Sprintf("https://example.com/problem/%d", status),
		Title:         title,
		Status:        status,
		Detail:        detail,
		Cause:         cause,
		Instance:      instance,
		InvalidParams: params,
	})
}
