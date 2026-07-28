package gate

import (
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
