package agentlaunch

import (
	"testing"

	"github.com/qiangli/coreutils/pkg/fleet"
)

// THE LADDER TEST. All four rungs, chosen from declarations alone.
func TestRungForCoversAllFourRungs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		launch fleet.ToolLaunch
		want   Rung
		str    string
	}{
		{
			name:   "rung 1: acp_exec with acp_rung native",
			launch: fleet.ToolLaunch{ACPExec: "bashy acp", ACPRung: "native"},
			want:   RungACPNative,
			str:    "acp-native",
		},
		{
			name:   "rung 2: acp_exec with acp_rung adapter",
			launch: fleet.ToolLaunch{ACPExec: "npx @zed/claude-acp", ACPRung: "adapter"},
			want:   RungACPAdapter,
			str:    "acp-adapter",
		},
		{
			name:   "rung 3: events_arg, no acp_exec",
			launch: fleet.ToolLaunch{Exec: "ycode prompt {prompt}", EventsArg: "--events {path}"},
			want:   RungEvents,
			str:    "events",
		},
		{
			name:   "rung 4: neither",
			launch: fleet.ToolLaunch{Exec: "agy -p {prompt}"},
			want:   RungPTY,
			str:    "pty",
		},
		{
			// ACPExec is AUTHORITATIVE; ACPRung is advisory. A tool that declares
			// an exec and no rung is claiming to speak ACP itself.
			name:   "acp_exec with no acp_rung is native",
			launch: fleet.ToolLaunch{ACPExec: "thing acp"},
			want:   RungACPNative,
			str:    "acp-native",
		},
		{
			// The prize outranks the side channel: a tool that can do both is
			// driven over ACP.
			name:   "acp_exec outranks events_arg",
			launch: fleet.ToolLaunch{ACPExec: "ycode acp", EventsArg: "--events {path}"},
			want:   RungACPNative,
			str:    "acp-native",
		},
		{
			// Advisory means advisory: a rung nobody recognizes does not silently
			// demote a tool that declared an exec.
			name:   "unknown acp_rung falls to native",
			launch: fleet.ToolLaunch{ACPExec: "thing acp", ACPRung: "carrier-pigeon"},
			want:   RungACPNative,
			str:    "acp-native",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := RungFor(tc.launch); got != tc.want {
				t.Errorf("RungFor = %d (%s), want %d (%s)", got, got, tc.want, tc.want)
			}
			if got := RungFor(tc.launch).String(); got != tc.str {
				t.Errorf("String = %q, want %q", got, tc.str)
			}
		})
	}
}

func TestRungIsACP(t *testing.T) {
	for r, want := range map[Rung]bool{
		RungACPNative:  true,
		RungACPAdapter: true,
		RungEvents:     false,
		RungPTY:        false,
	} {
		if got := r.IsACP(); got != want {
			t.Errorf("%s.IsACP() = %v, want %v", r, got, want)
		}
	}
}

