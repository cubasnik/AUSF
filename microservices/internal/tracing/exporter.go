package tracing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	exportBatchSize    = 128
	exportFlushSeconds = 5
	exportChanCap      = 1024
	exportTimeout      = 3 * time.Second
)

// Exporter batches completed spans and sends them to an OTLP/HTTP collector
// via the JSON wire encoding.  When endpoint is empty every operation is a
// silent no-op.
type Exporter struct {
	endpoint    string
	serviceName string
	ch          chan *Span
	wg          sync.WaitGroup
	once        sync.Once
	stopCh      chan struct{}
}

// NewExporter creates and starts the background flush goroutine.
// Call Shutdown() before process exit to drain the queue.
func NewExporter(endpoint, serviceName string) *Exporter {
	e := &Exporter{
		endpoint:    endpoint,
		serviceName: serviceName,
		ch:          make(chan *Span, exportChanCap),
		stopCh:      make(chan struct{}),
	}
	if endpoint != "" {
		e.wg.Add(1)
		go e.run()
	}
	return e
}

// Shutdown drains the queue and stops the background goroutine.
func (e *Exporter) Shutdown() {
	if e.endpoint == "" {
		return
	}
	e.once.Do(func() { close(e.stopCh) })
	e.wg.Wait()
}

func (e *Exporter) enqueue(s *Span) {
	if e.endpoint == "" {
		return
	}
	select {
	case e.ch <- s:
	default:
		// drop span when buffer is full — prefer shedding telemetry over blocking
	}
}

func (e *Exporter) run() {
	defer e.wg.Done()
	ticker := time.NewTicker(exportFlushSeconds * time.Second)
	defer ticker.Stop()

	var batch []*Span
	for {
		select {
		case s := <-e.ch:
			batch = append(batch, s)
			if len(batch) >= exportBatchSize {
				e.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				e.flush(batch)
				batch = batch[:0]
			}
		case <-e.stopCh:
			// drain remaining spans
			for {
				select {
				case s := <-e.ch:
					batch = append(batch, s)
				default:
					if len(batch) > 0 {
						e.flush(batch)
					}
					return
				}
			}
		}
	}
}

// ────────────────────────────────────────────────────────────────────────────
// OTLP/HTTP JSON payload types (minimal subset needed for spans)
// https://opentelemetry.io/docs/specs/otlp/#otlphttp
// ────────────────────────────────────────────────────────────────────────────

type otlpExportRequest struct {
	ResourceSpans []otlpResourceSpans `json:"resourceSpans"`
}

type otlpResourceSpans struct {
	Resource   otlpResource     `json:"resource"`
	ScopeSpans []otlpScopeSpans `json:"scopeSpans"`
}

type otlpResource struct {
	Attributes []otlpKV `json:"attributes"`
}

type otlpScopeSpans struct {
	Scope otlpScope  `json:"scope"`
	Spans []otlpSpan `json:"spans"`
}

type otlpScope struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type otlpSpan struct {
	TraceID           string     `json:"traceId"`
	SpanID            string     `json:"spanId"`
	ParentSpanID      string     `json:"parentSpanId,omitempty"`
	Name              string     `json:"name"`
	Kind              int        `json:"kind"`
	StartTimeUnixNano string     `json:"startTimeUnixNano"`
	EndTimeUnixNano   string     `json:"endTimeUnixNano"`
	Attributes        []otlpKV   `json:"attributes,omitempty"`
	Status            otlpStatus `json:"status"`
}

type otlpStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

type otlpKV struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

type otlpValue struct {
	StringValue *string  `json:"stringValue,omitempty"`
	IntValue    *int64   `json:"intValue,omitempty"`
	DoubleValue *float64 `json:"doubleValue,omitempty"`
	BoolValue   *bool    `json:"boolValue,omitempty"`
}

// ────────────────────────────────────────────────────────────────────────────

func (e *Exporter) flush(spans []*Span) {
	payload := e.buildPayload(spans)
	body, err := json.Marshal(payload)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, `{"level":"WARN","msg":"otlp marshal error","error":%q}`+"\n", err.Error())
		return
	}

	client := &http.Client{Timeout: exportTimeout}
	resp, err := client.Post(e.endpoint+"/v1/traces", "application/json", bytes.NewReader(body))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, `{"level":"WARN","msg":"otlp export error","error":%q}`+"\n", err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		_, _ = fmt.Fprintf(os.Stderr, `{"level":"WARN","msg":"otlp collector rejected spans","status":%d}`+"\n", resp.StatusCode)
	}
}

func (e *Exporter) buildPayload(spans []*Span) otlpExportRequest {
	svcName := e.serviceName
	otlpSpans := make([]otlpSpan, 0, len(spans))
	for _, s := range spans {
		endNano := s.endNano
		if endNano == 0 {
			endNano = nowNano()
		}
		os := otlpSpan{
			TraceID:           s.sc.TraceID,
			SpanID:            s.sc.SpanID,
			ParentSpanID:      s.sc.ParentID,
			Name:              s.name,
			Kind:              int(s.kind),
			StartTimeUnixNano: fmt.Sprintf("%d", s.startNano),
			EndTimeUnixNano:   fmt.Sprintf("%d", endNano),
			Status:            otlpStatus{Code: s.statusCode, Message: s.statusMsg},
		}
		for _, attr := range s.attrs {
			os.Attributes = append(os.Attributes, toOtlpKV(attr))
		}
		otlpSpans = append(otlpSpans, os)
	}

	svcAttr := stringKV("service.name", svcName)
	return otlpExportRequest{
		ResourceSpans: []otlpResourceSpans{
			{
				Resource: otlpResource{Attributes: []otlpKV{svcAttr}},
				ScopeSpans: []otlpScopeSpans{
					{
						Scope: otlpScope{Name: "ausf", Version: "1.0.0"},
						Spans: otlpSpans,
					},
				},
			},
		},
	}
}

func toOtlpKV(a Attribute) otlpKV {
	switch v := a.Value.(type) {
	case string:
		return stringKV(a.Key, v)
	case int:
		i := int64(v)
		return otlpKV{Key: a.Key, Value: otlpValue{IntValue: &i}}
	case int64:
		return otlpKV{Key: a.Key, Value: otlpValue{IntValue: &v}}
	case float64:
		return otlpKV{Key: a.Key, Value: otlpValue{DoubleValue: &v}}
	case bool:
		return otlpKV{Key: a.Key, Value: otlpValue{BoolValue: &v}}
	default:
		s := fmt.Sprintf("%v", v)
		return stringKV(a.Key, s)
	}
}

func stringKV(key, val string) otlpKV {
	return otlpKV{Key: key, Value: otlpValue{StringValue: &val}}
}
