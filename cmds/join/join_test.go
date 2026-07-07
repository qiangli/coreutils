package joincmd

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

func TestJoin(t *testing.T) {
	f1 := "1 a\n2 b\n3 c\n"
	f2 := "1 x\n3 y\n4 z\n"
	cases := []struct {
		name   string
		f1, f2 string
		args   []string
		want   string
	}{
		{"default join on field 1", f1, f2, nil, "1 a x\n3 c y\n"},
		{"-a 1 adds unpairable from file 1", f1, f2, []string{"-a", "1"}, "1 a x\n2 b\n3 c y\n"},
		{"-a 2 adds unpairable from file 2", f1, f2, []string{"-a", "2"}, "1 a x\n3 c y\n4 z\n"},
		{"-a 1 -a 2", f1, f2, []string{"-a", "1", "-a", "2"}, "1 a x\n2 b\n3 c y\n4 z\n"},
		{"-v 1 only unpairable from file 1", f1, f2, []string{"-v", "1"}, "2 b\n"},
		{"-v 2 only unpairable from file 2", f1, f2, []string{"-v", "2"}, "4 z\n"},
		{"-v 1 -v 2", f1, f2, []string{"-v", "1", "-v", "2"}, "2 b\n4 z\n"},
		{"attached value -a1", f1, f2, []string{"-a1"}, "1 a x\n2 b\n3 c y\n"},
		{"join field selection -1 -2", "a 1\nb 2\n", "1 x\n2 y\n", []string{"-1", "2", "-2", "1"}, "1 a x\n2 b y\n"},
		{"join field selection -j", "a 1\nb 2\n", "x 1\ny 2\n", []string{"-j", "2"}, "1 a x\n2 b y\n"},
		{"-t separator", "1:a\n2:b\n", "1:x\n3:y\n", []string{"-t", ":"}, "1:a:x\n"},
		{"-t separator empty fields significant", "1::z\n", "1:x\n", []string{"-t", ":"}, "1::z:x\n"},
		{"-i case-insensitive", "A 1\n", "a 2\n", []string{"-i"}, "A 1 2\n"},
		{"--ignore-case long form", "A 1\n", "a 2\n", []string{"--ignore-case"}, "A 1 2\n"},
		{"cartesian product of equal keys", "k a\nk b\n", "k 1\nk 2\n", nil, "k a 1\nk a 2\nk b 1\nk b 2\n"},
		{"default split collapses blanks", "1   a\n", "1  x\n", nil, "1 a x\n"},
		{"leading blanks ignored by default", "  1 a\n", "1 x\n", nil, "1 a x\n"},
		{"missing join field is empty key", "1 a\n", "1 x\n", []string{"-1", "5"}, ""},
		{"out-of-order but fully pairable is fine", "2 b\n1 a\n", "2 x\n1 y\n", nil, "2 b x\n1 a y\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.f1, c.f2, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("%s: join %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

func TestJoinStdin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f2"), []byte("1 x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runRaw(t, dir, "1 a\n", "-", "f2")
	if code != 0 || out != "1 a x\n" {
		t.Errorf("join - f2 = (%q, %d)", out, code)
	}
	_, errb, code := runRaw(t, dir, "", "-", "-")
	if code != 2 || !strings.Contains(errb, "both files cannot be standard input") {
		t.Errorf("join - -: code=%d err=%q", code, errb)
	}
}

func TestJoinOrderCheck(t *testing.T) {
	// Disorder read after unpairable lines have been seen: per-line
	// diagnostic, final fatal message, exit 1 (GNU's default gate).
	_, errb, code := runTool(t, "1 a\n2 b\n5 e\n4 d\n", "1 x\n9 y\n")
	if code != 1 {
		t.Errorf("unsorted: code=%d err=%q", code, errb)
	}
	if !strings.Contains(errb, "join: input is not in sorted order") {
		t.Errorf("unsorted: err=%q", errb)
	}
	// The per-line diagnostic names FILE:LINENO and the offending line.
	if !strings.Contains(errb, "f1:4: is not sorted: 4 d") {
		t.Errorf("unsorted diagnostic shape: err=%q", errb)
	}
	// A disorder read before any unpairable line is seen is not
	// diagnosed (it cannot affect the output) — GNU default behavior.
	_, errb, code = runTool(t, "2 b\n1 a\n", "2 x\n1 y\n")
	if code != 0 || errb != "" {
		t.Errorf("pairable disorder: code=%d err=%q", code, errb)
	}
	// Sorted inputs with unpairable lines: no diagnostics.
	_, errb, code = runTool(t, "1 a\n2 b\n", "1 x\n3 y\n")
	if code != 0 || errb != "" {
		t.Errorf("sorted: code=%d err=%q", code, errb)
	}
}

func TestJoinErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runRaw(t, dir, "")
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no operands: code=%d err=%q", code, errb)
	}
	_, errb, code = runRaw(t, dir, "", "one")
	if code != 2 || !strings.Contains(errb, "missing operand after 'one'") {
		t.Errorf("one operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runRaw(t, dir, "", "a", "b", "c")
	if code != 2 || !strings.Contains(errb, "extra operand 'c'") {
		t.Errorf("three operands: code=%d err=%q", code, errb)
	}
	_, errb, code = runRaw(t, dir, "", "-1", "0", "a", "b")
	if code != 2 || !strings.Contains(errb, "invalid field number: '0'") {
		t.Errorf("-1 0: code=%d err=%q", code, errb)
	}
	_, errb, code = runRaw(t, dir, "", "-1", "x", "a", "b")
	if code != 2 || !strings.Contains(errb, "invalid field number: 'x'") {
		t.Errorf("-1 x: code=%d err=%q", code, errb)
	}
	_, errb, code = runRaw(t, dir, "", "-a", "3", "a", "b")
	if code != 2 || !strings.Contains(errb, "invalid file number: '3'") {
		t.Errorf("-a 3: code=%d err=%q", code, errb)
	}
	_, errb, code = runRaw(t, dir, "", "-t", "xy", "a", "b")
	if code != 2 || !strings.Contains(errb, "multi-character tab 'xy'") {
		t.Errorf("-t xy: code=%d err=%q", code, errb)
	}
	_, errb, code = runRaw(t, dir, "", "-t")
	if code != 2 || !strings.Contains(errb, "option requires an argument") {
		t.Errorf("-t no value: code=%d err=%q", code, errb)
	}
	_, errb, code = runRaw(t, dir, "", "nope1", "nope2")
	if code != 1 || !strings.Contains(errb, "nope1") {
		t.Errorf("missing file: code=%d err=%q", code, errb)
	}
	// Unknown short flag: contract error.
	_, errb, code = runRaw(t, dir, "", "-x", "a", "b")
	if code != 2 || !strings.Contains(errb, "x") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("-x: code=%d err=%q", code, errb)
	}
	// Unknown long flag: contract error.
	_, errb, code = runRaw(t, dir, "", "--frobnicate", "a", "b")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("--frobnicate: code=%d err=%q", code, errb)
	}
}

