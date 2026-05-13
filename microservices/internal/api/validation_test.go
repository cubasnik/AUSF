package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexey/ausf/microservices/internal/service"
)

func TestValidationMiddlewareMissingRequiredField(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()

	// supiOrSuci present but servingNetworkName missing
	body, _ := json.Marshal(map[string]string{
		"supiOrSuci": "imsi-250010000000001",
	})
	req := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var pd ProblemDetails
	if err := json.NewDecoder(w.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.Cause != "SCHEMA_VALIDATION_FAILED" {
		t.Errorf("expected SCHEMA_VALIDATION_FAILED cause, got %q", pd.Cause)
	}
	if len(pd.InvalidParams) == 0 {
		t.Fatal("expected invalidParams to be non-empty")
	}
	found := false
	for _, ip := range pd.InvalidParams {
		if ip.Param == "servingNetworkName" {
			found = true
		}
	}
	if !found {
		t.Error("expected invalidParams to contain 'servingNetworkName'")
	}
}

func TestValidationMiddlewareInvalidEnumValue(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()

	body, _ := json.Marshal(map[string]string{
		"supiOrSuci":         "imsi-250010000000001",
		"servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
		"authType":           "UNKNOWN_METHOD",
	})
	req := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var pd ProblemDetails
	if err := json.NewDecoder(w.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.Cause != "SCHEMA_VALIDATION_FAILED" {
		t.Errorf("expected SCHEMA_VALIDATION_FAILED cause, got %q", pd.Cause)
	}
	found := false
	for _, ip := range pd.InvalidParams {
		if ip.Param == "authType" && strings.Contains(ip.Reason, "one of") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected invalidParams to flag authType enum violation, got %+v", pd.InvalidParams)
	}
}

func TestValidationMiddlewareWrongFieldType(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()

	// ackIndication on sor-protection must be boolean, not string
	body, _ := json.Marshal(map[string]interface{}{
		"steeringContainer": "aabbcc",
		"ackIndication":     "yes", // wrong type
	})
	req := httptest.NewRequest(http.MethodPut, "/nausf-auth/v1/ue-authentications/ctx-1/sor-protection", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var pd ProblemDetails
	if err := json.NewDecoder(w.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.Cause != "SCHEMA_VALIDATION_FAILED" {
		t.Errorf("expected SCHEMA_VALIDATION_FAILED cause, got %q", pd.Cause)
	}
	found := false
	for _, ip := range pd.InvalidParams {
		if ip.Param == "ackIndication" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected invalidParams to flag ackIndication type error, got %+v", pd.InvalidParams)
	}
}

func TestValidationMiddlewarePassesValidRequest(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()

	body, _ := json.Marshal(map[string]string{
		"supiOrSuci":         "imsi-250010000000001",
		"servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
		"authType":           "5G_AKA",
	})
	req := httptest.NewRequest(http.MethodPost, "/nausf-auth/v1/ue-authentications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Without a real service the handler returns 500/error, but NOT 400 validation failure
	if w.Code == http.StatusBadRequest {
		var pd ProblemDetails
		_ = json.NewDecoder(w.Body).Decode(&pd)
		t.Fatalf("valid request rejected by middleware: %+v", pd)
	}
}

func TestValidationMiddlewarePassthroughNonNausfPath(t *testing.T) {
	handler := NewHandler(service.NewAuthService(stubControlPlaneClient{}, nil)).Routes()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 from healthz, got %d", w.Code)
	}
}
