package edcmd

import (
	"strings"
	"testing"
)

// An initial file operand is loaded before command mode begins. Its diagnostic
// belongs on stderr; the command-mode "?" marker must not leak to stdout.
func TestMissingInitialFileHasNoCommandModeMarker(t *testing.T) {
	code, out, stderr, _ := runEd(t, "q\n", "-s", "missing-initial-file")
	if code == 0 {
		t.Fatalf("missing initial file exited zero")
	}
	if out != "" {
		t.Fatalf("stdout = %q, want empty", out)
	}
	if !strings.Contains(stderr, "missing-initial-file") {
		t.Fatalf("stderr = %q, want missing operand name", stderr)
	}
}
