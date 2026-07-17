package agentlaunch

import (
	"reflect"
	"testing"
)

// TestStripKillSwitches — the attended transform removes the auto-approve
// kill-switches (so the tool's own approval gate stays on and the uncontained-
// host guard has nothing to refuse) while leaving write flags and model args
// intact, and downgrades --sandbox danger-full-access to the tool default.
func TestStripKillSwitches(t *testing.T) {
	in := []string{
		"--danger-skip-permissions", "--model", "deepseek-v4-pro",
		"--dangerously-skip-permissions", "--sandbox", "danger-full-access", "prompt",
	}
	got := StripKillSwitches("ycode", in)
	want := []string{"--model", "deepseek-v4-pro", "prompt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("StripKillSwitches = %v, want %v", got, want)
	}

	// And its whole point: what it produces passes the guard on an uncontained host.
	if err := GuardUnsafeArgs("ycode", got); err != nil {
		t.Fatalf("stripped argv should pass the guard, got: %v", err)
	}
	// A sandbox mode that is not danger-full-access is preserved.
	if got := StripKillSwitches("codex", []string{"--sandbox", "workspace-write", "--yolo"}); !reflect.DeepEqual(got, []string{"--sandbox", "workspace-write"}) {
		t.Fatalf("workspace-write sandbox should survive, got %v", got)
	}
}
