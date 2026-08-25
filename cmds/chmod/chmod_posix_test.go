package chmodcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runToolEnv is runTool with an invocation environment (POSIX mode is
// keyed on the presence of POSIXLY_CORRECT in rc.Env).
func runToolEnv(t *testing.T, dir string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

// TestModeApplyXUsesOriginalUnmodifiedMode pins the Issue 7 X clause:
// X represents execute "if the current (unmodified) file mode bits have
// at least one of the execute bits set" — the mode before this chmod
// invocation, not the in-progress value of earlier clauses.
func TestModeApplyXUsesOriginalUnmodifiedMode(t *testing.T) {
	cases := []struct {
		mode  string
		old   uint32
		isDir bool
		want  uint32
	}{
		// Originally executable: a later clause removing x must not
		// stop X from applying.
		{"a-x,a+X", 0o755, false, 0o755},
		// Originally non-executable: an earlier clause adding x must
		// not make X apply.
		{"u+x,a+X", 0o644, false, 0o744},
		// Directories always take X.
		{"a-x,a+X", 0o755, true, 0o755},
		{"-X", 0o755, false, 0o644},
	}
	for _, c := range cases {
		mc, err := parseMode(c.mode)
		if err != nil {
			t.Fatalf("parseMode(%q): %v", c.mode, err)
		}
		if got := mc.apply(c.old, c.isDir, 0); got != c.want {
			t.Errorf("%q on %04o (dir=%v) = %04o, want %04o", c.mode, c.old, c.isDir, got, c.want)
		}
	}
}

// TestModeApplyBareOpAndOWithS pins two Issue 7 clauses: an op with no
// perm makes no change ('+'/'-') or clears the who classes ('='), and
// "who symbol o ... with the perm symbol s" neither modifies the set-id
// bits nor is an error.
func TestModeApplyBareOpAndOWithS(t *testing.T) {
	cases := []struct {
		mode  string
		old   uint32
		umask uint32
		want  uint32
	}{
		{"u+", 0o644, 0o22, 0o644},
		{"a-", 0o644, 0o22, 0o644},
		// '=' with no perm and no who clears all of the file mode bits.
		{"=", 0o7755, 0o22, 0},
		{"u=", 0o4755, 0, 0o055},
		// o with s: set-id bits shall not be modified, and it is not
		// an error.
		{"o+s", 0o755, 0, 0o755},
		{"o-s", 0o6755, 0, 0o6755},
		{"o=s", 0o755, 0, 0o750},
	}
	for _, c := range cases {
		mc, err := parseMode(c.mode)
		if err != nil {
			t.Fatalf("parseMode(%q): %v", c.mode, err)
		}
		if got := mc.apply(c.old, false, c.umask); got != c.want {
			t.Errorf("%q on %04o (umask %03o) = %04o, want %04o", c.mode, c.old, c.umask, got, c.want)
		}
	}
}

// TestModeApplyPOSIXOctalAbsolute pins the Issue 7 rule that an octal
// mode operand sets the file mode bits absolutely: in POSIX mode the
// GNU keep-directory-setid rule must not preserve a directory's
// setuid/setgid bits.
func TestModeApplyPOSIXOctalAbsolute(t *testing.T) {
	mc, err := parseMode("755")
	if err != nil {
		t.Fatal(err)
	}
	if got := mc.apply(0o2775, true, 0); got != 0o2755 {
		t.Errorf("non-POSIX default: 755 on dir 02775 = %04o, want 02755", got)
	}
	mc.absolute = true
	if got := mc.apply(0o2775, true, 0); got != 0o755 {
		t.Errorf("POSIX absolute: 755 on dir 02775 = %04o, want 0755", got)
	}
}

// makeSetgidDir creates a directory carrying the setgid bit, skipping
// the test when the platform or filesystem refuses to set it.
func makeSetgidDir(t *testing.T, parent string) string {
	t.Helper()
	dir := filepath.Join(parent, "sgdir")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o775|os.ModeSetgid); err != nil {
		t.Skipf("cannot set setgid on directory: %v", err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSetgid == 0 {
		t.Skip("filesystem dropped the directory setgid bit")
	}
	return dir
}

// TestChmodPOSIXModeOctalClearsDirectorySetID drives the POSIX-mode
// gate end to end: with POSIXLY_CORRECT present, "chmod 755 dir" sets
// the mode absolutely and clears setgid; without it, the GNU
// keep-directory-setid default is retained.
func TestChmodPOSIXModeOctalClearsDirectorySetID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	parent := t.TempDir()
	dir := makeSetgidDir(t, parent)

	if _, errb, code := runToolEnv(t, parent, nil, "755", "sgdir"); code != 0 {
		t.Fatalf("chmod 755 (default): code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSetgid == 0 {
		t.Fatalf("non-POSIX default cleared directory setgid: mode=%v", fi.Mode())
	}

	if _, errb, code := runToolEnv(t, parent, []string{"POSIXLY_CORRECT="}, "755", "sgdir"); code != 0 {
		t.Fatalf("chmod 755 (POSIX mode): code=%d err=%q", code, errb)
	}
	fi, err = os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSetgid != 0 {
		t.Fatalf("POSIX mode kept directory setgid: mode=%v", fi.Mode())
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("POSIX mode perm=%#o, want 0755", fi.Mode().Perm())
	}
}

// TestChmodReferenceCopiesExactModeToDirectory pins the --reference
// extension to its GNU-documented meaning "use RFILE's mode": the
// short-octal keep-directory-setid rule must not leak the target
// directory's setgid bit into the copied mode.
func TestChmodReferenceCopiesExactModeToDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	parent := t.TempDir()
	dir := makeSetgidDir(t, parent)
	if err := os.WriteFile(filepath.Join(parent, "ref"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, errb, code := runToolEnv(t, parent, nil, "--reference=ref", "sgdir"); code != 0 {
		t.Fatalf("chmod --reference: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSetgid != 0 {
		t.Fatalf("--reference kept target setgid not present on RFILE: mode=%v", fi.Mode())
	}
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("--reference perm=%#o, want 0644", fi.Mode().Perm())
	}
}

// TestChmodContinuesAfterOperandError pins per-operand independence: a
// failing file operand is diagnosed, the remaining operands are still
// processed, and the exit status is >0.
func TestChmodContinuesAfterOperandError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is unix-only")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runToolEnv(t, dir, nil, "600", "no-such-file", "f")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d (out=%q err=%q)", code, out, errb)
	}
	if !strings.Contains(errb, "cannot access 'no-such-file'") {
		t.Errorf("missing diagnostic for failed operand: %q", errb)
	}
	fi, err := os.Stat(f)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("later operand not processed: mode=%#o want 0600", fi.Mode().Perm())
	}
}
