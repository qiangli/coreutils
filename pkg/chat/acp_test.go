package chat

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/acp"
	"github.com/qiangli/coreutils/pkg/agentlaunch"
	"github.com/qiangli/coreutils/pkg/fleet"
)

// TestFakeACPAgentHelper is the FAKE ACP AGENT — a real subprocess speaking
// real JSON-RPC over real stdio, not a stub and not a live CLI.
//
// It is this test binary re-exec'd with CHAT_FAKE_ACP_AGENT=1, which is the
// standard way to get a hermetic child process out of a Go test: no toolchain
// at test time, no network, no third-party agent installed. It blocks until the
// parent closes the connection, so the testing package never gets a chance to
// print its own verdict onto the protocol stream.
func TestFakeACPAgentHelper(t *testing.T) {
	if os.Getenv("CHAT_FAKE_ACP_AGENT") != "1" {
		t.Skip("helper process; run via TestACPSessionIsDrivenOverTheProtocol")
	}
	agent := acp.NewAgent(
		acp.RunnerFunc(func(_ context.Context, req acp.TurnRequest) (acp.TurnResponse, error) {
			var b strings.Builder
			for _, p := range req.Prompt {
				b.WriteString(p.Text)
			}
			if strings.Contains(b.String(), "refuse") {
				return acp.TurnResponse{Text: "no", StopReason: acp.StopReasonRefusal}, nil
			}
			return acp.TurnResponse{Text: "ack:" + b.String(), StopReason: acp.StopReasonEndTurn}, nil
		}),
		acp.AgentOptions{},
		os.Stdin, os.Stdout,
	)
	<-agent.Done()
}

// pinACPCatalog installs a tool whose ACP launch is the helper above, and
// points the launcher at a store containing it.
func pinACPCatalog(t *testing.T) {
	t.Helper()
	// Keep the launcher off the developer's own home: room cards, shims and the
	// event dir all resolve from it.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BASHY_FORCE_AGENT_SHELL", "0")
	t.Setenv("CHAT_FAKE_ACP_AGENT", "1")

	root := t.TempDir()
	prev := newCatalog
	newCatalog = func() *fleet.Catalog { return fleet.New(fleet.WithRoot(root)) }
	t.Cleanup(func() { newCatalog = prev })

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	tool := fleet.Tool{
		Name: "fakeacp",
		Kind: fleet.ToolKindCLI,
		CLI: fleet.ToolCLI{
			Binary: self,
			Launch: fleet.ToolLaunch{
				// A headless one-shot it will never use here, so the resolution
				// path is the ordinary one.
				Exec: "fakeacp -run {prompt}",
				// Deliberately NO steer_exec: a tool can speak ACP and have no
				// interactive pty launch at all, and that must still be drivable.
				ACPExec: "fakeacp -test.run=^TestFakeACPAgentHelper$",
				ACPRung: "native",
			},
		},
	}
	if err := newCatalog().SaveTool(tool); err != nil {
		t.Fatalf("SaveTool: %v", err)
	}
}

