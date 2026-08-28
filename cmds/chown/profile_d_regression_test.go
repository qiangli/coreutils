package chowncmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestChownStopsOptionScanningAtOwnerOperand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chown is unix-only")
	}
	u := currentUser(t)
	dir := t.TempDir()
	for _, name := range []string{"first", "-R", "--recursive", "--"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	spec := u.Uid + ":" + u.Gid
	if out, errb, code := runTool(t, dir, spec, "first", "-R", "--recursive", "--"); code != 0 || out != "" || errb != "" {
		t.Fatalf("chown option-looking operands = (%q, %q, %d)", out, errb, code)
	}
}
