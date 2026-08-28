package chmodcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestChmodStopsOptionScanningAtModeOperand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	for _, name := range []string{"first", "-R", "--recursive", "--"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if out, errb, code := runTool(t, dir, "ugo=rw", "first", "-R", "--recursive", "--"); code != 0 || out != "" || errb != "" {
		t.Fatalf("chmod option-looking operands = (%q, %q, %d)", out, errb, code)
	}
	for _, name := range []string{"first", "-R", "--recursive", "--"} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("stat %s: %v", name, err)
			continue
		}
		if fi.Mode().Perm() != 0o666 {
			t.Errorf("%s mode = %v, want 0666", name, fi.Mode().Perm())
		}
	}
}

func TestChmodDashModeRetainsOperandBoundary(t *testing.T) {
	mode, rest := extractDashMode([]string{"-w", "-R", "--recursive"})
	if mode != "-w" || len(rest) != 3 || rest[0] != "--" || rest[1] != "-R" || rest[2] != "--recursive" {
		t.Fatalf("dash mode boundary = mode %q rest %q", mode, rest)
	}
}
