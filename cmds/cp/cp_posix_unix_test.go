//go:build unix

package cpcmd

// -p S_ISUID/S_ISGID evidence. Both tests go through the chownFn seam
// because the natural failure (chown to a foreign uid) needs root to
// arm and the natural side effect (a kernel clearing setuid on a no-op
// chown) is platform-dependent; the seam makes both deterministic.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func setuidSource(t *testing.T, dir string) string {
	t.Helper()
	src := filepath.Join(dir, "src")
	write(t, src, "payload")
	if err := os.Chmod(src, os.FileMode(0o755)|os.ModeSetuid); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(src)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSetuid == 0 {
		t.Skip("filesystem does not retain the setuid bit")
	}
	return src
}

// TestCpPreserveModeAppliedAfterOwnership pins the load-bearing -p
// ordering: chmod must run after chown, because several kernels clear
// S_ISUID/S_ISGID as a chown() side effect even when the owner chowns
// to the unchanged uid/gid. The seam simulates that side effect
// deterministically; the duplicated mode must still carry setuid.
func TestCpPreserveModeAppliedAfterOwnership(t *testing.T) {
	dir := t.TempDir()
	setuidSource(t, dir)
	old := chownFn
	chownFn = func(name string, uid, gid int) error {
		if fi, err := os.Lstat(name); err == nil {
			_ = os.Chmod(name, fi.Mode().Perm()) // kernel-style suid/sgid strip
		}
		return os.Chown(name, uid, gid)
	}
	defer func() { chownFn = old }()

	_, errb, code := runTool(t, dir, "-p", "src", "dst")
	if code != 0 || errb != "" {
		t.Fatalf("cp -p: code=%d err=%q", code, errb)
	}
	fi, err := os.Lstat(filepath.Join(dir, "dst"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSetuid == 0 {
		t.Fatalf("S_ISUID not duplicated when ownership duplication succeeded: mode=%v", fi.Mode())
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("permission bits = %03o, want 755", fi.Mode().Perm())
	}
}

// TestCpPreserveClearsSetuidWhenOwnershipFails pins the Issue 7 -p
// shall: "If the user ID or group ID cannot be duplicated, the file
// permission bits S_ISUID and S_ISGID shall be cleared." The failure
// itself is not an error (no diagnostic is required and GNU treats it
// as success), so the run still exits 0.
func TestCpPreserveClearsSetuidWhenOwnershipFails(t *testing.T) {
	dir := t.TempDir()
	setuidSource(t, dir)
	old := chownFn
	chownFn = func(name string, uid, gid int) error {
		return errors.New("operation not permitted")
	}
	defer func() { chownFn = old }()

	_, errb, code := runTool(t, dir, "-p", "src", "dst")
	if code != 0 || errb != "" {
		t.Fatalf("cp -p with failed ownership: code=%d err=%q, want silent success", code, errb)
	}
	fi, err := os.Lstat(filepath.Join(dir, "dst"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		t.Fatalf("S_ISUID/S_ISGID not cleared after ownership-duplication failure: mode=%v", fi.Mode())
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("permission bits = %03o, want 755", fi.Mode().Perm())
	}
}
