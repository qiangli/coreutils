package mkdircmd

// Story #682 (S245.5f): mkdir -p walks its operand upward one component at a
// time. That walk must stay in the SPELLING the caller used. On the
// bash-5.3 Windows runner it did not: filepath.Dir("/tmp/empty/a/a/a")
// returned \tmp\empty\a\a, pathconv's mount lookup is defined on the POSIX
// spelling, so every ancestor after the first was looked for under C:\tmp
// instead of the mounted TEMP — "mkdir: cannot create directory
// '/tmp/empty/a/a/a': The system cannot find the path specified".

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

// TestMakeAllWalksOperandSpelling pins the operand-spelling mode to Windows
// and hands mkdir a natively-spelled operand. On a Unix host a backslash is
// an ordinary filename character, so the three directories mkdir -p creates
// are three flat names — which is precisely what makes the walk observable
// here: with filepath.Dir the operand would have no components at all on
// this host and only the last name would be created.
func TestMakeAllWalksOperandSpelling(t *testing.T) {
	defer tool.SetOperandWindows(true)()

	dir := t.TempDir()
	out, errb, code := runTool(t, dir, "-pv", `a\b\c`)
	if code != 0 || errb != "" {
		t.Fatalf(`mkdir -pv 'a\b\c': code=%d err=%q`, code, errb)
	}
	want := "mkdir: created directory 'a'\n" +
		`mkdir: created directory 'a\b'` + "\n" +
		`mkdir: created directory 'a\b\c'` + "\n"
	if out != want {
		t.Errorf("mkdir -pv output = %q; want %q", out, want)
	}
	for _, name := range []string{`a`, `a\b`, `a\b\c`} {
		if fi, err := os.Stat(filepath.Join(dir, name)); err != nil || !fi.IsDir() {
			t.Errorf("ancestor %q not created: %v", name, err)
		}
	}
}

// The POSIX half of the same rule: every ancestor tool.OperandDir produces
// for /tmp/empty/a/a/a still resolves under the injected /tmp mount. This is
// the operand chain mkdir -p actually walks.
func TestMakeAllAncestorsResolveUnderMount(t *testing.T) {
	defer tool.SetOperandWindows(true)()

	m := pathconv.NewMounts(operandFixtureRoot, nil, operandFixtureTmp)
	cur := "/tmp/empty/a/a/a"
	wants := []string{
		operandFixtureTmp + `\empty\a\a\a`,
		operandFixtureTmp + `\empty\a\a`,
		operandFixtureTmp + `\empty\a`,
		operandFixtureTmp + `\empty`,
		operandFixtureTmp,
	}
	for _, want := range wants {
		if got := tool.ResolveOperandMode(m, "", cur, true); got != want {
			t.Fatalf("ancestor %q resolved to %q; want %q", cur, got, want)
		}
		cur = tool.OperandDir(cur)
	}
	// One more step off /tmp reaches the root mount, where the walk stops.
	if cur != "/" {
		t.Errorf("walk ended at %q; want /", cur)
	}
}
