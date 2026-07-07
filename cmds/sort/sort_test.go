package sortcmd

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

func TestSortBasic(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"plain byte order", "b\nA\na\n", nil, "A\na\nb\n"},
		{"reverse", "a\nc\nb\n", []string{"-r"}, "c\nb\na\n"},
		{"numeric", "10\n9\n-3\n2.5\n", []string{"-n"}, "-3\n2.5\n9\n10\n"},
		{"numeric non-numbers are zero", "5\nabc\n-1\n", []string{"-n"}, "-1\nabc\n5\n"},
		{"numeric big integers beyond float precision", "9007199254740993\n9007199254740992\n", []string{"-n"}, "9007199254740992\n9007199254740993\n"},
		{"human numeric", "1G\n2K\n500\n1023M\n", []string{"-h"}, "500\n2K\n1023M\n1G\n"},
		{"human numeric negative", "1K\n-2G\n3\n", []string{"-h"}, "-2G\n3\n1K\n"},
		// Equal-under-fold lines are ordered by the last-resort byte compare.
		{"fold case", "b\nA\nB\na\n", []string{"-f"}, "A\na\nB\nb\n"},
		{"ignore leading blanks", " b\na\n", []string{"-b"}, "a\n b\n"},
		{"unique", "b\na\nb\na\n", []string{"-u"}, "a\nb\n"},
		{"unique with fold keeps first", "A\na\n", []string{"-uf"}, "A\n"},
		{"combined short flags", "2\n10\n1\n", []string{"-rn"}, "10\n2\n1\n"},
		{"no trailing newline still a line", "b\na", nil, "a\nb\n"},
		{"empty input", "", nil, ""},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("%s: sort %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

func TestSortKeys(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"key field 2 to end", "b 1\na 2\n", []string{"-k2"}, "b 1\na 2\n"},
		{"key field includes leading blanks (manual example)", "x  z\ny b\n", []string{"-k2,2"}, "x  z\ny b\n"},
		// b on POS1 skips the field's leading blanks; on POS2 with no .C
		// it has no effect (GNU limfield skips the echar block entirely).
		{"key field with b skips blanks", "x  z\ny b\n", []string{"-k2b,2"}, "y b\nx  z\n"},
		{"b on POS2 without char offset is a no-op", "x  z\ny b\n", []string{"-k2,2b"}, "x  z\ny b\n"},
		{"numeric key", "x 10\ny 9\n", []string{"-k2,2n"}, "y 9\nx 10\n"},
		{"per-key reverse", "x a\ny b\n", []string{"-k2,2r"}, "y b\nx a\n"},
		{"two keys", "a 2 x\na 1 y\nb 1 z\n", []string{"-k1,1", "-k2,2n"}, "a 1 y\na 2 x\nb 1 z\n"},
		{"char offsets", "bca\nabc\n", []string{"-k1.2,1.3"}, "abc\nbca\n"},
		{"char offsets reorder", "xb\nya\n", []string{"-k1.2,1.2"}, "ya\nxb\n"},
		{"separator key", "a:zz\nb:aa\n", []string{"-t:", "-k2,2"}, "b:aa\na:zz\n"},
		{"separator first field", "b:aa\na:zz\n", []string{"-t:", "-k1,1"}, "a:zz\nb:aa\n"},
		{"separator range retains separators", "a:b:c\n", []string{"-t:", "-k2"}, "a:b:c\n"},
		{"empty fields with separator", ":2\n:1\n", []string{"-t:", "-k2,2n"}, ":1\n:2\n"},
		{"key inherits global numeric", "x 10\ny 9\n", []string{"-n", "-k2,2"}, "y 9\nx 10\n"},
		// A key with its own type letters does not inherit the global -r
		// (GNU inheritance rule), so the key still sorts ascending here.
		{"key with own opts ignores global", "y 10\nx 9\n", []string{"-r", "-k2,2n"}, "x 9\ny 10\n"},
		{"missing field sorts empty first", "b\na 1\n", []string{"-k2,2"}, "b\na 1\n"},
		{"global reverse applies to last resort", "a 1\nb 1\n", []string{"-r", "-k2,2n"}, "b 1\na 1\n"},
		{"stable disables last resort", "b 1\na 1\n", []string{"-s", "-k2,2n"}, "b 1\na 1\n"},
		{"last resort orders equal keys", "b 1\na 1\n", []string{"-k2,2n"}, "a 1\nb 1\n"},
		{"unique by key keeps first", "b 1\na 1\n", []string{"-u", "-k2,2n"}, "b 1\n"},
		{"tab separator via -t", "a\t10\nb\t2\n", []string{"-t", "\t", "-k2,2n"}, "b\t2\na\t10\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.stdin, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("%s: sort %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

func TestSortCheck(t *testing.T) {
	if _, _, code := runTool(t, "a\nb\n", "-c"); code != 0 {
		t.Errorf("-c on sorted input: code=%d, want 0", code)
	}
	_, errb, code := runTool(t, "b\na\n", "-c")
	if code != 1 || !strings.Contains(errb, "-:2: disorder: a") {
		t.Errorf("-c on unsorted input: code=%d err=%q", code, errb)
	}
	// -cu: equal lines are disorder.
	_, errb, code = runTool(t, "a\na\n", "-c", "-u")
	if code != 1 || !strings.Contains(errb, "disorder") {
		t.Errorf("-cu on duplicate input: code=%d err=%q", code, errb)
	}
	// File-name diagnostics use the operand as given.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("b\na\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code = runToolDir(t, dir, "", "-c", "f.txt")
	if code != 1 || !strings.Contains(errb, "f.txt:2: disorder: a") {
		t.Errorf("-c file: code=%d err=%q", code, errb)
	}
	// -c allows at most one operand.
	_, errb, code = runTool(t, "", "-c", "a", "b")
	if code != 2 || !strings.Contains(errb, "extra operand 'b' not allowed with -c") {
		t.Errorf("-c two operands: code=%d err=%q", code, errb)
	}
	// -c and -o are incompatible.
	_, errb, code = runTool(t, "", "-c", "-o", "out")
	if code != 2 || !strings.Contains(errb, "'-co' are incompatible") {
		t.Errorf("-co: code=%d err=%q", code, errb)
	}
}

func TestSortFilesAndOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "one"), []byte("c\na\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "two"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runToolDir(t, dir, "", "one", "two")
	if code != 0 || out != "a\nb\nc\n" {
		t.Errorf("sort one two = (%q, %d)", out, code)
	}
	// -o writes the file (resolved against rc.Dir) and nothing to stdout.
	out, _, code = runToolDir(t, dir, "", "-o", "result", "one", "two")
	if code != 0 || out != "" {
		t.Errorf("sort -o: out=%q code=%d", out, code)
	}
	got, err := os.ReadFile(filepath.Join(dir, "result"))
	if err != nil || string(got) != "a\nb\nc\n" {
		t.Errorf("sort -o result content = (%q, %v)", got, err)
	}
	// Sorting a file onto itself works (input is fully read first).
	_, _, code = runToolDir(t, dir, "", "-o", "one", "one")
	if code != 0 {
		t.Errorf("sort -o one one: code=%d", code)
	}
	got, _ = os.ReadFile(filepath.Join(dir, "one"))
	if string(got) != "a\nc\n" {
		t.Errorf("in-place sort = %q", got)
	}
	// Missing file is serious trouble: exit 2.
	_, errb, code := runToolDir(t, dir, "", "nonexistent")
	if code != 2 || !strings.Contains(errb, "cannot read: nonexistent") {
		t.Errorf("missing file: code=%d err=%q", code, errb)
	}
}

func TestSortErrors(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-k", "0"}, "field number is zero: invalid field specification '0'"},
		{[]string{"-k", "x"}, "invalid number at field start: invalid field specification 'x'"},
		{[]string{"-k", "1."}, "invalid number after '.': invalid field specification '1.'"},
		{[]string{"-k", "1.0"}, "character offset is zero: invalid field specification '1.0'"},
		{[]string{"-k", "1,"}, "invalid number after ',': invalid field specification '1,'"},
		{[]string{"-k", "1,0"}, "field number is zero: invalid field specification '1,0'"},
		{[]string{"-k", "1!"}, "stray character in field spec: invalid field specification '1!'"},
		{[]string{"-t", "xy"}, "multi-character tab 'xy'"},
		{[]string{"-t", ""}, "empty tab"},
		{[]string{"-n", "-h"}, "options '-hn' are incompatible"},
		{[]string{"-k", "1nh"}, "options '-hn' are incompatible"},
	}
	for _, c := range cases {
		_, errb, code := runTool(t, "", c.args...)
		if code != 2 || !strings.Contains(errb, c.want) {
			t.Errorf("sort %v: code=%d err=%q, want err containing %q", c.args, code, errb, c.want)
		}
	}
	// Month sort (-k 1M) is now implemented.
	out, _, code := runTool(t, "MAR\nJAN\nFEB\n", "-k", "1M")
	if code != 0 || out != "JAN\nFEB\nMAR\n" {
		t.Errorf("-k 1M: code=%d out=%q, want sorted months", code, out)
	}
	// Unknown flag: contract error, exit 2, names the flag.
	_, errb, code := runTool(t, "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
	// -z is now --zero-terminated, a valid flag.
	out, _, code = runTool(t, "a\000b\000c\000", "-z")
	if code != 0 {
		t.Errorf("-z zero-terminated: code=%d err=%q", code, out)
	}
}

func TestSortHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: sort") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "--version")
	if code != 0 || !strings.Contains(out, "sort") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestSortNewFlags(t *testing.T) {
	// --check-silent / -C returns 1 on disorder but no output
	_, errs, code := runTool(t, "b\na\n", "-C")
	if code != 1 || strings.Contains(errs, "disorder") {
		t.Errorf("-C: code=%d err=%q, want code=1 and silent", code, errs)
	}

	// --zero-terminated / -z
	out, _, code := runTool(t, "c\x00a\x00b\x00", "-z")
	if code != 0 || out != "a\x00b\x00c\x00" {
		t.Errorf("-z: got=%q code=%d", out, code)
	}

	// --dictionary-order / -d
	out, _, code = runTool(t, "a-b\nab\n", "-d")
	if code != 0 || out != "a-b\nab\n" {
		t.Errorf("-d: got=%q", out)
	}

	// --month-sort / -M
	out, _, code = runTool(t, "OCT\nJAN\nMAR\n", "-M")
	if code != 0 || out != "JAN\nMAR\nOCT\n" {
		t.Errorf("-M: got=%q", out)
	}

	// --version-sort / -V
	out, _, code = runTool(t, "a10\na2\n", "-V")
	if code != 0 || out != "a2\na10\n" {
		t.Errorf("-V: got=%q", out)
	}

	// --general-numeric-sort / -g
	out, _, code = runTool(t, "10\n2.5\n1e10\n", "-g")
	if code != 0 || out != "2.5\n10\n1e10\n" {
		t.Errorf("-g: got=%q", out)
	}

	// --ignore-nonprinting / -i
	out, _, code = runTool(t, "a\x01b\nab\n", "-i")
	if code != 0 || out != "a\x01b\nab\n" {
		t.Errorf("-i: got=%q", out)
	}

	// --merge / -m: passes input through (already sorted)
	out, _, code = runTool(t, "a\nb\nc\n", "-m")
	if code != 0 || out != "a\nb\nc\n" {
		t.Errorf("-m: got=%q", out)
	}

	// --random-sort / -R produces same lines (just in different order)
	out, _, code = runTool(t, "a\nb\nc\n", "-R")
	if code != 0 {
		t.Errorf("-R: code=%d", code)
	}
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") || !strings.Contains(out, "c") {
		t.Errorf("-R lost lines: %q", out)
	}

	// --temporary-directory / -T is accepted (no-op)
	_, _, code = runTool(t, "a\nb\n", "-T", "/tmp")
	if code != 0 {
		t.Errorf("-T: code=%d", code)
	}

	out, _, code = runTool(t, "10\n2\n", "--sort=numeric", "-S", "1M", "--batch-size=2", "--compress-program=gzip", "--parallel=4")
	if code != 0 || out != "2\n10\n" {
		t.Errorf("resource aliases with --sort=numeric: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "a10\na2\n", "--sort=version")
	if code != 0 || out != "a2\na10\n" {
		t.Errorf("--sort=version: code=%d out=%q", code, out)
	}
	_, errb, code := runTool(t, "a\n", "--sort=bogus")
	if code != 2 || !strings.Contains(errb, "invalid --sort argument") {
		t.Errorf("--sort=bogus: code=%d err=%q", code, errb)
	}

	// --files0-from
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("b\na\nc\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "list"), []byte("f.txt\x00"), 0644); err != nil {
		t.Fatal(err)
	}
	out, _, code = runToolDir(t, dir, "", "--files0-from", "list")
	if code != 0 || out != "a\nb\nc\n" {
		t.Errorf("--files0-from: got=%q code=%d", out, code)
	}
}
