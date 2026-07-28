package gate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Moved here with attestEnv when the gate primitive left pkg/herald to break
// the herald <-> acp import cycle. The guarantee is unchanged and belongs with
// the code it constrains.
// TestAttestEnvExcludesSecrets pins that a gate does not inherit the operator's
// vault. A gate is operator-supplied code run to judge a REMOTE party's work;
// handing it every credential in the environment would be a needless blast
// radius.
func TestAttestEnvExcludesSecrets(t *testing.T) {
	t.Setenv("GATE_TEST_FAKE_SECRET", "super-secret-value")
	env := attestEnv("/tmp")
	for _, kv := range env {
		if strings.HasPrefix(kv, "GATE_TEST_FAKE_SECRET=") {
			t.Fatal("gate environment must not carry arbitrary parent env vars")
		}
	}
	var sawPath bool
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			sawPath = true
		}
	}
	if !sawPath {
		t.Error("gate environment should preserve PATH so build/test gates work")
	}
}

func TestRunLocalSilentSuccessAbstains(t *testing.T) {
	requirePOSIXShell(t)
	o := RunLocal(context.Background(), t.TempDir(), "exit 0", "completed")
	if !o.Ran || o.Passed || !o.Abstained || o.Trusted() {
		t.Fatalf("silent local gate must abstain, got %+v", o)
	}
}

func TestRunLocalMutationProbe(t *testing.T) {
	requirePOSIXShell(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "MUTATED")
	probe := &MutationProbe{
		Name: "marker appears",
		Mutate: func() (func() error, error) {
			if err := os.WriteFile(marker, []byte("mutation"), 0o644); err != nil {
				return nil, err
			}
			return func() error { return os.Remove(marker) }, nil
		},
	}
	command := "if test -f MUTATED; then echo mutation; exit 2; else echo healthy; fi"
	o := RunLocalWithProbe(context.Background(), dir, command, "completed", probe)
	if !o.Passed || !o.Trusted() || !o.Probe.Proved {
		t.Fatalf("mutation-proved local gate did not pass: %+v", o)
	}
}
