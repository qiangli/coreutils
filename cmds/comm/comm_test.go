package commcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages.
// Files f1/f2 are created in rc.Dir and passed as relative operands.
func runTool(t *testing.T, f1, f2 string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f1"), []byte(f1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f2"), []byte(f2), 0o644); err != nil {
		t.Fatal(err)
	}
	return runRaw(t, dir, "", append(args, "f1", "f2")...)
}

func runRaw(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
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

func TestComm(t *testing.T) {
	f1 := "a\nb\nc\n"
	f2 := "b\nc\nd\n"
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"three columns", nil, "a\n\t\tb\n\t\tc\n\td\n"},
		{"suppress column 1", []string{"-1"}, "\tb\n\tc\nd\n"},
		{"suppress column 2", []string{"-2"}, "a\n\tb\n\tc\n"},
		{"suppress column 3", []string{"-3"}, "a\n\td\n"},
		{"cluster -12", []string{"-12"}, "b\nc\n"},
		{"separate -1 -3", []string{"-1", "-3"}, "d\n"},
		{"all suppressed", []string{"-123"}, ""},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, f1, f2, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("%s: comm %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

func TestCommStdin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f2"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runRaw(t, dir, "a\nb\n", "-", "f2")
	if code != 0 || out != "a\n\t\tb\n" {
		t.Errorf("comm - f2 = (%q, %d)", out, code)
	}
}

func TestCommOrderCheck(t *testing.T) {
	// Unsorted input with unpairable lines: per-file diagnostic plus the
	// final "input is not in sorted order", exit 1 — but output is still
	// produced.
	out, errb, code := runTool(t, "b\na\nx\n", "a\nx\n")
	if code != 1 {
		t.Errorf("unsorted: code=%d", code)
	}
	if !strings.Contains(errb, "comm: file 1 is not in sorted order") ||
		!strings.Contains(errb, "comm: input is not in sorted order") {
		t.Errorf("unsorted: err=%q", errb)
	}
	if out == "" {
		t.Errorf("unsorted: output should still be produced")
	}
	// Sorted inputs: no diagnostics, exit 0.
	_, errb, code = runTool(t, "a\nb\n", "b\nc\n")
	if code != 0 || errb != "" {
		t.Errorf("sorted: code=%d err=%q", code, errb)
	}
}

func TestCommErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runRaw(t, dir, "")
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no operands: code=%d err=%q", code, errb)
	}
	_, errb, code = runRaw(t, dir, "", "onlyone")
	if code != 2 || !strings.Contains(errb, "missing operand after 'onlyone'") {
		t.Errorf("one operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runRaw(t, dir, "", "a", "b", "c")
	if code != 2 || !strings.Contains(errb, "extra operand 'c'") {
		t.Errorf("three operands: code=%d err=%q", code, errb)
	}
	_, errb, code = runRaw(t, dir, "", "nope1", "nope2")
	if code != 1 || !strings.Contains(errb, "nope1") {
		t.Errorf("missing file: code=%d err=%q", code, errb)
	}
	// Unknown short flag (GNU's -z, unimplemented): contract error.
	_, errb, code = runRaw(t, dir, "", "-z", "a", "b")
	if code != 2 || !strings.Contains(errb, "z") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown short flag: code=%d err=%q", code, errb)
	}
	// Unknown long flag: contract error via the framework.
	_, errb, code = runRaw(t, dir, "", "--total", "a", "b")
	if code != 2 || !strings.Contains(errb, "total") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown long flag: code=%d err=%q", code, errb)
	}
	// After --, -1 is an operand, not a flag.
	if err := os.WriteFile(filepath.Join(dir, "-1"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f2"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runRaw(t, dir, "", "--", "-1", "f2")
	if code != 0 || out != "\t\ta\n" {
		t.Errorf("-- handling: out=%q code=%d", out, code)
	}
}

func TestCommHelpAndVersion(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runRaw(t, dir, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: comm") || !strings.Contains(out, "-1") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runRaw(t, dir, "", "--version")
	if code != 0 || !strings.Contains(out, "comm") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
