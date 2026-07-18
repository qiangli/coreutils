package uniqcmd

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
func runTool(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runToolDir(t, t.TempDir(), stdin, args...)
}

func runToolDir(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
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

func TestUniq(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"adjacent dedup", "a\na\nb\nb\nb\na\n", nil, "a\nb\na\n"},
		{"count format is %7d space", "a\na\nb\n", []string{"-c"}, "      2 a\n      1 b\n"},
		{"repeated only", "a\na\nb\nc\nc\n", []string{"-d"}, "a\nc\n"},
		{"unique only", "a\na\nb\nc\nc\n", []string{"-u"}, "b\n"},
		{"repeated and unique prints nothing", "a\na\nb\n", []string{"-d", "-u"}, ""},
		{"count with repeated", "a\na\nb\n", []string{"-c", "-d"}, "      2 a\n"},
		{"ignore case keeps first of group", "A\na\nb\n", []string{"-i"}, "A\nb\n"},
		{"skip fields", "x a\ny a\nz b\n", []string{"-f", "1"}, "x a\nz b\n"},
		{"skip chars", "1a\n2a\n3b\n", []string{"-s", "1"}, "1a\n3b\n"},
		{"check chars", "ab\nac\nbd\n", []string{"-w", "1"}, "ab\nbd\n"},
		{"check chars zero compares nothing", "a\nb\nc\n", []string{"-w", "0"}, "a\n"},
		{"fields skipped before chars", "x 1same\ny 2same\n", []string{"-f", "1", "-s", "2"}, "x 1same\n"},
		{"no trailing newline", "a\na", nil, "a\n"},
		{"empty input", "", nil, ""},
		{"zero terminated", "a\x00a\x00b\x00", []string{"-z"}, "a\x00b\x00"},
		{"all repeated", "a\na\nb\nc\nc\nc\n", []string{"-D"}, "a\na\nc\nc\nc\n"},
		{"all repeated separate", "a\na\nb\nc\nc\n", []string{"--all-repeated=separate"}, "a\na\n\nc\nc\n"},
		{"all repeated attached prepend", "a\na\nb\nc\nc\n", []string{"-Dprepend"}, "\na\na\n\nc\nc\n"},
		{"group prepend", "a\na\nb\n", []string{"--group=prepend"}, "\na\na\n\nb\n"},
		{"group both", "a\na\nb\n", []string{"--group=both"}, "\na\na\n\n\nb\n\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("%s: uniq %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

func TestUniqOperands(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in.txt"), []byte("a\na\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runToolDir(t, dir, "", "in.txt")
	if code != 0 || out != "a\nb\n" {
		t.Errorf("uniq in.txt = (%q, %d)", out, code)
	}
	// Second operand is the output file, resolved against rc.Dir.
	out, _, code = runToolDir(t, dir, "", "in.txt", "out.txt")
	if code != 0 || out != "" {
		t.Errorf("uniq in.txt out.txt: stdout=%q code=%d", out, code)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil || string(got) != "a\nb\n" {
		t.Errorf("out.txt = (%q, %v)", got, err)
	}
	// "-" output operand means stdout.
	out, _, code = runToolDir(t, dir, "", "in.txt", "-")
	if code != 0 || out != "a\nb\n" {
		t.Errorf("uniq in.txt -: stdout=%q code=%d", out, code)
	}
	_, errb, code := runToolDir(t, dir, "", "nonexistent")
	if code != 1 || !strings.Contains(errb, "nonexistent") {
		t.Errorf("missing input: code=%d err=%q", code, errb)
	}
}

func TestUniqErrors(t *testing.T) {
	_, errb, code := runTool(t, "", "a", "b", "c")
	if code != 2 || !strings.Contains(errb, "extra operand 'c'") {
		t.Errorf("3 operands: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "", "-f", "-1")
	if code != 2 || !strings.Contains(errb, "invalid number of fields to skip") {
		t.Errorf("-f -1: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "", "-w", "-2")
	if code != 2 || !strings.Contains(errb, "invalid number of bytes to compare") {
		t.Errorf("-w -2: code=%d err=%q", code, errb)
	}
	// Unknown flag: contract error, exit 2, names the flag.
	_, errb, code = runTool(t, "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "", "-c", "-D")
	if code != 2 || !strings.Contains(errb, "meaningless") {
		t.Errorf("-c -D: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "", "--group=bad")
	if code != 2 || !strings.Contains(errb, "invalid group method") {
		t.Errorf("--group=bad: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "", "-Dbad")
	if code != 2 || !strings.Contains(errb, "invalid delimit method") {
		t.Errorf("-Dbad: code=%d err=%q", code, errb)
	}
}

func TestUniqHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: uniq") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "--version")
	if code != 0 || !strings.Contains(out, "uniq") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
