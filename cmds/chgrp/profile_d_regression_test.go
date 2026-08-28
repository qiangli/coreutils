package chgrpcmd

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"
)

func TestChgrpStopsOptionScanningAtGroupOperand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chgrp is unix-only")
	}
	u, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	dir := t.TempDir()
	for _, name := range []string{"first", "-R", "--recursive", "--"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, errb, code := runTool(t, dir, u.Gid, "first", "-R", "--recursive", "--"); code != 0 || out != "" || errb != "" {
		t.Fatalf("chgrp option-looking operands = (%q, %q, %d)", out, errb, code)
	}
}
