package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// The spool sink writes spans to a FILE instead of a collector, so telemetry
// costs nothing to have on and needs nothing pre-started.
//
// The problem it solves: an OTLP endpoint means a service must already be
// running at the moment a span is produced. Anything that runs before it, or
// while it is down, or on a host that never started it, exports into a closed
// port and is lost — silently, because a dropped span looks exactly like a
// process that had nothing to say. That failure was found in the field: a stack
// ran for eight days with zero data because nothing was configured to send to
// it, and nobody could tell from the outside.
//
// Records are VictoriaLogs jsonline, which is Victoria's own native ingest
// format — NOT OTLP JSON. That choice is what keeps this cheap: a spool file
// imports with
//
//	curl --data-binary @spool.jsonl '<vlogs>/insert/jsonline?_stream_fields=service'
//
// and is then queryable with real LogsQL. Writing OTLP JSON instead would have
// required a translation step or a query engine of our own. Verified: importing
// a spool into a fresh store and filtering on a structured field (exit_code:2)
// returns the exact record, so the query verbs keep working over spooled data.
//
// Selected by the standard OTEL_TRACES_EXPORTER env var ("file"), per this
// package's rule of standard OTEL vars and nothing bespoke.

// spoolExporter appends one jsonline record per span.
//
// It is deliberately a plain appender: no batching beyond the SDK's, no index,
// no rotation-by-write. Concurrency safety comes from O_APPEND plus a mutex —
// a single write() of a line under the pipe-buffer size is atomic on POSIX, so
// several processes can share one spool without coordinating.
type spoolExporter struct {
	mu   sync.Mutex
	path string
	f    *os.File
}

func newSpoolExporter(path string) (*spoolExporter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("spool dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open spool: %w", err)
	}
	return &spoolExporter{path: path, f: f}, nil
}

// ExportSpans writes each span as one jsonline record.
//
// Span attributes are flattened to top-level fields rather than nested under an
// "attributes" object, because LogsQL filters on top-level field names: a
// nested shape would make `exit_code:2` unexpressible and reduce the verbs to
// substring matching on the message.
func (e *spoolExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	if len(spans) == 0 {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	var b strings.Builder
	for _, s := range spans {
		rec := map[string]any{
			// _time/_msg are VictoriaLogs' conventional field names; the import
			// call names them explicitly so this stays readable as plain JSON.
			"_time":       s.EndTime().UTC().Format(time.RFC3339Nano),
			"_msg":        s.Name(),
			"trace_id":    s.SpanContext().TraceID().String(),
			"span_id":     s.SpanContext().SpanID().String(),
			"duration_ms": s.EndTime().Sub(s.StartTime()).Milliseconds(),
			"status":      s.Status().Code.String(),
		}
		if s.Parent().IsValid() {
			rec["parent_span_id"] = s.Parent().SpanID().String()
		}
		if d := s.Status().Description; d != "" {
			rec["status_message"] = d
		}
		for _, kv := range s.Resource().Attributes() {
			rec[sanitizeField(string(kv.Key))] = kv.Value.Emit()
		}
		// Span attributes last so they win over resource attributes on a clash —
		// the span is the more specific statement about that event.
		for _, kv := range s.Attributes() {
			rec[sanitizeField(string(kv.Key))] = kv.Value.Emit()
		}
		line, err := json.Marshal(rec)
		if err != nil {
			continue // one unmarshalable span must not drop the batch
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	_, err := e.f.WriteString(b.String())
	return err
}

func (e *spoolExporter) Shutdown(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.f == nil {
		return nil
	}
	err := e.f.Close()
	e.f = nil
	return err
}

// sanitizeField maps OTel's dotted attribute keys to LogsQL-friendly names.
// "exit.code" would parse as a field access in a query; "exit_code" filters.
func sanitizeField(k string) string {
	return strings.ReplaceAll(k, ".", "_")
}

// SpoolPath is where spans land when the file sink is selected.
// $BASHY_OTEL_SPOOL overrides; otherwise it sits beside the stack's own data so
// one directory holds all observability state.
func SpoolPath() string {
	if p := strings.TrimSpace(os.Getenv("BASHY_OTEL_SPOOL")); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "bashy-otel-spool.jsonl")
	}
	return filepath.Join(home, ".agents", "otel", "spool", "spans.jsonl")
}

// isFileExporter reports whether the file sink is selected.
//
// OTEL_TRACES_EXPORTER is the spec's own knob (its defined values include
// "otlp", "console" and "none"); "file" is the OTLP file-exporter convention.
// "console" is accepted as a synonym rather than writing to stdout directly,
// because a shell that prints spans into its own stdout corrupts every pipeline
// it is part of — the spool file is the honest reading of the intent.
func isFileExporter() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_TRACES_EXPORTER"))) {
	case "file", "console":
		return true
	}
	return false
}
