package commcmd

import (
	"bytes"
	"context"
	"errors"
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

// POSIX STDOUT: comm writes three columns — column 1 (lines only in file1),
// column 2 (only in file2, prefixed by one <tab>), column 3 (in both, prefixed
// by two <tab>s). Each -1/-2/-3 suppresses its column AND removes the leading
// <tab> that every printed column to its right carries, so the prefix count of
// a printed column equals the number of lower-numbered columns still printed.
func TestCommColumnTabPrefixesPerPOSIX(t *testing.T) {
	f1 := "a\nb\nc\n" // a only in f1; b,c common
	f2 := "b\nc\nd\n" // d only in f2
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"all three", nil, "a\n\t\tb\n\t\tc\n\td\n"},
		{"-1 shifts survivors left", []string{"-1"}, "\tb\n\tc\nd\n"},
		{"-2 keeps col1 flush, col3 one tab", []string{"-2"}, "a\n\tb\n\tc\n"},
		{"-3 keeps col1 flush, col2 one tab", []string{"-3"}, "a\n\td\n"},
		{"-13 leaves only col2 flush", []string{"-13"}, "d\n"},
		{"-23 leaves only col1 flush", []string{"-23"}, "a\n"},
		{"-12 leaves only col3 flush", []string{"-12"}, "b\nc\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, f1, f2, c.args...)
		if code != 0 || errb != "" || out != c.want {
			t.Errorf("%s: comm %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

// POSIX OPERANDS/EXIT STATUS: comm requires exactly file1 and file2, and a
// failure to open an operand is diagnosed with a status greater than zero.
func TestCommOperandCountAndOpenFailure(t *testing.T) {
	dir := t.TempDir()
	if _, errb, code := runRaw(t, dir, "", "only"); code != 2 || !strings.Contains(errb, "missing operand after 'only'") {
		t.Errorf("one operand: code=%d err=%q", code, errb)
	}
	if err := os.WriteFile(filepath.Join(dir, "f2"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runRaw(t, dir, "", "nosuch", "f2")
	if code == 0 || out != "" || !strings.Contains(errb, "nosuch") {
		t.Errorf("open failure: out=%q err=%q code=%d, want >0 naming the operand", out, errb, code)
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

	if err := os.WriteFile(filepath.Join(dir, "f1"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runRaw(t, dir, "a\nb\n", "f1", "-")
	if code != 0 || errb != "" || out != "\t\ta\n\tb\n" {
		t.Errorf("comm f1 - = (%q, %q, %d)", out, errb, code)
	}

	out, errb, code = runRaw(t, dir, "a\n", "-", "-")
	if code != 2 || out != "" || !strings.Contains(errb, "both files cannot be standard input") {
		t.Errorf("comm - - = (%q, %q, %d)", out, errb, code)
	}
}

func TestCommDuplicateAndEmptyLines(t *testing.T) {
	out, errb, code := runTool(t, "\na\na\nc\n", "\na\nb\nc\nc\n")
	want := "\t\t\n\t\ta\na\n\tb\n\t\tc\n\tc\n"
	if code != 0 || errb != "" || out != want {
		t.Errorf("comm duplicate lines = (%q, %q, %d), want (%q, %q, 0)", out, errb, code, want, "")
	}
}

type gatedReader struct {
	first *strings.Reader
	rest  *strings.Reader
	allow func() bool
}

func (r *gatedReader) Read(p []byte) (int, error) {
	if r.first.Len() > 0 {
		return r.first.Read(p)
	}
	if !r.allow() {
		return 0, errors.New("read requested before earlier output")
	}
	return r.rest.Read(p)
}

func TestCommStreamsBeforeEOF(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f2"), []byte("z\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	first := strings.Repeat("a", 5000) + "\n"
	var out, errb bytes.Buffer
	in := &gatedReader{
		first: strings.NewReader(first),
		rest:  strings.NewReader("y\n"),
		allow: func() bool { return out.Len() > 0 },
	}
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: in, Out: &out, Err: &errb},
	}

	code := cmd.Run(rc, []string{"-", "f2"})
	want := first + "y\n\tz\n"
	if code != 0 || errb.String() != "" || out.String() != want {
		t.Errorf("streaming comm = (%q, %q, %d), want (%q, %q, 0)", out.String(), errb.String(), code, want, "")
	}
}

func TestCommFinalRecordWithoutDelimiter(t *testing.T) {
	out, errb, code := runTool(t, "a\nb", "b\nc")
	want := "a\n\t\tb\n\tc\n"
	if code != 0 || errb != "" || out != want {
		t.Errorf("unterminated inputs = (%q, %q, %d), want (%q, %q, 0)", out, errb, code, want, "")
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
	// Unknown short flag: contract error.
	_, errb, code = runRaw(t, dir, "", "-x", "a", "b")
	if code != 2 || !strings.Contains(errb, "x") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown short flag: code=%d err=%q", code, errb)
	}
	// Unknown long flag: contract error via the framework.
	_, errb, code = runRaw(t, dir, "", "--frobnicate", "a", "b")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
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

func TestCommNewOptions(t *testing.T) {
	// 1. Output Delimiter
	out, _, code := runTool(t, "a\nb\n", "b\nc\n", "--output-delimiter=,")
	if code != 0 || out != "a\n,,b\n,c\n" {
		t.Errorf("output delimiter: out=%q code=%d", out, code)
	}

	// 2. Total
	out, _, code = runTool(t, "a\nb\n", "b\nc\n", "--total")
	wantTotal := "a\n\t\tb\n\tc\n1\t1\t1\t3 total\n"
	if code != 0 || out != wantTotal {
		t.Errorf("total: out=%q code=%d", out, code)
	}

	// Total with suppressions
	out, _, code = runTool(t, "a\nb\n", "b\nc\n", "--total", "-1")
	wantTotalSuppress := "\tb\nc\n0\t1\t1\t2 total\n"
	if code != 0 || out != wantTotalSuppress {
		t.Errorf("total with -1: out=%q code=%d", out, code)
	}

	// 3. Zero Terminated
	out, _, code = runTool(t, "a\x00b\x00", "b\x00c\x00", "-z")
	if code != 0 || out != "a\x00\t\tb\x00\tc\x00" {
		t.Errorf("zero terminated: out=%q code=%d", out, code)
	}

	// 4. Check Order
	// Without --check-order: all pairable, so no diagnostic
	_, errb, code := runTool(t, "b\na\n", "b\na\n")
	if code != 0 || errb != "" {
		t.Errorf("check order default: code=%d err=%q", code, errb)
	}

	// With --check-order: fails immediately on first out-of-order
	_, errb, code = runTool(t, "b\na\n", "b\na\n", "--check-order")
	if code != 1 || !strings.Contains(errb, "is not in sorted order") {
		t.Errorf("check order: code=%d err=%q", code, errb)
	}

	// With --nocheck-order: even with unpairable lines, no error
	_, errb, code = runTool(t, "b\na\nx\n", "a\nx\n", "--nocheck-order")
	if code != 0 || errb != "" {
		t.Errorf("nocheck order: code=%d err=%q", code, errb)
	}
}

// failingWriter fails every Write, so a buffered emit surfaces the error
// only at the final Flush — the path POSIX pins as exit >0 on output failure.
type failingWriter struct{ err error }

func (w failingWriter) Write(p []byte) (int, error) { return 0, w.err }

func TestCommStandardOutputWriteFailure(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"f1", "f2"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("a\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: failingWriter{err: errors.New("device full")}, Err: &errb},
	}
	code := cmd.Run(rc, []string{"f1", "f2"})
	if code != 1 || !strings.Contains(errb.String(), "comm: write failed") || !strings.Contains(errb.String(), "device full") {
		t.Fatalf("write failure = (%q, %d), want exit 1 with write-failed diagnostic", errb.String(), code)
	}
}

// errReader yields a non-EOF error immediately, exercising the record
// reader's error path (distinct from a clean EOF, which ends input).
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

func TestCommInputReadErrorIsDiagnosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f2"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: errReader{err: errors.New("input error")}, Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{"-", "f2"})
	if code != 1 || !strings.Contains(errb.String(), "comm: -:") || !strings.Contains(strings.ToLower(errb.String()), "input error") {
		t.Fatalf("read error = (%q, %q, %d), want exit 1 naming operand '-'", out.String(), errb.String(), code)
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

type fakeCollator struct {
	compare  func(string, string) (int, error)
	closed   bool
	closeErr error
}

func (f *fakeCollator) Compare(a, b string) (int, error) { return f.compare(a, b) }
func (f *fakeCollator) Close() error                     { f.closed = true; return f.closeErr }

func TestCommUsesInvocationCollatorForMergeAndOrderChecks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f1"), []byte("b\na\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f2"), []byte("b\na\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	f := &fakeCollator{compare: func(a, b string) (int, error) {
		return -strings.Compare(a, b), nil
	}}
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir,
		Env:   []string{"LC_COLLATE=de_DE.iso88591"},
		Stdio: tool.Stdio{Out: &out, Err: &errb},
	}
	code := runWithCollator(rc, []string{"--check-order", "f1", "f2"}, func(name string) (stringCollator, error) {
		if name != "de_DE.iso88591" {
			t.Fatalf("opened locale %q", name)
		}
		return f, nil
	})
	if code != 0 || out.String() != "\t\tb\n\t\ta\n" || errb.Len() != 0 || !f.closed {
		t.Fatalf("locale comm = (%q, %q, %d, closed=%v)", out.String(), errb.String(), code, f.closed)
	}
}

func TestCommLocaleInitFailsBeforeInputOpen(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Env:   []string{"LC_COLLATE=unsupported"},
		Stdio: tool.Stdio{Out: &out, Err: &errb},
	}
	wantErr := errors.New("provider unavailable")
	code := runWithCollator(rc, []string{"missing-one", "missing-two"}, func(string) (stringCollator, error) {
		return nil, wantErr
	})
	if code != 2 || out.Len() != 0 || !strings.Contains(errb.String(), wantErr.Error()) || strings.Contains(errb.String(), "missing-one") {
		t.Fatalf("init failure = (%q, %q, %d)", out.String(), errb.String(), code)
	}
}

func TestCommComparisonFailureIsDiagnosedAndCloses(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"f1", "f2"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("a\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var out, errb bytes.Buffer
	f := &fakeCollator{compare: func(string, string) (int, error) {
		return 0, errors.New("compare broke")
	}}
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"LANG=de_DE.ISO-8859-1"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := runWithCollator(rc, []string{"f1", "f2"}, func(string) (stringCollator, error) { return f, nil })
	if code != 1 || out.Len() != 0 || !strings.Contains(errb.String(), "compare broke") || !f.closed {
		t.Fatalf("compare failure = (%q, %q, %d, closed=%v)", out.String(), errb.String(), code, f.closed)
	}
}

func TestCommCloseFailureChangesSuccessfulStatus(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"f1", "f2"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("a\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var out, errb bytes.Buffer
	closeErr := errors.New("close broke")
	f := &fakeCollator{
		compare:  func(a, b string) (int, error) { return strings.Compare(a, b), nil },
		closeErr: closeErr,
	}
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"LC_COLLATE=de_DE.iso88591"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := runWithCollator(rc, []string{"f1", "f2"}, func(string) (stringCollator, error) { return f, nil })
	if code != 1 || out.String() != "\t\ta\n" || !strings.Contains(errb.String(), closeErr.Error()) || !f.closed {
		t.Fatalf("close failure = (%q, %q, %d, closed=%v)", out.String(), errb.String(), code, f.closed)
	}
}

func TestCommCAndPOSIXBypassCollator(t *testing.T) {
	for _, name := range []string{"C", "POSIX"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for _, file := range []string{"f1", "f2"} {
				if err := os.WriteFile(filepath.Join(dir, file), []byte("a\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var out, errb bytes.Buffer
			opened := false
			rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"LC_ALL=" + name}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
			code := runWithCollator(rc, []string{"f1", "f2"}, func(string) (stringCollator, error) {
				opened = true
				return nil, errors.New("must not open")
			})
			if code != 0 || out.String() != "\t\ta\n" || errb.Len() != 0 || opened {
				t.Fatalf("C path = (%q, %q, %d, opened=%v)", out.String(), errb.String(), code, opened)
			}
		})
	}
}
