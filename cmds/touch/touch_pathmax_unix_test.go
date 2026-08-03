//go:build unix

// Near-PATH_MAX conformance coverage (POSIX GA67: a pathname of no more
// than PATH_MAX bytes can be resolved). Exercised through the
// native-current-directory mode (tool.RunContext.DirIsProcessCwd) the
// standalone multicall binary runs under, so the scenarios below need
// the process cwd to actually be the invocation directory — hence the
// os.Chdir choreography and the unix-only tag (PathMax is the unix
// limit; Windows' \\?\ namespace makes the joined form effectively
// unbounded there).

package touchcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/qiangli/coreutils/tool"
)

// runNative invokes touch the way the standalone multicall binary does:
// the invocation working directory is the process working directory,
// declared as such via DirIsProcessCwd.
func runNative(t *testing.T, dir string, args ...string) (stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:             context.Background(),
		Dir:             dir,
		DirIsProcessCwd: true,
		FS:              tool.NewLocalFS(),
		Stdio:           tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return errb.String(), code
}

// TestTouchNearPathMaxRelativeOperands covers both halves of the GA67
// defect: a relative operand that is itself nearly PATH_MAX bytes under
// a short working directory, and a short operand under a working
// directory that is itself nearly PATH_MAX bytes. In both, every
// individual piece is valid and the file is reachable by a plain
// relative lookup from the cwd, but naively joining Dir+operand into
// one absolute string overruns PATH_MAX — the string GNU touch never
// builds at all.
func TestTouchNearPathMaxRelativeOperands(t *testing.T) {
	// Resolve the temp dir's own symlinks (macOS /var -> /private/var)
	// up front so the sizes computed below survive kernel expansion.
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Chdir(base); err != nil {
		t.Fatal(err)
	}

	// Grown by relative mkdir + cd at every step, exactly like a shell,
	// so the setup itself never materializes an overlong string either.
	dir := base
	var parts []string
	mkdirChdir := func(name string) {
		t.Helper()
		if err := os.Mkdir(name, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.Chdir(name); err != nil {
			t.Fatalf("chdir: %v", err)
		}
		dir = filepath.Join(dir, name)
		parts = append(parts, name)
	}
	comp := strings.Repeat("x", 40)
	for len(dir)+1+len(comp)+1+len("ff") < unix.PathMax {
		mkdirChdir(comp)
	}
	// Top up so the directory itself lands 2 bytes short of PathMax:
	// valid and reachable on its own, overlong once anything is joined.
	if extra := unix.PathMax - 2 - len(dir) - 1; extra > 0 {
		mkdirChdir(strings.Repeat("x", extra))
	}
	rel := filepath.Join(strings.Join(parts, "/"), "ff")
	joined := filepath.Join(base, rel)
	if len(joined) < unix.PathMax {
		t.Fatalf("setup: joined path len %d does not reach PathMax %d", len(joined), unix.PathMax)
	}
	if len(rel)+1 > unix.PathMax {
		t.Fatalf("setup: relative operand len %d is not itself resolvable under PathMax %d", len(rel), unix.PathMax)
	}

	// Half one (the harness shape): short working directory, relative
	// operand of nearly PATH_MAX bytes.
	if err := os.Chdir(base); err != nil {
		t.Fatal(err)
	}
	if errb, code := runNative(t, base, rel); code != 0 || errb != "" {
		t.Fatalf("touch %d-byte relative operand: code=%d err=%q", len(rel), code, errb)
	}
	if _, err := os.Stat(rel); err != nil {
		t.Fatalf("file created by touch is not there: %v", err)
	}
	// Again on the now-existing file — the update path (utimensat) has
	// to survive the same resolution.
	if errb, code := runNative(t, base, rel); code != 0 || errb != "" {
		t.Fatalf("touch existing %d-byte relative operand: code=%d err=%q", len(rel), code, errb)
	}

	// Half two (the shell cd'd deep): near-PATH_MAX working directory,
	// short relative operand.
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if errb, code := runNative(t, dir, "gg"); code != 0 || errb != "" {
		t.Fatalf("touch short operand in near-PathMax working directory: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat("gg"); err != nil {
		t.Fatalf("file created by touch is not there: %v", err)
	}
}
