package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type ProblemDetails struct {
	Type       string `json:"type"`
	Title      string `json:"title"`
	Status     int    `json:"status"`
	Detail     string `json:"detail,omitempty"`
	Cause      string `json:"cause,omitempty"`
	Instance   string `json:"instance,omitempty"`
	EapPayload string `json:"eapPayload,omitempty"`
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
