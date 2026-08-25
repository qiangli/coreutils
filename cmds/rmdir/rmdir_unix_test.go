//go:build unix

package rmdircmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRmdirPermissionDeniedContinuesAfterOperand covers operand continuation
// when a directory entry cannot be unlinked because its parent denies write
// permission: the failing operand is diagnosed, the exit status reflects the
// failure, and a later operand is still processed.
func TestRmdirPermissionDeniedContinuesAfterOperand(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can bypass directory write permissions")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	child := filepath.Join(locked, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	_, errb, code := runTool(t, dir, filepath.Join("locked", "child"), "ok")
	if code != 1 {
		t.Fatalf("code=%d, want 1; err=%q", code, errb)
	}
	if !strings.Contains(errb, "failed to remove '"+filepath.Join("locked", "child")+"'") {
		t.Fatalf("missing permission diagnostic: %q", errb)
	}
	if _, err := os.Stat(child); err != nil {
		t.Fatalf("child removed despite non-writable parent: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ok")); !os.IsNotExist(err) {
		t.Fatalf("later operand not removed after permission failure: %v", err)
	}
}
