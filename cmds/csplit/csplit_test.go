package csplitcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir string, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestCsplitLineNumber(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "", "-s", "in", "2")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errb)
	}
	assertFile(t, dir, "xx00", "a\n")
	assertFile(t, dir, "xx01", "b\nc\n")
}

func TestCsplitRegexAndPrefix(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "one\ntwo\nthree\n", "-f", "part", "-n", "1", "-", "/two/")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if stdout != "4\n10\n" {
		t.Fatalf("stdout=%q", stdout)
	}
	assertFile(t, dir, "part0", "one\n")
	assertFile(t, dir, "part1", "two\nthree\n")
}

func TestCsplitRepeatedRegexAdvances(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "a\nb\nb\nc\n", "-f", "p", "-n", "1", "-", "/b/", "/b/")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if stdout != "2\n2\n4\n" {
		t.Fatalf("stdout=%q", stdout)
	}
	assertFile(t, dir, "p0", "a\n")
	assertFile(t, dir, "p1", "b\n")
	assertFile(t, dir, "p2", "b\nc\n")
}

func TestCsplitErrors(t *testing.T) {
	_, errb, code := runTool(t, t.TempDir(), "", "missing")
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, t.TempDir(), "a\n", "-", "{*}")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, t.TempDir(), "a\n", "-", "/a/+1")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}

func assertFile(t *testing.T, dir, name, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s=%q want %q", name, got, want)
	}
}
