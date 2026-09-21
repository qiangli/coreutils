package weavecli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/polyglot"
)

// Sprint 221 B4 (Story-ID af2e56930f08): the sh foreign-worker error detail
// maps into EnvelopeError through exactly one adapter, and an envelope for an
// ordinary Go error is byte-identical to the pre-B4 shape.
//
// Frames below are decoded through polyglot.ErrorDetail's own UnmarshalJSON
// (sh 53a95043), so the legacy plain-string normalization under test is sh's
// real one, not a re-implementation.
//
// Source-derived fixtures:
//
//   - CPython: python/cpython 23116f998f6789d8c2fbe5ed5b8146854c8c2a4f,
//     Lib/test/test_exceptions.py `testChainingAttrs` (an exception whose
//     __cause__ is another exception) and Doc/library/exceptions.rst
//     "Exception context"; PSF-2.0. The Python worker in sh serializes that
//     chain as nested {code,message,help,cause} with the traceback in help.
//   - Rust: serde-rs/json v1.0.151 8d25f3af9f94471a75a18f04da4ca4cdb3cb5f64,
//     tests/test.rs `test_missing_nonoption_field` (message
//     "missing field `x`"), and the Rust std Result docs shipped with rustc
//     1.93.1; MIT OR Apache-2.0 for both. The Rust worker in sh sends
//     {code:"RUST-ECALL", message} with no help and no cause.

// decodeDetail runs a worker error frame's "error" value through sh's decoder.
func decodeDetail(t *testing.T, frame string) *polyglot.ErrorDetail {
	t.Helper()
	var d polyglot.ErrorDetail
	if err := json.Unmarshal([]byte(frame), &d); err != nil {
		t.Fatalf("decode %s: %v", frame, err)
	}
	return &d
}

