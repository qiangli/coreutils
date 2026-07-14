package agentctl

import (
	"strings"
	"testing"
)

// THE REGRESSION. This is agy's real output, verbatim. The binding
// `agy:gemini3.1` sat in the fleet listed, banded and routable, and failed every
// single time it was asked to speak — while the ledger recorded it as "medium
// reliability, needs interactive sign-in first". It was never signing in. It was
// never launching.
func TestClassifyCatchesTheDeadModelBinding(t *testing.T) {
	raw := `Error: invalid --model "gemini-3.1": model gemini-3.1 is not recognized ` +
		`as a known model or custom model in settings
Available models:
  Gemini 3.5 Flash (Medium)
  Gemini 3.1 Pro (High)`

	st, note := Classify(raw, false)
	if st != ProbeBadModel {
		t.Fatalf("Classify = %q (%s), want %q — a dead model binding must be named as one", st, note, ProbeBadModel)
	}
	if st.OK() {
		t.Error("a dead binding must not report OK")
	}
}

// A model rejection is checked BEFORE a flag rejection, because a tool that
// rejects a model usually prints a usage block too. Reporting that as a stale
// contract sends the operator to edit an argv template that is perfectly fine.
func TestModelErrorWinsOverTheUsageBlockItPrints(t *testing.T) {
	raw := "Error: invalid --model \"x\"\nUsage: agy [options]"
	if st, _ := Classify(raw, false); st != ProbeBadModel {
		t.Errorf("Classify = %q, want %q — the usage block is a symptom, not the cause", st, ProbeBadModel)
	}
}

// THE SECOND ONE, found by the probe itself the first time it ran — and reported
// OK, because the first version of Classify passed anything that produced output
// and matched no known failure signature.
//
// A verifier that passes on the ABSENCE of a known failure is not a verifier; it
// is a list of the failures somebody already thought of. Only PROBE_OK passes.
func TestClassifyRefusesToPassWithoutPositiveEvidence(t *testing.T) {
	// aider:deepseek-v4, verbatim. Every run of this agent dies here.
	raw := `litellm.BadRequestError: DeepseekException - {"error":{"message":"The supported ` +
		`API model names are deepseek-v4-pro or deepseek-v4-flash, but you passed deepseek-v4."}}`
	if st, note := Classify(raw, false); st.OK() {
		t.Fatalf("Classify = %q (%s) — a dead binding must never pass just because its error was unfamiliar", st, note)
	}
	if st, _ := Classify(raw, false); st != ProbeBadModel {
		t.Errorf("Classify = %q, want %q so the operator knows to re-peg the model", st, ProbeBadModel)
	}

	// And the general case: output nobody has a signature for still FAILS.
	st, note := Classify("some novel error nobody has ever seen", false)
	if st.OK() {
		t.Errorf("unrecognised output must fail, not pass: %q (%s)", st, note)
	}
}

func TestClassifyVerdicts(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		timedOut bool
		want     ProbeStatus
	}{
		{"answered", "PROBE_OK", false, ProbeOK},
		{"stale flag", "error: unexpected argument '--workspace'", false, ProbeStaleContract},
		{"needs login", "You are currently not signed in.", false, ProbeNeedsAuth},
		{"trust prompt", "Do you trust the contents of this folder?", false, ProbeNeedsAuth},
		// Parked at a prompt: ran to the deadline having said nothing. This is the
		// shape of a tool waiting for a human who is not there.
		{"stalled silent", "", true, ProbeNeedsAuth},
		{"timed out talking", "thinking…" + string(make([]byte, 60)), true, ProbeFailed},
		{"said nothing", "", false, ProbeFailed},
		// Said SOMETHING, but not the word. Fails: a pass must be earned by
		// evidence, not by the absence of a signature we happened to list.
		{"answered without the token", "Sure! The answer is OK.", false, ProbeFailed},
		{"answered with the token", "here you go: PROBE_OK", false, ProbeOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, note := Classify(c.raw, c.timedOut); got != c.want {
				t.Errorf("Classify = %q (%s), want %q", got, note, c.want)
			}
		})
	}
}

// A terminal is given only to a tool that can use one. codex and agy declare
// supports_say=false; handing them a pty merges stderr into the answer and buys
// nothing, because neither listens.
func TestNeedsTerminal(t *testing.T) {
	if (Profile{Steerable: true}).NeedsTerminal() != true {
		t.Error("a steerable tool needs a terminal")
	}
	if (Profile{Clear: "say:1"}).NeedsTerminal() != true {
		t.Error("a tool with a trust prompt to clear needs a terminal")
	}
	if (Profile{}).NeedsTerminal() != false {
		t.Error("a tool that neither listens nor prompts must get a pipe")
	}
}

// A rejected credential is not a mystery failure. Both kimi agents were reporting
// "failed" when what they actually needed was a Moonshot API key — and "failed"
// sends an operator hunting through a registry that is perfectly fine.
func TestClassifyNamesARejectedCredential(t *testing.T) {
	raw := "litellm.AuthenticationError: AuthenticationError: MoonshotException - The provided API key is invalid"
	st, note := Classify(raw, false)
	if st != ProbeNeedsAuth {
		t.Fatalf("Classify = %q (%s), want %q so the operator is sent to `bashy secrets`", st, note, ProbeNeedsAuth)
	}
	if st.OK() {
		t.Error("an agent that cannot authenticate is not usable")
	}
}

// The excerpt must not be the banner. Agent CLIs open with a box-drawing rule, so
// "the first line" is reliably `────────` — which tells nobody anything.
func TestFirstLinePrefersTheErrorOverTheBanner(t *testing.T) {
	raw := "────────────────────\nAider v0.86.2\nMain model: x\n\nlitellm.BadRequestError: boom\n"
	if got := firstLine(raw); !strings.Contains(got, "BadRequestError") {
		t.Errorf("firstLine = %q, want the error, not the banner", got)
	}
}
