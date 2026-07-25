package notify

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/room"
)

// isolate points the room store at a private tempdir.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())
}

// run executes the command with args, returning stdout, stderr and the error.
func run(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewCommand()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

// A published notification must actually land on the timeline, carrying who sent
// it and how it was addressed. Without this, "publish succeeded" is an assertion
// about a function call rather than about the bus.
func TestPublishLandsOnTheTimeline(t *testing.T) {
	isolate(t)

	if _, _, err := run(t, "--topic", "build", "--principal", "alice", "rebase", "first"); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	events, err := room.Timeline(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	e := events[0]
	if e.Type != room.EventNotify {
		t.Errorf("type = %q, want %q", e.Type, room.EventNotify)
	}
	if e.Principal != "alice" {
		t.Errorf("principal = %q, want alice", e.Principal)
	}
	if e.Topic != "build" {
		t.Errorf("topic = %q, want build", e.Topic)
	}
	if e.Body != "rebase first" {
		t.Errorf("body = %q, want %q", e.Body, "rebase first")
	}
}

// Addressing is required, and it was documented as required long before it was
// enforced. An unaddressed publish reaches nobody: no topic to match, no
// recipient to route to, no room to scope it. Accepting it and exiting 0 reports
// success for something that cannot be delivered.
func TestUnaddressedPublishIsRefused(t *testing.T) {
	isolate(t)

	_, _, err := run(t, "--principal", "alice", "to nobody in particular")
	if err == nil {
		t.Fatal("an unaddressed notification was accepted — it can never be delivered")
	}
	if !strings.Contains(err.Error(), "addressing is required") {
		t.Errorf("error should name the missing addressing, got: %v", err)
	}

	events, _ := room.Timeline(0)
	if len(events) != 0 {
		t.Errorf("a refused publish still wrote %d event(s) to the timeline", len(events))
	}
}

// Each addressing form on its own is sufficient.
func TestEachAddressingFormIsAccepted(t *testing.T) {
	for _, flag := range []string{"--topic", "--to", "--room"} {
		t.Run(flag, func(t *testing.T) {
			isolate(t)
			if _, _, err := run(t, flag, "x", "--principal", "alice", "msg"); err != nil {
				t.Errorf("%s alone was refused: %v", flag, err)
			}
			events, _ := room.Timeline(0)
			if len(events) != 1 {
				t.Errorf("got %d events, want 1", len(events))
			}
		})
	}
}

// The REPORT/AUTHOR invariant: a notification nobody can be attributed to is not
// a notification. $USER is the fallback, so this has to clear it explicitly.
func TestPrincipalIsRequired(t *testing.T) {
	isolate(t)
	t.Setenv("BASHY_PRINCIPAL", "")
	t.Setenv("USER", "")

	_, _, err := run(t, "--topic", "build", "unattributed")
	if err == nil {
		t.Fatal("a publish with no principal was accepted")
	}
	if !strings.Contains(err.Error(), "principal") {
		t.Errorf("error should name the missing principal, got: %v", err)
	}
	events, _ := room.Timeline(0)
	if len(events) != 0 {
		t.Errorf("a refused publish still wrote %d event(s)", len(events))
	}
}

func TestPrincipalFallsBackToEnvironment(t *testing.T) {
	isolate(t)
	t.Setenv("BASHY_PRINCIPAL", "from-env")

	if _, _, err := run(t, "--topic", "build", "msg"); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	events, _ := room.Timeline(0)
	if len(events) != 1 || events[0].Principal != "from-env" {
		t.Errorf("principal was not taken from BASHY_PRINCIPAL: %+v", events)
	}
}

// --json must be machine-readable on BOTH paths. The error envelope matters more
// than the success one: an agent that cannot parse a refusal will treat it as a
// crash rather than as a fixable mistake.
func TestJSONEnvelopes(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		isolate(t)
		out, _, err := run(t, "--topic", "build", "--principal", "alice", "--json", "msg")
		if err != nil {
			t.Fatalf("publish failed: %v", err)
		}
		var env Envelope
		if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
			t.Fatalf("stdout is not valid JSON: %v\n%s", jerr, out)
		}
		if env.SchemaVersion != SchemaVersion || env.Status != "ok" {
			t.Errorf("envelope = %+v", env)
		}
		if env.Principal != "alice" || env.Topic != "build" {
			t.Errorf("envelope lost its addressing: %+v", env)
		}
	})

	t.Run("error", func(t *testing.T) {
		isolate(t)
		_, errOut, err := run(t, "--principal", "alice", "--json", "unaddressed")
		if err == nil {
			t.Fatal("expected a refusal")
		}
		var env Envelope
		if jerr := json.Unmarshal([]byte(errOut), &env); jerr != nil {
			t.Fatalf("stderr is not valid JSON: %v\n%s", jerr, errOut)
		}
		if env.Status != "error" || env.Error == "" {
			t.Errorf("error envelope = %+v", env)
		}
	})
}

// A message is required — publishing an empty notification is a no-op that looks
// like a delivery.
func TestMessageIsRequired(t *testing.T) {
	isolate(t)
	if _, _, err := run(t, "--topic", "build", "--principal", "alice"); err == nil {
		t.Error("a publish with no message was accepted")
	}
}
