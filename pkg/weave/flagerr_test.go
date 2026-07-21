package weave

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/weavecli"
)

// runWeaveStreams drives the weave tree with stdout and stderr captured
// separately — the flag-error contract is specifically about STDERR.
func runWeaveStreams(t *testing.T, args ...string) (stdout, stderr string, code int, structured bool) {
	t.Helper()
	cmd := newWeaveCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errb.String(), ExitCode(err), IsStructuredExit(err)
}

// TestUnknownFlagFailsLoud pins the fix for the silent-flag-error bug: a
// misspelled flag on ANY weave subverb must name itself on stderr and
// exit 2. `weave baton write --note ...` (instead of --notes) silently
// dropped a conductor checkpoint — the handoff-correctness case.
func TestUnknownFlagFailsLoud(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "") // human rows, not the agent JSON envelope

	cases := []struct {
		name     string
		args     []string
		wantFlag string
		wantHint string // nearest-valid-flag suggestion, "" if none required
	}{
		{"baton write typo", []string{"baton", "write", "--note", "x"}, "--note", "--notes"},
		{"list typo", []string{"list", "--jsonn"}, "--jsonn", "--json"},
		{"root typo", []string{"--bogus"}, "--bogus", ""},
		{"start typo", []string{"start", "--too"}, "--too", "--tool"},
		{"shorthand typo", []string{"list", "-Z"}, "-Z", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code, structured := runWeaveStreams(t, tc.args...)
			if code != weavecli.ExitInvalidArg {
				t.Errorf("exit = %d, want %d (usage error); stderr=%q", code, weavecli.ExitInvalidArg, stderr)
			}
			if strings.TrimSpace(stderr) == "" {
				t.Fatalf("stderr must not be empty for %v (stdout=%q)", tc.args, stdout)
			}
			if !strings.Contains(stderr, tc.wantFlag) {
				t.Errorf("stderr should name the offending flag %q, got %q", tc.wantFlag, stderr)
			}
			if tc.wantHint != "" && !strings.Contains(stderr, tc.wantHint) {
				t.Errorf("stderr should suggest %q, got %q", tc.wantHint, stderr)
			}
			// weave reported it itself, so a host driving Execute() must
			// stay silent rather than double-print.
			if !structured {
				t.Errorf("flag error should be a structured exit (already reported), stderr=%q", stderr)
			}
		})
	}
}

// TestUnknownFlagJSONEnvelope: flags parsed before the offending one
// still shape the output, so an agent that asked for --json gets an
// invalid_arg envelope rather than a human line it cannot parse.
func TestUnknownFlagJSONEnvelope(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	_, stderr, code, _ := runWeaveStreams(t, "list", "--json", "--bogus")
	if code != weavecli.ExitInvalidArg {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, weavecli.ExitInvalidArg, stderr)
	}
	var env weavecli.Envelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr should be a JSON envelope, got %q (%v)", stderr, err)
	}
	if env.Status != "error" || env.Error == nil {
		t.Fatalf("envelope should carry an error, got %+v", env)
	}
	if env.Error.Code != "invalid_arg" {
		t.Errorf("error code = %q, want invalid_arg", env.Error.Code)
	}
	if !strings.Contains(env.Error.Message, "--bogus") {
		t.Errorf("message should name the flag, got %q", env.Error.Message)
	}
}

func TestUnknownFlagJSONEnvelopeCoversGuideSubverb(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	_, stderr, code, _ := runWeaveStreams(t, "guide", "--json", "--bogus")
	if code != weavecli.ExitInvalidArg {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, weavecli.ExitInvalidArg, stderr)
	}
	var env weavecli.Envelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("guide should honor --json for flag errors, got %q (%v)", stderr, err)
	}
	if env.Error == nil {
		t.Fatalf("envelope should carry an error, got %+v", env)
	}
	if !strings.Contains(env.Error.Message, "--bogus") {
		t.Fatalf("message should name the offending flag after --json, got %q", env.Error.Message)
	}
}

func TestMissingFlagArgumentDoesNotSuggestSameFlag(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	_, stderr, code, _ := runWeaveStreams(t, "start", "--tool")
	if code != weavecli.ExitInvalidArg {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, weavecli.ExitInvalidArg, stderr)
	}
	if !strings.Contains(stderr, "flag needs an argument: --tool") {
		t.Fatalf("stderr should report the missing value for --tool, got %q", stderr)
	}
	if strings.Contains(stderr, "did you mean --tool?") {
		t.Fatalf("stderr should not suggest the same valid flag for a missing value, got %q", stderr)
	}
}

func TestMissingFlagArgumentJSONDoesNotSuggestSameFlag(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	_, stderr, code, _ := runWeaveStreams(t, "start", "--json", "--tool")
	if code != weavecli.ExitInvalidArg {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, weavecli.ExitInvalidArg, stderr)
	}
	var env weavecli.Envelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		t.Fatalf("stderr should be a JSON envelope, got %q (%v)", stderr, err)
	}
	if env.Error == nil {
		t.Fatalf("envelope should carry an error, got %+v", env)
	}
	if !strings.Contains(env.Error.Message, "flag needs an argument: --tool") {
		t.Fatalf("message should report the missing value for --tool, got %q", env.Error.Message)
	}
	if strings.Contains(env.Error.Message, "did you mean --tool?") {
		t.Fatalf("message should not suggest the same valid flag for a missing value, got %q", env.Error.Message)
	}
}

func TestNearestFlagSuggestion(t *testing.T) {
	cmd := newWeaveBatonWriteCmd()
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"note", "notes", true},
		{"noets", "notes", true},
		{"stag", "stage", true},
		{"zzzzzzzzzzzzz", "", false},
	} {
		got, ok := nearestFlag(cmd, tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("nearestFlag(%q) = (%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestUnknownFlagName(t *testing.T) {
	for _, tc := range []struct {
		msg, want string
		ok        bool
	}{
		{"unknown flag: --note", "note", true},
		{"flag needs an argument: --tool", "", false},
		{"unknown flag: --note=x", "note", true},
		{"unknown shorthand flag: 'Z' in -Z", "", false},
	} {
		got, ok := unknownFlagName(tc.msg)
		if ok != tc.ok || got != tc.want {
			t.Errorf("unknownFlagName(%q) = (%q,%v), want (%q,%v)", tc.msg, got, ok, tc.want, tc.ok)
		}
	}
}
