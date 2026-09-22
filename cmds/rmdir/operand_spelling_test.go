package rmdircmd

// Story #682 (S245.5f): rmdir resolves its operand through
// RunContext.RawPath, and -p walks it upward one component at a time. Both
// must see the operand in the SPELLING the caller used. On the bash-5.3
// Windows runner rmdir rewrote it with filepath.FromSlash first, so
// /tmp/bash-globignore-6244 became \tmp\bash-globignore-6244 — pathconv's
// mount lookup is defined on the POSIX spelling, so it fell through to the
// drive-relative rule, landed on C:\tmp and failed with "The system cannot
// find the file specified" (extglob.tests).

import (
	"os"
	"path/filepath"
	"testing"

	"mvdan.cc/sh/v3/pathconv"

	"github.com/qiangli/coreutils/tool"
)

const (
	operandFixtureRoot = `C:\T\bash53-1\root`
	operandFixtureTmp  = `C:\T\bash53-1\tmp`
)

// TestOperandResolvesUnderMount is the extglob.tests failure stated
// directly: the operand rmdir hands to the path layer is the one the caller
// typed, so on Windows it lands in the mounted TEMP.
func TestOperandResolvesUnderMount(t *testing.T) {
	m := pathconv.NewMounts(operandFixtureRoot, nil, operandFixtureTmp)
	const op = "/tmp/bash-globignore-6244"
	got := tool.ResolveOperandMode(m, "", op, true)
	if want := operandFixtureTmp + `\bash-globignore-6244`; got != want {
		t.Errorf("resolved %q = %q; want %q", op, got, want)
	}
	// The spelling rmdir used to build with filepath.FromSlash leaves the
	// mount table entirely — the negative half of the regression.
	if got := tool.ResolveOperandMode(m, "", `\tmp\bash-globignore-6244`, true); got != `C:\tmp\bash-globignore-6244` {
		t.Errorf("native-spelled operand = %q; want the drive-relative fallback", got)
	}
}

// TestParentsWalksOperandSpelling pins the operand-spelling mode to Windows
// and hands rmdir a natively-spelled operand. On a Unix host a backslash is
// an ordinary filename character, so the ancestors are flat names — which is
// what makes the -p walk observable here.
func TestParentsWalksOperandSpelling(t *testing.T) {
	defer tool.SetOperandWindows(true)()

	dir := t.TempDir()
	for _, name := range []string{`a`, `a\b`, `a\b\c`} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	out, errb, code := runTool(t, dir, "-pv", `a\b\c`)
	if code != 0 || errb != "" {
		t.Fatalf(`rmdir -pv 'a\b\c': code=%d err=%q`, code, errb)
	}
	want := `rmdir: removing directory, 'a\b\c'` + "\n" +
		`rmdir: removing directory, 'a\b'` + "\n" +
		"rmdir: removing directory, 'a'\n"
	if out != want {
		t.Errorf("rmdir -pv output = %q; want %q", out, want)
	}
	for _, name := range []string{`a`, `a\b`, `a\b\c`} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("ancestor %q survived: %v", name, err)
		}
	}
}
