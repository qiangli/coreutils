//go:build !windows

package mkdircmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

func TestMkdirParentsRetainOwnerWriteAndSearch(t *testing.T) {
	dir := t.TempDir()
	old := unix.Umask(0o777)
	defer unix.Umask(old)
	defer func() {
		_ = os.Chmod(filepath.Join(dir, "a"), 0o700)
		_ = os.Chmod(filepath.Join(dir, "a", "b"), 0o700)
	}()

	_, errb, code := runTool(t, dir, "-p", filepath.Join("a", "b"))
	if code != 0 {
		t.Fatalf("mkdir -p under restrictive umask: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "a"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm() & 0o300; got != 0o300 {
		t.Fatalf("intermediate owner write/search = %03o, want 300", got)
	}
}

// TestMkdirParentsTrailingSlashFinalMode pins the POSIX pathname rule
// that "x/y/" and "x/y/." name the directory x/y: with -p, the final
// component must receive the -m mode (or the umask default), never the
// u+wx-augmented intermediate mode.
func TestMkdirParentsTrailingSlashFinalMode(t *testing.T) {
	dir := t.TempDir()
	// -m 505 has no owner-write bit; the intermediate mode always does
	// (u+wx is ORed in), so this distinguishes final from intermediate
	// treatment under any umask.
	_, errb, code := runTool(t, dir, "-p", "-m", "505", "x/y/")
	if code != 0 {
		t.Fatalf("mkdir -p -m 505 x/y/: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "x", "y"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o505 {
		t.Errorf("final mode with trailing slash = %o, want 505", got)
	}

	// Without -m, the final directory gets the a=rwx&~umask default, not
	// the (u+wx|~umask) intermediate mode. umask 0327 separates the two:
	// default 0450 vs intermediate-styled 0750.
	old := unix.Umask(0o327)
	defer unix.Umask(old)
	_, errb, code = runTool(t, dir, "-p", "p/q/")
	unix.Umask(old)
	if code != 0 {
		t.Fatalf("mkdir -p p/q/: code=%d err=%q", code, errb)
	}
	qfi, err := os.Stat(filepath.Join(dir, "p", "q"))
	if err != nil {
		t.Fatal(err)
	}
	if got := qfi.Mode().Perm(); got != 0o450 {
		t.Errorf("final default mode with trailing slash = %o, want 450 (umask 0327)", got)
	}

	// "x/y/." is the same directory again: with -p that is ignored
	// without error and without disturbing the mode.
	_, errb, code = runTool(t, dir, "-p", "x/y/.")
	if code != 0 || errb != "" {
		t.Fatalf("mkdir -p x/y/.: code=%d err=%q", code, errb)
	}
	fi2, err := os.Stat(filepath.Join(dir, "x", "y"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi2.Mode().Perm(); got != 0o505 {
		t.Errorf("mode after mkdir -p x/y/. = %o, want 505 unchanged", got)
	}
}

func TestMkdirParentsRejectsDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("missing-target", filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-p", "dangling")
	if code != 1 || !strings.Contains(errb, "cannot create directory") {
		t.Fatalf("mkdir -p dangling symlink: code=%d err=%q", code, errb)
	}
}

func TestMkdirParentsAcceptsSymlinkToDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-p", "link")
	if code != 0 || errb != "" {
		t.Fatalf("mkdir -p symlink to directory: code=%d err=%q", code, errb)
	}
	if fi, err := os.Lstat(filepath.Join(dir, "link")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link entry was not preserved as a symlink: mode=%v err=%v", fi.Mode(), err)
	}
}

func TestMkdirPermissionDeniedContinuesAfterOperand(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can bypass directory write permissions")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	_, errb, code := runTool(t, dir, filepath.Join("locked", "child"), "ok")
	if code != 1 {
		t.Fatalf("code=%d, want 1; err=%q", code, errb)
	}
	if !strings.Contains(errb, "cannot create directory") {
		t.Fatalf("missing permission diagnostic: %q", errb)
	}
	if fi, err := os.Stat(filepath.Join(dir, "ok")); err != nil || !fi.IsDir() {
		t.Fatalf("later operand not created after permission failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(locked, "child")); err == nil {
		t.Fatalf("child unexpectedly created under non-writable parent")
	}
}

func TestMkdirVirtualUmaskPreservesInheritedSetgid(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "parent")
	if err := os.Mkdir(parent, 0o770); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o2770); err != nil {
		t.Skipf("cannot enable setgid inheritance: %v", err)
	}
	if fi, err := os.Stat(parent); err != nil || fi.Mode()&os.ModeSetgid == 0 {
		t.Skip("filesystem does not retain setgid on directories")
	}

	// First prove this filesystem actually inherits setgid on mkdir; POSIX
	// permits platform variation, so absence is a skip rather than a failure.
	control := filepath.Join(parent, "control")
	if err := os.Mkdir(control, 0o755); err != nil {
		t.Fatal(err)
	}
	cfi, err := os.Stat(control)
	if err != nil {
		t.Fatal(err)
	}
	if cfi.Mode()&os.ModeSetgid == 0 {
		t.Skip("filesystem does not inherit directory setgid")
	}

	// Make the host process umask stricter than the virtual shell mask so
	// mkdir must take the corrective chmod path, where inherited special bits
	// used to be lost.
	old := unix.Umask(0o077)
	defer unix.Umask(old)
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Umask: 0o022, UmaskSet: true}
	_, errb, code := runToolWithContext(t, rc, filepath.Join("parent", "child"))
	unix.Umask(old)
	if code != 0 {
		t.Fatalf("mkdir with virtual umask: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(parent, "child"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSetgid == 0 {
		t.Fatalf("virtual-umask correction cleared inherited setgid: mode=%v", fi.Mode())
	}
}
