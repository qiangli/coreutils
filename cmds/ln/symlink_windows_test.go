//go:build windows

package lncmd

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCreateSymlinkWindows drives the real path on a Windows host: with
// the privilege (an admin runner, or Developer Mode) a symbolic link whose
// content is the NATIVE spelling of the operand; without it, a hard link
// or copy that reads (and runs) as the source does.
func TestCreateSymlinkWindows(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.exe")
	if err := os.WriteFile(src, []byte("MZ-not-really"), 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "bash")
	if err := createSymlink(src, dst, src); err != nil {
		t.Fatalf("createSymlink: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "MZ-not-really" {
		t.Fatalf("destination content = %q, err %v", got, err)
	}
	fi, err := os.Lstat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		if target, _ := os.Readlink(dst); target != src {
			t.Errorf("symlink content = %q, want the native %q", target, src)
		}
	} else {
		si, _ := os.Stat(src)
		if !os.SameFile(si, fi) && fi.Size() != si.Size() {
			t.Errorf("fallback produced neither a hard link nor a copy of the source")
		}
	}

	// Shell-spelled operands become native link content.
	cases := []struct{ in, want string }{
		{"/c/Users/x/bash.exe", `C:\Users\x\bash.exe`},
		{`C:\already\native`, `C:\already\native`},
		{"../rel/x", `..\rel\x`},
		{"x*x", "x\uf02ax"},
	}
	for _, c := range cases {
		if got := nativeLinkContent(c.in); got != c.want {
			t.Errorf("nativeLinkContent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