func TestEnvelopeErrorFromDetail(t *testing.T) {
	const legacyText = "rendered by err.Error()"
	cases := []struct {
		name  string
		frame string // the worker frame's "error" value, "" = no detail (plain Go error)
		code  int
		want  EnvelopeError
	}{
		{
			name:  "structured python exception (CPython testChainingAttrs shape)",
			frame: `{"code":"ValueError","message":"boom","help":"Traceback (most recent call last):\n  raise ValueError('boom')"}`,
			code:  ExitGenericFail,
			want:  EnvelopeError{Code: "ValueError", Message: "boom", Help: "Traceback (most recent call last):\n  raise ValueError('boom')"},
		},
		{
			name:  "structured rust result (serde_json test_missing_nonoption_field)",
			frame: `{"code":"RUST-ECALL","message":"missing field ` + "`x`" + `"}`,
			code:  ExitGenericFail,
			want:  EnvelopeError{Code: "RUST-ECALL", Message: "missing field `x`"},
		},
		{
			name:  "legacy plain string keeps the exit class as code",
			frame: `"plain boom"`,
			code:  ExitPrecondFail,
			want:  EnvelopeError{Code: "precondition_failed", Message: "plain boom"},
		},
		{
			name:  "missing message falls back to the rendered text",
			frame: `{"code":"KeyError"}`,
			code:  ExitGenericFail,
			want:  EnvelopeError{Code: "KeyError", Message: legacyText},
		},
		{
			name:  "missing code falls back to the exit class",
			frame: `{"message":"no code here","help":"h"}`,
			code:  ExitInvalidArg,
			want:  EnvelopeError{Code: "invalid_arg", Message: "no code here", Help: "h"},
		},
		{
			name:  "empty object falls back on both",
			frame: `{}`,
			code:  ExitStateConflict,
			want:  EnvelopeError{Code: "state_conflict", Message: legacyText},
		},
		{
			name:  "null detail is the legacy pair",
			frame: `null`,
			code:  ExitDepUnhealthy,
			want:  EnvelopeError{Code: "dependency_unhealthy", Message: legacyText},
		},
		{
			name:  "nested cause maps field for field",
			frame: `{"code":"RuntimeError","message":"outer","cause":{"code":"ValueError","message":"inner","help":"inner trace"}}`,
			code:  ExitGenericFail,
			want: EnvelopeError{Code: "RuntimeError", Message: "outer",
				Cause: &EnvelopeError{Code: "ValueError", Message: "inner", Help: "inner trace"}},
		},
		{
			name:  "empty cause level is kept, not dropped",
			frame: `{"code":"RuntimeError","message":"outer","cause":{}}`,
			code:  ExitGenericFail,
			want:  EnvelopeError{Code: "RuntimeError", Message: "outer", Cause: &EnvelopeError{}},
		},
		{
			name:  "no detail: plain go error / transport failure is the legacy pair",
			frame: "",
			code:  ExitInputRequired,
			want:  EnvelopeError{Code: "input_required", Message: legacyText},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var detail *polyglot.ErrorDetail
			if tc.frame != "" {
				detail = decodeDetail(t, tc.frame)
			}
			got := EnvelopeErrorFromDetail(tc.code, detail, legacyText)
			if got == nil {
				t.Fatal("nil envelope error")
			}
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tc.want)
			if !bytes.Equal(gotJSON, wantJSON) {
				t.Fatalf("got  %s\nwant %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestEnvelopeErrorFromDetail_CauseIsBounded(t *testing.T) {
	// A chain deeper than MaxCauseDepth (the sh Python worker itself stops at
	// 16, so 40 is a hostile or broken peer) is cut at the envelope's own
	// bound; the levels kept are the outermost ones.
	const deep = 40
	var frame strings.Builder
	for i := 0; i < deep; i++ {
		fmt.Fprintf(&frame, `{"code":"level%d","message":"m%d","cause":`, i, i)
	}
	frame.WriteString(`{"code":"root","message":"r"}`)
	frame.WriteString(strings.Repeat("}", deep))

	got := EnvelopeErrorFromDetail(ExitGenericFail, decodeDetail(t, frame.String()), "text")
	if got.Code != "level0" {
		t.Fatalf("top code = %q, want level0", got.Code)
	}
	depth := 0
	last := got
	for c := got.Cause; c != nil; c = c.Cause {
		depth++
		last = c
	}
	if depth != MaxCauseDepth {
		t.Fatalf("nested cause depth = %d, want exactly MaxCauseDepth (%d)", depth, MaxCauseDepth)
	}
	if want := fmt.Sprintf("level%d", MaxCauseDepth); last.Code != want {
		t.Fatalf("deepest kept level = %q, want %q (outermost levels win)", last.Code, want)
	}

	// Exactly at the bound nothing is lost, including the root.
	var exact strings.Builder
	for i := 0; i < MaxCauseDepth; i++ {
		fmt.Fprintf(&exact, `{"code":"level%d","cause":`, i)
	}
	exact.WriteString(`{"code":"root"}`)
	exact.WriteString(strings.Repeat("}", MaxCauseDepth))
	got = EnvelopeErrorFromDetail(ExitGenericFail, decodeDetail(t, exact.String()), "text")
	last = got
	for c := got.Cause; c != nil; c = c.Cause {
		last = c
	}
	if last.Code != "root" {
		t.Fatalf("chain of exactly MaxCauseDepth causes lost its root: deepest = %q", last.Code)
	}
}

func TestNewEnvelopeError_NilAndNonForeign(t *testing.T) {
	if got := NewEnvelopeError(ExitPrecondFail, nil); got == nil || got.Code != "precondition_failed" || got.Message != "" || got.Help != "" || got.Cause != nil {
		t.Fatalf("nil error => %#v, want bare exit-class code", got)
	}

	plain := errors.New("queue empty")
	got := NewEnvelopeError(ExitPrecondFail, plain)
	want := &EnvelopeError{Code: "precondition_failed", Message: "queue empty"}
	if *got != *want {
		t.Fatalf("plain error => %#v, want %#v", got, want)
	}

	// A wrapped ordinary error is still not foreign detail: no code is
	// invented, the rendered text is the message.
	wrapped := fmt.Errorf("weave start: %w", plain)
	if got := NewEnvelopeError(ExitGenericFail, wrapped); got.Code != "generic_failure" || got.Message != "weave start: queue empty" || got.Cause != nil {
		t.Fatalf("wrapped error => %#v", got)
	}

	// sh reports a transport failure (a frame that is not JSON) as an
	// ordinary error with no detail; it must not become a structured error.
	transport := errors.New("invalid worker response: {not-json")
	if got := NewEnvelopeError(ExitDepUnhealthy, transport); got.Code != "dependency_unhealthy" || got.Message != transport.Error() || got.Help != "" || got.Cause != nil {
		t.Fatalf("transport failure => %#v", got)
	}
	if _, ok := polyglot.ForeignErrorDetail(transport); ok {
		t.Fatal("test premise broken: a plain error reported foreign detail")
	}
}

// TestEnvelopeError_JSONCompatibility pins the wire contract for legacy
// {code,message} readers and writers in both directions.
func TestEnvelopeError_JSONCompatibility(t *testing.T) {
	// 1. An ordinary error still serializes to exactly the two legacy keys.
	var buf bytes.Buffer
	EmitError(&buf, OutputJSON, "weave start", ExitPrecondFail, errors.New("queue empty"))
	var raw struct {
		Error map[string]json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw.Error) != 2 || raw.Error["code"] == nil || raw.Error["message"] == nil {
		t.Fatalf("legacy envelope error keys = %v, want exactly code+message", raw.Error)
	}
	if strings.Contains(buf.String(), `"help"`) || strings.Contains(buf.String(), `"cause"`) {
		t.Fatalf("legacy envelope grew keys: %s", buf.String())
	}

	// 2. A structured envelope decodes cleanly into a legacy reader's struct.
	structured := EnvelopeErrorFromDetail(ExitGenericFail, decodeDetail(t,
		`{"code":"RuntimeError","message":"outer","help":"trace","cause":{"code":"ValueError","message":"inner"}}`), "text")
	data, err := json.Marshal(Envelope{SchemaVersion: SchemaVersion, Command: "island", Status: "error", Error: structured})
	if err != nil {
		t.Fatal(err)
	}
	var legacy struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatalf("legacy reader rejected structured envelope: %v", err)
	}
	if legacy.Error == nil || legacy.Error.Code != "RuntimeError" || legacy.Error.Message != "outer" {
		t.Fatalf("legacy reader saw %#v", legacy.Error)
	}
	for _, key := range []string{`"help":"trace"`, `"cause":{`, `"code":"ValueError"`} {
		if !strings.Contains(string(data), key) {
			t.Fatalf("structured envelope lost %s: %s", key, data)
		}
	}

	// 3. A legacy envelope decodes into the new struct with the new fields empty.
	var got Envelope
	if err := json.Unmarshal([]byte(`{"schema_version":"loom-v2","command":"x","status":"error","error":{"code":"invalid_arg","message":"m"}}`), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error == nil || got.Error.Code != "invalid_arg" || got.Error.Message != "m" || got.Error.Help != "" || got.Error.Cause != nil {
		t.Fatalf("legacy envelope decoded as %#v", got.Error)
	}

	// 4. Round trip of a nested structured error is lossless.
	var back EnvelopeError
	if err := json.Unmarshal(mustJSON(t, structured), &back); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mustJSON(t, &back), mustJSON(t, structured)) {
		t.Fatalf("round trip changed the error: %s vs %s", mustJSON(t, &back), mustJSON(t, structured))
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestEmitError_PlainModeUnchanged: the non-JSON path prints the rendered
// error text exactly as before, regardless of any structured detail.
func TestEmitError_PlainModeUnchanged(t *testing.T) {
	var buf bytes.Buffer
	code := EmitError(&buf, OutputPlain, "island", ExitGenericFail, errors.New("ValueError: boom: ValueError: inner"))
	if code != ExitGenericFail || buf.String() != "island: ValueError: boom: ValueError: inner\n" {
		t.Fatalf("code=%d out=%q", code, buf.String())
	}
}
