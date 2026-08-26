package envcmd

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// explodingReader fails on any Read. env must never touch standard input when
// it has no utility operand, so a print-only invocation with this stdin still
// succeeds.
type explodingReader struct{}

func (explodingReader) Read([]byte) (int, error) { return 0, fmt.Errorf("stdin must not be read") }

// TestEnvNoUtilityDoesNotReadStdin pins the Issue 7 STDIN clause: "The env
// utility shall not read its standard input" in the no-utility form. If env
// read stdin here the exploding reader would surface an error.
func TestEnvNoUtilityDoesNotReadStdin(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   []string{"A=1"},
		Stdio: tool.Stdio{In: explodingReader{}, Out: &out, Err: &errb},
	}
	if code := cmd.Run(rc, nil); code != 0 || errb.String() != "" || out.String() != "A=1\n" {
		t.Fatalf("no-utility env = (%q, %q, %d), want (%q, '', 0)", out.String(), errb.String(), code, "A=1\n")
	}
}

// TestEnvValueContainsEquals proves that only the first '=' delimits NAME from
// VALUE, so an embedded '=' is data preserved byte-for-byte on output.
func TestEnvValueContainsEquals(t *testing.T) {
	out, errb, code := runTool(t, nil, "A=B=C")
	if code != 0 || errb != "" || out != "A=B=C\n" {
		t.Fatalf("A=B=C = (%q, %q, %d), want (%q, '', 0)", out, errb, code, "A=B=C\n")
	}
}

// TestEnvErrorStatusPartition pins that a failure in env itself (unrelated to
// the utility being found or executable) exits in the 1-125 band, not 126/127
// which are reserved for utility invocation outcomes.
func TestEnvErrorStatusPartition(t *testing.T) {
	// --chdir to a nonexistent directory: env cannot set up the requested
	// state, so it fails before any utility lookup.
	_, errb, code := runTool(t, []string{"PATH=/nonexistent"}, "--chdir", "definitely-not-here", "some-utility")
	if code < 1 || code > 125 {
		t.Fatalf("chdir failure status = %d, want a 1-125 env error (stderr %q)", code, errb)
	}
	if !strings.Contains(errb, "cannot change directory") {
		t.Fatalf("chdir failure diagnostic missing: %q", errb)
	}
	// A --file operand that cannot be read is likewise an env error in 1-125.
	_, errb, code = runTool(t, []string{"A=1"}, "--file", filepath.Join(t.TempDir(), "no-such-envfile"))
	if code < 1 || code > 125 {
		t.Fatalf("missing --file status = %d, want a 1-125 env error (stderr %q)", code, errb)
	}
	if !strings.Contains(errb, "env:") {
		t.Fatalf("missing --file diagnostic absent: %q", errb)
	}
}
