package otelquery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGuessedQueryAndSummary(t *testing.T) {
	var gotPath, gotQuery, gotLimit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("query")
		gotLimit = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
{"trace_id":"t1","span_id":"s1","service.name":"ycode","span.name":"turn","event.name":"value","value.name":"context_tokens","value.amount":123,"value.source":"estimate"},
{"trace_id":"t2","span_id":"s2","service.name":"ycode","span.name":"turn","event.name":"value","value.name":"price","value.amount":1.2,"value.source":"GUESS-default-rate"},
{"trace_id":"t3","span_id":"s3","service.name":"bashy","span.name":"turn","event.name":"value","value.name":"context_tokens","value.amount":99,"value.source":"estimate"}
]`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL)
	env, err := c.Guessed(t.Context(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/traces/select/logsql/query" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotQuery, "value.source") || !strings.Contains(gotQuery, "GUESS-default-rate") {
		t.Fatalf("query = %q", gotQuery)
	}
	if gotLimit != "201" {
		t.Fatalf("limit = %q", gotLimit)
	}
	if env.TotalMatches != 3 || env.Items[0].Count != 2 || !strings.Contains(env.Items[0].Key, "estimate") {
		t.Fatalf("bad summary: %+v", env)
	}
}

func TestBoundsQueryAndSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Query().Get("query"), "bound.was_hit") {
			t.Fatalf("query = %q", r.URL.Query().Get("query"))
		}
		w.Write([]byte(`[
{"trace_id":"t1","bound.kind":"rate_limit","bound.limit":90,"bound.actual":130},
{"trace_id":"t2","bound.kind":"rate_limit","bound.limit":90,"bound.actual":100},
{"trace_id":"t3","bound.kind":"bytes","bound.limit":1000,"bound.actual":1200}
]`))
	}))
	defer srv.Close()
	env, err := NewClient(srv.URL).Bounds(t.Context(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if env.TotalMatches != 3 || len(env.Items) != 3 {
		t.Fatalf("bad bounds: %+v", env)
	}
}

func TestFailedGroupsIdenticalFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
{"trace_id":"t1","cmd.name":"go test","cmd.exit_code":1,"cwd":"/repo","agent.principal":"codex","duration_ms":100},
{"trace_id":"t2","cmd.name":"go test","cmd.exit_code":1,"cwd":"/repo","agent.principal":"codex","duration_ms":90},
{"trace_id":"t3","cmd.name":"go vet","cmd.exit_code":0,"cwd":"/repo"}
]`))
	}))
	defer srv.Close()
	env, err := NewClient(srv.URL).Failed(t.Context(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if env.TotalMatches != 3 || len(env.Items) != 1 || env.Items[0].Count != 2 {
		t.Fatalf("bad failed summary: %+v", env)
	}
}

func TestCostSuspectQuery(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("query"))
		w.Write([]byte(`{"status":"success","data":{"result":[{"metric":{"__name__":"ycode.llm.cost.dollars","model":"unknown","pricing_known":"false"},"value":[1,"0.42"]}]}}`))
	}))
	defer srv.Close()
	env, err := NewClient(srv.URL).Cost(t.Context(), Options{Suspect: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queries[0], `pricing_known="false"`) {
		t.Fatalf("query = %q", queries[0])
	}
	if env.Items[0].CostUSD != 0.42 {
		t.Fatalf("cost = %+v", env.Items[0])
	}
}

func TestWhySlowSummarizesTrace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/traces/select/jaeger/api/traces/abc" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"data":[{"spans":[
{"spanID":"s1","operationName":"turn","duration":40000000},
{"spanID":"s2","operationName":"rate wait","duration":50000000,"tags":[{"key":"bound.kind","value":"rate_limit"}]}
]}]}`))
	}))
	defer srv.Close()
	env, err := NewClient(srv.URL).WhySlow(t.Context(), "abc", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if env.Trace.SpanCount != 2 || env.Trace.BoundWaitMS != 50_000 {
		b, _ := json.Marshal(env)
		t.Fatalf("bad trace summary: %s", b)
	}
}