func TestJoinNewOptions(t *testing.T) {
	// 1. Output Format (-o)
	out, _, code := runTool(t, "1 a b\n2 c d\n", "1 x y\n", "-o", "0,1.2,2.2")
	if code != 0 || out != "1 a x\n" {
		t.Errorf("-o formatting: out=%q code=%d", out, code)
	}

	// 2. Empty filler (-e)
	out, _, code = runTool(t, "1 a\n", "1 x y\n", "-o", "1.1,1.2,2.2,2.4", "-e", "MISSING")
	if code != 0 || out != "1 a x MISSING\n" {
		t.Errorf("-e formatting: out=%q code=%d", out, code)
	}

	// 3. Header (--header)
	out, _, code = runTool(t, "ID Name\n1 Alice\n", "ID Score\n1 95\n", "--header")
	if code != 0 || out != "ID Name Score\n1 Alice 95\n" {
		t.Errorf("--header: out=%q code=%d", out, code)
	}

	// 4. Zero Terminated (-z)
	out, _, code = runTool(t, "1 a\x002 b\x00", "1 x\x00", "-z")
	if code != 0 || out != "1 a x\x00" {
		t.Errorf("-z option: out=%q code=%d", out, code)
	}

	// 5. Check Order (--check-order)
	// With check-order, fail immediately on disorder even without unpairable lines
	_, errb, code := runTool(t, "2 b\n1 a\n", "2 x\n1 y\n", "--check-order")
	if code != 1 || !strings.Contains(errb, "is not sorted") {
		t.Errorf("--check-order failed: code=%d err=%q", code, errb)
	}

	// 6. Nocheck Order (--nocheck-order)
	_, errb, code = runTool(t, "1 a\n2 b\n5 e\n4 d\n", "1 x\n9 y\n", "--nocheck-order")
	if code != 0 || errb != "" {
		t.Errorf("--nocheck-order failed: code=%d err=%q", code, errb)
	}
}

func TestJoinHelpAndVersion(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runRaw(t, dir, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: join") || !strings.Contains(out, "-a FILENUM") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	for _, want := range []string{"-e EMPTY", "-j FIELD", "-o FORMAT", "-h, --help", "-V, --version"} {
		if !strings.Contains(out, want) {
			t.Errorf("--help missing %q in %q", want, out)
		}
	}
	out, _, code = runRaw(t, dir, "", "--version")
	if code != 0 || !strings.Contains(out, "join") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
	out, _, code = runRaw(t, dir, "", "-h")
	if code != 0 || !strings.Contains(out, "Usage: join") {
		t.Errorf("-h: code=%d out=%q", code, out)
	}
	out, _, code = runRaw(t, dir, "", "-V")
	if code != 0 || !strings.Contains(out, "join") {
		t.Errorf("-V: code=%d out=%q", code, out)
	}
}
