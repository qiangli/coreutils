package weave

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestWeaveOtelTraceparentPropagation(t *testing.T) {
	if os.Getenv("TEST_AS_AGENT") == "1" {
		tp := os.Getenv("TRACEPARENT")
		if tp == "" {
			fmt.Fprintln(os.Stderr, "no traceparent")
			os.Exit(1)
		}
		// init telemetry
		shutdown := telemetry.Init(context.Background())
		
		ctx := context.Background()
		carrier := propagation.MapCarrier{}
		carrier.Set("traceparent", tp)
		ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
		
		ctx, span := telemetry.Tracer().Start(ctx, "agent.turn")
		span.End()
		shutdown(context.Background())

		os.WriteFile(os.Getenv("TRACE_OUT_FILE"), []byte(tp), 0644)
		exec.Command("git", "commit", "--allow-empty", "-m", "mock commit").Run(); fmt.Println(`{"type":"result","num_turns":3}`)
		os.Exit(0)
	}

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("system git not available")
	}

	// Because telemetry is initialized globally in the process, and we need
	// the exporter to point to our test server, we re-exec the test itself
	// if we haven't done so.
	if os.Getenv("TEST_OTEL_ISOLATION") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=TestWeaveOtelTraceparentPropagation")
		cmd.Env = append(os.Environ(), "TEST_OTEL_ISOLATION=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("isolated test failed: %v\n%s", err, out)
		}
		return
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	repo := t.TempDir()
	gitE2E(t, repo, "init", "-q", "-b", "main")
	gitE2E(t, repo, "config", "user.email", "test@test.local")
	gitE2E(t, repo, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello"), 0644)
	gitE2E(t, repo, "add", "-A")
	gitE2E(t, repo, "commit", "-q", "-m", "init")
	t.Chdir(repo)

	var reqs []tracepb.ExportTraceServiceRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req tracepb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &req); err == nil {
			reqs = append(reqs, req)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	
	shutdown := telemetry.Init(context.Background())
	defer shutdown(context.Background())

	runWeave(t, "add", "test issue")

	traceOutFile := filepath.Join(home, "trace.out")
	t.Setenv("TRACE_OUT_FILE", traceOutFile)
	t.Setenv("TEST_AS_AGENT", "1")

	out, code := runWeave(t, "start", "--", os.Args[0], "-test.run=TestWeaveOtelTraceparentPropagation")
	if code != 0 {
		t.Fatalf("start failed: %d\n%s", code, out)
	}

	shutdown(context.Background())

	tpBytes, err := os.ReadFile(traceOutFile)
	if err != nil {
		t.Fatalf("agent did not write traceparent: %v", err)
	}
	tp := string(tpBytes)
	parts := strings.Split(tp, "-")
	if len(parts) < 3 {
		t.Fatalf("invalid traceparent parts: %s", tp)
	}
	extractedTraceID := parts[1]

	var weaveRunTraceID, agentTurnTraceID string
	var weaveRunFound, agentTurnFound bool
	var outcomeConverged bool

	for _, req := range reqs {
		for _, rs := range req.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for _, span := range ss.Spans {
					if span.Name == "weave.run" {
						weaveRunFound = true
						weaveRunTraceID = hex.EncodeToString(span.TraceId)
						for _, attr := range span.Attributes {
							if attr.Key == "converged" {
								outcomeConverged = attr.Value.GetBoolValue()
							}
						}
					}
					if span.Name == "agent.turn" {
						agentTurnFound = true
						agentTurnTraceID = hex.EncodeToString(span.TraceId)
					}
				}
			}
		}
	}

	if !weaveRunFound {
		t.Errorf("weave.run span not found")
	} else if !outcomeConverged {
		t.Errorf("weave.run span was not flagged as converged despite exiting 0")
	}

	if !agentTurnFound {
		t.Errorf("agent.turn span not found")
	}

	if weaveRunTraceID != agentTurnTraceID {
		t.Errorf("trace IDs do not match! weave.run=%s, agent.turn=%s", weaveRunTraceID, agentTurnTraceID)
	}
	if weaveRunTraceID != extractedTraceID {
		t.Errorf("trace ID from environment does not match! env=%s, weave.run=%s", extractedTraceID, weaveRunTraceID)
	}
}

func TestWeaveOtelNoop(t *testing.T) {
	if os.Getenv("TEST_OTEL_ISOLATION_NOOP") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=TestWeaveOtelNoop")
		cmd.Env = append(os.Environ(), "TEST_OTEL_ISOLATION_NOOP=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("isolated test failed: %v\n%s", err, out)
		}
		return
	}

	// Ensure no endpoint is set
	os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")

	// This should not panic or start any exporters
	shutdown := telemetry.Init(context.Background())
	defer shutdown(context.Background())

	if telemetry.Enabled() {
		t.Fatalf("telemetry should be disabled when endpoint is unset")
	}
}
