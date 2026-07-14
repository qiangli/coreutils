package otelquery

import "time"

const SchemaVersion = "bashy-otelquery-v1"

const defaultBaseURL = "http://127.0.0.1:8428"

type Options struct {
	BaseURL string
	JSON    bool
	Since   time.Duration
	Suspect bool
}

type Envelope struct {
	SchemaVersion string         `json:"schema_version"`
	Verb          string         `json:"verb"`
	QueryURL      string         `json:"query_url"`
	Backend       string         `json:"backend"`
	Query         string         `json:"query,omitempty"`
	TotalMatches  int            `json:"total_matches"`
	Shown         int            `json:"shown"`
	Truncated     bool           `json:"truncated"`
	Summary       string         `json:"summary"`
	Items         []SummaryItem  `json:"items,omitempty"`
	Trace         *TraceSummary  `json:"trace,omitempty"`
	Metrics       []MetricSeries `json:"metrics,omitempty"`
}

type SummaryItem struct {
	Key          string  `json:"key"`
	Count        int     `json:"count"`
	TraceID      string  `json:"trace_id,omitempty"`
	SpanID       string  `json:"span_id,omitempty"`
	Service      string  `json:"service,omitempty"`
	Span         string  `json:"span,omitempty"`
	Event        string  `json:"event,omitempty"`
	Source       string  `json:"source,omitempty"`
	ValueName    string  `json:"value_name,omitempty"`
	Amount       string  `json:"amount,omitempty"`
	Kind         string  `json:"kind,omitempty"`
	Limit        string  `json:"limit,omitempty"`
	Actual       string  `json:"actual,omitempty"`
	Command      string  `json:"command,omitempty"`
	ExitCode     string  `json:"exit_code,omitempty"`
	CWD          string  `json:"cwd,omitempty"`
	Principal    string  `json:"agent_principal,omitempty"`
	DurationMS   float64 `json:"duration_ms,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
	Tokens       float64 `json:"tokens,omitempty"`
	Model        string  `json:"model,omitempty"`
	PricingKnown string  `json:"pricing_known,omitempty"`
}

type TraceSummary struct {
	TraceID     string        `json:"trace_id"`
	DurationMS  float64       `json:"duration_ms"`
	BoundWaitMS float64       `json:"bound_wait_ms"`
	WorkMS      float64       `json:"work_ms"`
	SpanCount   int           `json:"span_count"`
	TopSpans    []SummaryItem `json:"top_spans,omitempty"`
	BoundSpans  []SummaryItem `json:"bound_spans,omitempty"`
}

type MetricSeries struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
	Value  float64           `json:"value"`
}