// THE ACP PATH, END TO END, over a subprocess and a protocol.
func TestACPSessionIsDrivenOverTheProtocol(t *testing.T) {
	pinACPCatalog(t)
	t.Setenv(agentlaunch.ACPEnv, "1")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var streamed strings.Builder
	s, err := Start(ctx, "fakeacp", SessionOptions{
		Prompt: "hello",
		Cwd:    t.TempDir(),
		Stream: &streamed,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()

	if got := s.Rung(); got != agentlaunch.RungACPNative {
		t.Errorf("Rung = %s, want %s", got, agentlaunch.RungACPNative)
	}
	if s.CtlSock != "" {
		t.Errorf("an ACP session advertised a pty control socket: %q", s.CtlSock)
	}
	if s.EventsPath() != "" {
		t.Errorf("an ACP session advertised an events file: %q", s.EventsPath())
	}

	// THE POINT OF THE WHOLE EXERCISE: this returns when the agent SAYS the turn
	// ended, not after 25 seconds of silence. A quiet period is passed and
	// ignored; if it were being honoured this would take 30 seconds.
	started := time.Now()
	if err := s.WaitIdle(ctx, 30*time.Second); err != nil {
		t.Fatalf("WaitIdle: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 20*time.Second {
		t.Errorf("WaitIdle took %s — it waited for silence, not for the report", elapsed)
	}
	if got := s.ACPStopReason(); got != string(acp.StopReasonEndTurn) {
		t.Errorf("ACPStopReason = %q, want %q", got, acp.StopReasonEndTurn)
	}
	if got := s.Turn(); got != "ack:hello" {
		t.Errorf("Turn = %q, want %q", got, "ack:hello")
	}

	// A steer is the next prompt in the SAME session, and it is answered.
	if err := s.Say("again"); err != nil {
		t.Fatalf("Say: %v", err)
	}
	if err := s.WaitIdle(ctx, 30*time.Second); err != nil {
		t.Fatalf("WaitIdle after Say: %v", err)
	}
	if got := s.Turn(); got != "ack:again" {
		t.Errorf("Turn after Say = %q, want %q", got, "ack:again")
	}

	// A stop reason that is NOT end_turn arrives as a fact, where rung 4 would
	// have reported an ordinary quiet turn and called it success.
	if err := s.Say("please refuse"); err != nil {
		t.Fatalf("Say: %v", err)
	}
	if err := s.WaitIdle(ctx, 30*time.Second); err != nil {
		t.Fatalf("WaitIdle after refusal: %v", err)
	}
	if got := s.ACPStopReason(); got != string(acp.StopReasonRefusal) {
		t.Errorf("ACPStopReason = %q, want %q", got, acp.StopReasonRefusal)
	}

	// The transcript is the same tee every observer of a session already reads.
	if got := s.Output(); !strings.Contains(got, "ack:hello") || !strings.Contains(got, "ack:again") {
		t.Errorf("Output = %q, want both turns", got)
	}
	if got := streamed.String(); !strings.Contains(got, "ack:hello") {
		t.Errorf("Stream = %q, want the agent's text", got)
	}

	s.Close()
	if s.Live() {
		t.Error("session still live after Close")
	}
}

// THE NON-REGRESSION TEST, and the one that matters most.
//
// It would fail if the ACP branch leaked into the path every session in the
// fleet takes today. Both halves are checked at the branch point itself
// (startACPSession reports mine=false, having done NOTHING — no governance, no
// process, no room card) and at the outcome (Start produces the identical
// pre-ACP error for a tool with no steerable launch).
func TestNonACPLaunchesTakeExactlyTheOldPath(t *testing.T) {
	t.Run("gate off: even an ACP-declaring tool is untouched", func(t *testing.T) {
		pinACPCatalog(t)
		t.Setenv(agentlaunch.ACPEnv, "") // the default: opt-in, not opt-out

		s, mine, err := startACPSession(context.Background(), "fakeacp", SessionOptions{Prompt: "hi"})
		if mine || s != nil || err != nil {
			t.Fatalf("startACPSession = (%v, %v, %v), want (nil, false, nil)", s, mine, err)
		}
		if got := agentlaunch.EffectiveRung(acpToolLaunch(t)); got != agentlaunch.RungPTY {
			t.Errorf("EffectiveRung = %s, want %s", got, agentlaunch.RungPTY)
		}

		// And Start therefore reports what it reported before ACP existed: this
		// tool declares no steer_exec, so it cannot be steered.
		_, err = Start(context.Background(), "fakeacp", SessionOptions{Prompt: "hi"})
		if err == nil {
			t.Fatal("Start succeeded for a tool with no steerable launch")
		}
		if !strings.Contains(err.Error(), "cannot be steered") && !strings.Contains(err.Error(), "pty") {
			t.Errorf("Start error = %v, want the pre-ACP refusal", err)
		}
	})

	t.Run("gate on: a tool with no acp_exec is untouched", func(t *testing.T) {
		pinACPCatalog(t)
		t.Setenv(agentlaunch.ACPEnv, "1")

		// agy is the acceptance criterion by name: `agy -p` prints nothing and
		// exits 0 when stdout is not a TTY, so it MUST stay on rung 4.
		for _, name := range []string{"agy", "claude", "codex"} {
			s, mine, err := startACPSession(context.Background(), name, SessionOptions{Prompt: "hi"})
			if mine || s != nil || err != nil {
				t.Errorf("%s: startACPSession = (%v, %v, %v), want (nil, false, nil)", name, s, mine, err)
			}
		}
	})

	t.Run("gate on: an unknown tool is untouched", func(t *testing.T) {
		pinACPCatalog(t)
		t.Setenv(agentlaunch.ACPEnv, "1")

		s, mine, err := startACPSession(context.Background(), "no-such-tool-anywhere", SessionOptions{Prompt: "hi"})
		if mine || s != nil || err != nil {
			t.Fatalf("startACPSession = (%v, %v, %v), want (nil, false, nil)", s, mine, err)
		}
	})
}

func acpToolLaunch(t *testing.T) fleet.ToolLaunch {
	t.Helper()
	tool, ok := newCatalog().Tool("fakeacp")
	if !ok {
		t.Fatal("fakeacp is not in the pinned catalog")
	}
	return tool.CLI.Launch
}
