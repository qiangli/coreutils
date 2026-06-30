package weavecli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestIsAgent(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"no", false},
		{"1", true},
		{"true", true},
		{"yes", true},
		{"anything", true},
	}
	for _, tc := range cases {
		t.Setenv("BASHY_AGENTIC", tc.val)
		if got := IsAgent(); got != tc.want {
			t.Errorf("IsAgent with BASHY_AGENTIC=%q got %v want %v", tc.val, got, tc.want)
		}
	}
}

func TestIsAgent_Unset(t *testing.T) {
	// BASHY_AGENTIC is the single agent-mode signal; unset ⇒ not agent.
	t.Setenv("BASHY_AGENTIC", "")
	if IsAgent() {
		t.Error("unset BASHY_AGENTIC should leave agent mode off")
	}
	t.Setenv("BASHY_AGENTIC", "1")
	if !IsAgent() {
		t.Error("BASHY_AGENTIC=1 should enable agent mode")
	}
}

func TestResolveOutputMode_Precedence(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	cases := []struct {
		jsonF, plainF, quietF bool
		want                  OutputMode
	}{
		{true, false, false, OutputJSON},
		{false, true, false, OutputPlain},
		{false, false, true, OutputQuiet},
		{false, false, false, OutputAuto},
		{true, true, true, OutputJSON}, // json wins
	}
	for _, tc := range cases {
		got := ResolveOutputMode(tc.jsonF, tc.plainF, tc.quietF)
		if got != tc.want {
			t.Errorf("ResolveOutputMode(json=%v plain=%v quiet=%v)=%v want %v",
				tc.jsonF, tc.plainF, tc.quietF, got, tc.want)
		}
	}
}

// TestResolveOutputMode_EnvFlagPrecedence is the full matrix of how the
// BASHY_AGENTIC env interacts with the explicit per-command flags, including the
// --json=false / --plain escape hatch. Precedence:
//
//	explicit --json(=true/false) > --plain > --quiet > BASHY_AGENTIC > auto
func TestResolveOutputMode_EnvFlagPrecedence(t *testing.T) {
	cases := []struct {
		name                           string
		env                            string // BASHY_AGENTIC value ("" = unset)
		jsonSet, jsonVal, plain, quiet bool
		want                           OutputMode
	}{
		// --- env off (or unset): flags / auto decide ---
		{"unset, no flags -> auto", "", false, false, false, false, OutputAuto},
		{"unset, --json -> json", "", true, true, false, false, OutputJSON},
		{"unset, --plain -> plain", "", false, false, true, false, OutputPlain},
		{"unset, --quiet -> quiet", "", false, false, false, true, OutputQuiet},
		{"off, no flags -> auto", "0", false, false, false, false, OutputAuto},

		// --- env on: JSON is the default, explicit flags override it ---
		{"agentic, no flags -> json (the global default)", "1", false, false, false, false, OutputJSON},
		{"agentic, --json=true -> json", "1", true, true, false, false, OutputJSON},
		{"agentic, --json=false -> plain (escape hatch)", "1", true, false, false, false, OutputPlain},
		{"agentic, --json=false --quiet -> quiet", "1", true, false, false, true, OutputQuiet},
		{"agentic, --plain -> plain (escape hatch)", "1", false, false, true, false, OutputPlain},
		{"agentic, --quiet -> quiet (escape hatch)", "1", false, false, false, true, OutputQuiet},

		// --- explicit --json wins over a concurrent --plain ---
		{"agentic, --json=true beats --plain", "1", true, true, true, false, OutputJSON},
		{"unset, --json=false with --plain -> plain", "", true, false, true, false, OutputPlain},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BASHY_AGENTIC", tc.env)
			if got := ResolveOutputModeEx(tc.jsonSet, tc.jsonVal, tc.plain, tc.quiet); got != tc.want {
				t.Errorf("env=%q jsonSet=%v jsonVal=%v plain=%v quiet=%v => %v, want %v",
					tc.env, tc.jsonSet, tc.jsonVal, tc.plain, tc.quiet, got, tc.want)
			}
		})
	}
}

// The legacy bool-only ResolveOutputMode can't express --json=false, but must
// still honor the env as the fallback and let --json/--plain/--quiet win.
func TestResolveOutputMode_LegacyDelegation(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "1")
	if got := ResolveOutputMode(false, false, false); got != OutputJSON {
		t.Errorf("BASHY_AGENTIC=1, no flags => %v, want JSON", got)
	}
	if got := ResolveOutputMode(false, true, false); got != OutputPlain {
		t.Errorf("BASHY_AGENTIC=1 + --plain => %v, want Plain (explicit flag overrides env)", got)
	}
	t.Setenv("BASHY_AGENTIC", "")
	if got := ResolveOutputMode(false, false, false); got != OutputAuto {
		t.Errorf("unset, no flags => %v, want Auto", got)
	}
}

func TestEmitOK_JSONShape(t *testing.T) {
	var buf bytes.Buffer
	code := EmitOK(&buf, OutputJSON, "weave start", map[string]any{"issue": 123})
	if code != ExitOK {
		t.Errorf("EmitOK returned %d want %d", code, ExitOK)
	}
	var got Envelope
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SchemaVersion != SchemaVersion {
		t.Errorf("schema_version=%q want %q", got.SchemaVersion, SchemaVersion)
	}
	if got.Command != "weave start" {
		t.Errorf("command=%q want 'weave start'", got.Command)
	}
	if got.Status != "ok" {
		t.Errorf("status=%q want ok", got.Status)
	}
}

func TestEmitError_JSONShape(t *testing.T) {
	var buf bytes.Buffer
	code := EmitError(&buf, OutputJSON, "weave start", ExitPrecondFail, errors.New("queue empty"))
	if code != ExitPrecondFail {
		t.Errorf("EmitError returned %d want %d", code, ExitPrecondFail)
	}
	var got Envelope
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Status != "error" {
		t.Errorf("status=%q want error", got.Status)
	}
	if got.Error == nil || got.Error.Code != "precondition_failed" {
		t.Errorf("error envelope wrong: %+v", got.Error)
	}
	if !strings.Contains(got.Error.Message, "queue empty") {
		t.Errorf("error message lost: %q", got.Error.Message)
	}
}

func TestEmitError_PlainShape(t *testing.T) {
	var buf bytes.Buffer
	code := EmitError(&buf, OutputPlain, "weave add", ExitInvalidArg, errors.New("title required"))
	if code != ExitInvalidArg {
		t.Errorf("EmitError returned %d want %d", code, ExitInvalidArg)
	}
	got := buf.String()
	if !strings.Contains(got, "weave add:") || !strings.Contains(got, "title required") {
		t.Errorf("plain text format wrong: %q", got)
	}
}
