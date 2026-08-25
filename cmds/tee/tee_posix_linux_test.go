//go:build linux

package teecmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTeeIssue7FileWriteFailureContinues proves the Issue 7 consequence-of-
// errors rule: a write failure on one successfully opened file must not stop
// standard output or the other successfully opened file operands.
func TestTeeIssue7FileWriteFailureContinues(t *testing.T) {
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skipf("/dev/full is unavailable: %v", err)
	}
	dir := t.TempDir()
	out, errb, code := runToolDir(t, dir, "payload\n", "/dev/full", "ok")
	if code == 0 || out != "payload\n" ||
		!strings.Contains(errb, "tee: /dev/full:") {
		t.Fatalf("tee /dev/full ok = (%q, %q, %d)", out, errb, code)
	}
	if got := readFile(t, filepath.Join(dir, "ok")); got != "payload\n" {
		t.Fatalf("remaining file = %q, want payload", got)
	}
}
