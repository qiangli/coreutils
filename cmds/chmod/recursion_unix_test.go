//go:build unix

package chmodcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// restoreTree makes a hierarchy readable again so a test can inspect and
// t.TempDir can remove it. It walks top-down, because a directory has to
// regain search permission before its own entries can be reached.
func restoreTree(t *testing.T, root string) {
	t.Helper()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		p := filepath.Join(root, e.Name())
		if e.IsDir() {
			restoreTree(t, p)
			continue
		}
		if err := os.Chmod(p, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestChmodRecursiveRemovingPermissionsReachesWholeHierarchy pins the
// Issue 7 -R requirement: "For each file operand that names a directory,
// chmod shall change the file mode bits of the directory and all files in
// the file hierarchy below it."
//
// A mode that removes search permission is the case that makes the order
// of the walk observable. Changing a directory before its entries puts
// those entries out of reach of the very command that was told to change
// them, so the requirement is only met if the walk changes children
// first. This is a real filesystem product: the caller owns every file
// and needs no privilege.
func TestChmodRecursiveRemovingPermissionsReachesWholeHierarchy(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(dir, "d", "sub", "f")
	if err := os.WriteFile(inner, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer restoreTree(t, filepath.Join(dir, "d"))

	out, errb, code := runTool(t, dir, "-R", "000", "d")
	if code != 0 || errb != "" {
		t.Fatalf("chmod -R 000: code=%d err=%q out=%q", code, errb, out)
	}

	// Inspect from the top down, restoring search permission as we go:
	// the modes just set are exactly what prevents a bottom-up look.
	for _, step := range []struct {
		path string
		next string
	}{
		{filepath.Join(dir, "d"), filepath.Join(dir, "d", "sub")},
		{filepath.Join(dir, "d", "sub"), inner},
	} {
		fi, err := os.Lstat(step.path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0 {
			t.Errorf("%s mode=%#o want 0000", step.path, fi.Mode().Perm())
		}
		if err := os.Chmod(step.path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fi, err := os.Lstat(inner)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0 {
		t.Errorf("%s mode=%#o want 0000 — the hierarchy below the operand was not reached", inner, fi.Mode().Perm())
	}
}

// TestChmodRecursiveVisitsChildrenBeforeDirectory is the ordering
// evidence behind the requirement above, read off -v output rather than
// off the resulting modes.
func TestChmodRecursiveVisitsChildrenBeforeDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d", "sub", "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "-R", "-v", "700", "d")
	if code != 0 || errb != "" {
		t.Fatalf("chmod -R -v: code=%d err=%q", code, errb)
	}
	order := map[string]int{}
	for i, line := range strings.Split(strings.TrimSpace(out), "\n") {
		for _, name := range []string{"'d/sub/f'", "'d/sub'", "'d'"} {
			if strings.Contains(line, "of "+name+" ") {
				order[name] = i
				break
			}
		}
	}
	if len(order) != 3 {
		t.Fatalf("expected a report for each entry, got %q", out)
	}
	if !(order["'d/sub/f'"] < order["'d/sub'"] && order["'d/sub'"] < order["'d'"]) {
		t.Errorf("entries must be reported before their directory, got %q", out)
	}
}

// TestChmodRecursiveUnreadableDirectoryIsDiagnosedAndStillChanged covers
// the other side of the walk: a directory whose entries cannot be listed
// is diagnosed and sets a nonzero status, but the directory itself is
// still changed and later operands are still attempted.
func TestChmodRecursiveUnreadableDirectoryIsDiagnosedAndStillChanged(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a 0000 directory, so the failure cannot be produced")
	}
	dir := t.TempDir()
	closed := filepath.Join(dir, "d", "closed")
	if err := os.MkdirAll(closed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(closed, "hidden"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other")
	if err := os.WriteFile(other, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(closed, 0o000); err != nil {
		t.Fatal(err)
	}
	defer restoreTree(t, filepath.Join(dir, "d"))

	_, errb, code := runTool(t, dir, "-R", "700", "d", "other")
	if code != 1 {
		t.Fatalf("code=%d want 1, err=%q", code, errb)
	}
	if !strings.Contains(errb, "cannot read directory 'd/closed'") {
		t.Errorf("expected an unreadable-directory diagnostic, got %q", errb)
	}
	fi, err := os.Lstat(closed)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("unreadable directory mode=%#o want 0700 — it must still be changed", fi.Mode().Perm())
	}
	// The later operand is independent of the earlier failure.
	if fi, err = os.Lstat(other); err != nil {
		t.Fatal(err)
	} else if fi.Mode().Perm() != 0o700 {
		t.Errorf("later operand mode=%#o want 0700", fi.Mode().Perm())
	}
}

// TestChmodRecursiveSymlinkLoopTerminates pins that a -L walk over a
// hierarchy containing a link back to an ancestor stops rather than
// descending forever, and still changes the real entries.
func TestChmodRecursiveSymlinkLoopTerminates(t *testing.T) {
	dir := t.TempDir()
	d := filepath.Join(dir, "d")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("..", filepath.Join(d, "up")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	_, errb, code := runTool(t, dir, "-R", "-L", "u+x", "d")
	if code != 0 {
		t.Fatalf("chmod -R -L over a loop: code=%d err=%q", code, errb)
	}
	fi, err := os.Lstat(filepath.Join(d, "f"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("f mode=%#o want the owner execute bit set", fi.Mode().Perm())
	}
}
