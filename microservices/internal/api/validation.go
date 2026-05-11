package api

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	"go.yaml.in/yaml/v2"
)

//go:embed schema/nausf-auth-v1.yaml
var embeddedSchema []byte

// schemaProperty describes one field constraint in a validation rule.
type schemaProperty struct {
	Type string   `yaml:"type"`
	Enum []string `yaml:"enum"`
}

// schemaRoute is one route rule parsed from the embedded schema file.
type schemaRoute struct {
	Method     string                    `yaml:"method"`
	Path       string                    `yaml:"path"`
	PathSuffix string                    `yaml:"path_suffix"`
	Required   []string                  `yaml:"required"`
	Properties map[string]schemaProperty `yaml:"properties"`
}

type schemaFile struct {
	Routes []schemaRoute `yaml:"routes"`
}

var (
	schemaOnce   sync.Once
	schemaRoutes []schemaRoute
)

func loadedSchema() []schemaRoute {
	schemaOnce.Do(func() {
		var sf schemaFile
		if err := yaml.Unmarshal(embeddedSchema, &sf); err != nil {
			panic("ausf: failed to parse embedded request schema: " + err.Error())
		}
		schemaRoutes = sf.Routes
	})
	return schemaRoutes
}

// withRequestValidation is an HTTP middleware that validates POST/PUT request
// bodies for Nausf_UEAuthentication routes against the embedded OAS3 schema
// (schema/nausf-auth-v1.yaml).  Non-conforming requests receive HTTP 400
// ProblemDetails with an invalidParams array per TS 29.500 §6.6.4.
func withRequestValidation(next http.Handler) http.Handler {
	routes := loadedSchema()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPut {
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/nausf-auth/") {
			next.ServeHTTP(w, r)
			return
		}

		rule := matchSchemaRule(routes, r.Method, r.URL.Path)
		if rule == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Buffer the body so the downstream handler can still read it.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "Invalid request", "failed to read request body", "MALFORMED_REQUEST", r.URL.Path)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		var doc map[string]interface{}
		if err := json.Unmarshal(body, &doc); err != nil {
			writeProblem(w, http.StatusBadRequest, "Invalid request", "request body must be valid JSON", "MALFORMED_REQUEST", r.URL.Path)
			return
		}

		var violations []InvalidParam
		violations = append(violations, checkRequiredFields(doc, rule.Required)...)
		violations = append(violations, checkPropertyTypes(doc, rule.Properties)...)

		if len(violations) > 0 {
			writeProblemWithInvalidParams(w, http.StatusBadRequest,
				"Invalid request",
				"request body does not conform to the API schema",
				"SCHEMA_VALIDATION_FAILED",
				r.URL.Path,
				violations,
			)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func matchSchemaRule(routes []schemaRoute, method, path string) *schemaRoute {
	for i := range routes {
		r := &routes[i]
		if r.Method != method {
			continue
		}
		if r.Path != "" && r.Path == path {
			return r
		}
		if r.PathSuffix != "" && strings.HasSuffix(path, r.PathSuffix) {
			return r
		}
	}
	return nil
}

func checkRequiredFields(doc map[string]interface{}, required []string) []InvalidParam {
	var out []InvalidParam
	for _, field := range required {
		val, ok := doc[field]
		if !ok || val == nil {
			out = append(out, InvalidParam{Param: field, Reason: "required field is missing"})
			continue
		}
		if s, isStr := val.(string); isStr && strings.TrimSpace(s) == "" {
			out = append(out, InvalidParam{Param: field, Reason: "required string field must not be blank"})
		}
	}
	return out
}

func checkPropertyTypes(doc map[string]interface{}, props map[string]schemaProperty) []InvalidParam {
	var out []InvalidParam
	for field, constraint := range props {
		val, ok := doc[field]
		if !ok || val == nil {
			continue // optional field; absence is fine
		}
		switch constraint.Type {
		case "string":
			s, isStr := val.(string)
			if !isStr {
				out = append(out, InvalidParam{Param: field, Reason: "must be a string"})
				continue
			}
			if len(constraint.Enum) > 0 && !schemaEnumContains(constraint.Enum, s) {
				out = append(out, InvalidParam{Param: field, Reason: "must be one of: " + strings.Join(constraint.Enum, ", ")})
			}
		case "boolean":
			if _, isBool := val.(bool); !isBool {
				out = append(out, InvalidParam{Param: field, Reason: "must be a boolean"})
			}
		case "integer", "number":
			switch val.(type) {
			case float64, int, int64:
				// valid JSON number
			default:
				out = append(out, InvalidParam{Param: field, Reason: "must be a number"})
			}
		}
	}
	return out
}

func schemaEnumContains(enum []string, value string) bool {
	for _, e := range enum {
		if e == value {
			return true
		}
	}
	return false
}
