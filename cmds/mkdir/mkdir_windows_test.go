//go:build windows

package mkdircmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// TestMkdirModeRecordsOnWindows drives -m end to end through the
// registered Tool: the option is accepted (bash's glob5.sub fixture runs
// `mkdir -m700 ./tmp/a ./tmp/a/b` and diffs on the refusal otherwise),
// each named directory is created, and the FULL computed mode reaches the
// recorder — chmod's recorded-mode transition (Story #686), not a fake
// POSIX mode on the host filesystem.
func TestMkdirModeRecordsOnWindows(t *testing.T) {
	dir := t.TempDir()
	recorded := map[string]os.FileMode{}
	restore := defaultMkdirDeps.record
	defaultMkdirDeps.record = func(path string, mode os.FileMode) error {
		recorded[filepath.Base(path)] = mode
		return nil
	}
	t.Cleanup(func() { defaultMkdirDeps.record = restore })

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{"-m700", "a", filepath.Join("a", "b")})
	if code != 0 {
		t.Fatalf("mkdir -m700 a a\\b: code=%d err=%q", code, errb.String())
	}
	for _, name := range []string{"a", "b"} {
		if got, ok := recorded[name]; !ok {
			t.Errorf("no mode recorded for %s", name)
		} else if got != 0o700 {
			t.Errorf("recorded %o for %s, want 700", got, name)
		}
	}
	if fi, err := os.Stat(filepath.Join(dir, "a", "b")); err != nil || !fi.IsDir() {
		t.Fatalf("directory not created: %v", err)
	}
}

func TestMkdirSymbolicModesForGlobFixtureOnWindows(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		mode string
		name string
		want os.FileMode
	}{
		{mode: "a=x", name: "searchable", want: 0o111},
		{mode: "a=r", name: "readable", want: 0o444},
	} {
		_, errb, code := runTool(t, dir, "-m", tc.mode, tc.name)
		if code != 0 {
			t.Fatalf("mkdir -m %s: code=%d err=%q", tc.mode, code, errb)
		}
		path := filepath.Join(dir, tc.name)
		t.Cleanup(func() {
			_ = tool.RecordMode(path, 0o700)
			_ = os.Chmod(path, 0o666)
		})
		fi, err := tool.Stat(path)
		if err != nil || !fi.IsDir() {
			t.Fatalf("stat %s: %v", tc.name, err)
		}
		if got := fi.Mode().Perm(); got != tc.want {
			t.Errorf("mkdir -m %s recorded %03o, want %03o", tc.mode, got, tc.want)
		}
	}
}

func TestMkdirVirtualUmaskDoesNotApproximatePOSIXModesOnWindows(t *testing.T) {
	dir := t.TempDir()
	var chmodCalled bool
	m := &maker{
		rc: &tool.RunContext{
			Ctx: context.Background(), Dir: dir, Umask: 0o777, UmaskSet: true,
			Stdio: tool.Stdio{Err: &bytes.Buffer{}},
		},
		parents: true,
		deps:    defaultMkdirDeps,
	}
	m.deps.chmod = func(string, os.FileMode) error {
		chmodCalled = true
		return nil
	}
	m.make(filepath.Join("a", "b"))
	if m.failed {
		t.Fatal("mkdir -p failed")
	}
	if chmodCalled {
		t.Fatal("Windows virtual umask must not be approximated through os.Chmod")
	}
	if fi, err := os.Stat(filepath.Join(dir, "a", "b")); err != nil || !fi.IsDir() {
		t.Fatalf("directory not created: %v", err)
	}
}