// The gate is the merge safety. Off (the default), an ACP-declaring tool is
// driven on the rung it was already on — one step down the SAME ladder, never
// a drop.
func TestEffectiveRungIsGatedByTheOptIn(t *testing.T) {
	acpOnly := fleet.ToolLaunch{ACPExec: "thing acp", ACPRung: "native"}
	acpAndEvents := fleet.ToolLaunch{ACPExec: "ycode acp", EventsArg: "--events {path}"}
	events := fleet.ToolLaunch{EventsArg: "--events {path}"}
	pty := fleet.ToolLaunch{Exec: "agy -p {prompt}"}

	// Default: the env var is unset for the whole process under `go test`, but
	// pin it so a developer's exported BASHY_ACP cannot make this pass wrongly.
	t.Setenv(ACPEnv, "")
	if ACPEnabled() {
		t.Fatal("ACPEnabled() with an empty BASHY_ACP")
	}
	for l, want := range map[*fleet.ToolLaunch]Rung{
		&acpOnly:      RungPTY,    // nothing else declared: back to the silence heuristic
		&acpAndEvents: RungEvents, // it also has a real turn.end: use that
		&events:       RungEvents,
		&pty:          RungPTY,
	} {
		if got := EffectiveRung(*l); got != want {
			t.Errorf("gate off: EffectiveRung(%+v) = %s, want %s", *l, got, want)
		}
	}

	t.Setenv(ACPEnv, "1")
	if !ACPEnabled() {
		t.Fatal("ACPEnabled() with BASHY_ACP=1")
	}
	for l, want := range map[*fleet.ToolLaunch]Rung{
		&acpOnly:      RungACPNative,
		&acpAndEvents: RungACPNative,
		&events:       RungEvents,
		&pty:          RungPTY,
	} {
		if got := EffectiveRung(*l); got != want {
			t.Errorf("gate on: EffectiveRung(%+v) = %s, want %s", *l, got, want)
		}
	}

	// The gate reads like every other bashy opt-in.
	for _, off := range []string{"", "0", "false", "off", "no", "  "} {
		t.Setenv(ACPEnv, off)
		if ACPEnabled() {
			t.Errorf("BASHY_ACP=%q enabled ACP", off)
		}
	}
	for _, on := range []string{"1", "true", "yes", "on"} {
		t.Setenv(ACPEnv, on)
		if !ACPEnabled() {
			t.Errorf("BASHY_ACP=%q did not enable ACP", on)
		}
	}
}

// agy MUST NOT MOVE. `agy -p` prints nothing and exits 0 when stdout is not a
// TTY, so its whole reason for being on rung 4 is that the pty IS the
// transport. This is the acceptance criterion, pinned against the compiled-in
// baseline catalog rather than a hand-written struct.
func TestBaselineToolsStayOnTheRungTheyAreOnToday(t *testing.T) {
	t.Setenv(ACPEnv, "1") // even with ACP fully on
	cat := fleet.New(fleet.WithRoot(t.TempDir()))
	for name, want := range map[string]Rung{
		"agy":      RungPTY,
		"claude":   RungPTY,
		"codex":    RungPTY,
		"aider":    RungPTY,
		"opencode": RungPTY,
	} {
		tool, ok := cat.Tool(name)
		if !ok {
			t.Errorf("%s is not in the baseline catalog", name)
			continue
		}
		if got := RungFor(tool.CLI.Launch); got != want {
			t.Errorf("%s: RungFor = %s, want %s", name, got, want)
		}
		if got := EffectiveRung(tool.CLI.Launch); got != want {
			t.Errorf("%s: EffectiveRung = %s, want %s", name, got, want)
		}
		if _, _, ok := ACPArgv(tool, ""); ok {
			t.Errorf("%s: renders an ACP argv but declares no acp_exec", name)
		}
	}
}

func TestACPArgvRendersTheTemplate(t *testing.T) {
	tool := fleet.Tool{
		Name: "thing",
		CLI:  fleet.ToolCLI{Binary: "/opt/thing/bin/thing", Launch: fleet.ToolLaunch{ACPExec: "thing acp --model {model}"}},
	}
	bin, args, ok := ACPArgv(tool, "gpt-9")
	if !ok {
		t.Fatal("ACPArgv reported false for a declared template")
	}
	if bin != "/opt/thing/bin/thing" {
		t.Errorf("bin = %q, want the tool's declared binary", bin)
	}
	if got, want := len(args), 3; got != want {
		t.Fatalf("args = %q, want %d", args, want)
	}
	if args[0] != "acp" || args[1] != "--model" || args[2] != "gpt-9" {
		t.Errorf("args = %q", args)
	}

	// No model selected: {model} and the flag holding it both go, exactly as
	// the headless template renders it.
	_, args, _ = ACPArgv(tool, "")
	if len(args) != 1 || args[0] != "acp" {
		t.Errorf("no-model args = %q, want [acp]", args)
	}

	// There is no {prompt} in an ACP template: the prompt travels in the
	// session. A template carrying one is rendered literally, never as a slot.
	if _, _, ok := ACPArgv(fleet.Tool{Name: "x"}, "m"); ok {
		t.Error("ACPArgv reported true for a tool with no acp_exec")
	}
}
