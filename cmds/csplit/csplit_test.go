package csplitcmd

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

func TestCsplitRepeatToEOF(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "a\ncut\nb\ncut\nc\n", "-s", "-f", "p", "-n", "1", "-", "/cut/", "{*}")
	if code != 0 || stdout != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, stdout, errb)
	}
	assertFile(t, dir, "p0", "a\n")
	assertFile(t, dir, "p1", "cut\nb\n")
	assertFile(t, dir, "p2", "cut\nc\n")
}

func TestCsplitRegexOffsets(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "a\nmatch\nb\nc\n", "-s", "-f", "p", "-n", "1", "-", "/match/+1")
	if code != 0 || stdout != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, stdout, errb)
	}
	assertFile(t, dir, "p0", "a\nmatch\n")
	assertFile(t, dir, "p1", "b\nc\n")

	dir = t.TempDir()
	stdout, errb, code = runTool(t, dir, "a\nskip\nb\nc\n", "-s", "-f", "q", "-n", "1", "-", "%skip%+1")
	if code != 0 || stdout != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, stdout, errb)
	}
	assertFile(t, dir, "q0", "b\nc\n")
}

// GNU csplit accepts an unsigned offset; it is equivalent to +N.
func TestCsplitRegexUnsignedOffset(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "a\nmatch\nb\n", "-s", "-", "/match/0")
	if code != 0 || stdout != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, stdout, errb)
	}
	assertFile(t, dir, "xx00", "a\n")
	assertFile(t, dir, "xx01", "match\nb\n")
}

func TestCsplitSuffixSuppressRepeatAndElideEmpty(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "a\nmatch\nb\nmatch\nc\n", "-q", "-z", "--suppress-matched", "-b", "%03d.out", "-", "/match/", "{1}")
	if code != 0 || stdout != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, stdout, errb)
	}
	assertFile(t, dir, "xx000.out", "a\n")
	assertFile(t, dir, "xx001.out", "b\n")
	assertFile(t, dir, "xx002.out", "c\n")
}

func TestCsplitSuffixIntegerAliases(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "a\nb\n", "-s", "-b", "%03i", "-", "2")
	if code != 0 || stdout != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, stdout, errb)
	}
	assertFile(t, dir, "xx000", "a\n")
	assertFile(t, dir, "xx001", "b\n")
}

func TestCsplitZeroRepeatIsNoop(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "a\nb\nc\n", "-s", "-", "2", "{0}")
	if code != 0 || stdout != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, stdout, errb)
	}
	assertFile(t, dir, "xx00", "a\n")
	assertFile(t, dir, "xx01", "b\nc\n")
}

func TestCsplitElidesEmptyInitialPiece(t *testing.T) {
	dir := t.TempDir()
	stdout, errb, code := runTool(t, dir, "match\nbody\n", "-s", "-z", "-", "/match/")
	if code != 0 || stdout != "" || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, stdout, errb)
	}
	assertFile(t, dir, "xx00", "match\nbody\n")
	if _, err := os.Stat(filepath.Join(dir, "xx01")); !os.IsNotExist(err) {
		t.Fatalf("xx01 exists or stat failed unexpectedly: %v", err)
	}
}

func TestCsplitErrors(t *testing.T) {
	_, errb, code := runTool(t, t.TempDir(), "", "missing")
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, t.TempDir(), "a\n", "-", "{*}")
	if code != 2 || !strings.Contains(errb, "missing pattern before repeat count") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, t.TempDir(), "a\n", "-", "/a/+2")
	if code != 2 || !strings.Contains(errb, "line number out of range") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, t.TempDir(), "a\n", "-b", "%s", "-", "/a/")
	if code != 2 || !strings.Contains(errb, "requires one integer conversion") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, t.TempDir(), "a\n", "-b", "%d-%d", "-", "/a/")
	if code != 2 || !strings.Contains(errb, "requires one integer conversion") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}

// A delimiter escaped inside either regexp form is a literal pattern
// character, not the closing delimiter.
func TestCsplitEscapedDelimiterInRegex(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pattern string
		input   string
		files   map[string]string
	}{
		{name: "slash form", pattern: `/a\/b/`, input: "foo\na/b\nbar\n", files: map[string]string{"xx00": "foo\n", "xx01": "a/b\nbar\n"}},
		{name: "percent form", pattern: `%a\%b%`, input: "foo\na%b\nbar\n", files: map[string]string{"xx00": "a%b\nbar\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "in"), []byte(tc.input), 0o644); err != nil {
				t.Fatal(err)
			}
			_, errb, code := runTool(t, dir, "", "-s", "in", tc.pattern)
			if code != 0 || errb != "" {
				t.Fatalf("code=%d err=%q", code, errb)
			}
			for name, want := range tc.files {
				assertFile(t, dir, name, want)
			}
		})
	}
}

// Spec CONSEQUENCES OF ERRORS: files created before an error are removed
// unless -k (--keep-files) is given. A pre-existing directory at the second
// piece's name makes its write fail after xx00 was already written.
func TestCsplitKeepFilesRetainsOutputOnError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		keep   bool
		exists bool
	}{
		{"default removes created files", false, false},
		{"keep-files retains created files", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "in"), []byte("a\nb\nc\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			// A directory named xx01 forces the second WriteFile to fail
			// after xx00 has been created.
			if err := os.Mkdir(filepath.Join(dir, "xx01"), 0o755); err != nil {
				t.Fatal(err)
			}
			args := []string{"-s", "in", "2"}
			if tc.keep {
				args = append([]string{"-k"}, args...)
			}
			_, errb, code := runTool(t, dir, "", args...)
			if code != 1 || !strings.Contains(errb, "csplit:") {
				t.Fatalf("code=%d err=%q, want exit 1 with diagnostic", code, errb)
			}
			_, err := os.Stat(filepath.Join(dir, "xx00"))
			if tc.exists && err != nil {
				t.Fatalf("xx00 should be retained under -k: %v", err)
			}
			if !tc.exists && !os.IsNotExist(err) {
				t.Fatalf("xx00 should be removed on error: stat err=%v", err)
			}
		})
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

// POSIX: a repeated line-number pattern advances by N lines each round.
func TestCsplitLineNumberRepeatAdvances(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "in", "3", "{1}")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	assertFile(t, dir, "xx00", "l1\nl2\n")
	assertFile(t, dir, "xx01", "l3\nl4\nl5\n")
	assertFile(t, dir, "xx02", "l6\nl7\nl8\n")
}

// GNU csplit treats a line-number {*} that would run past EOF as an error
// and removes all output files (unless --keep-files was requested).
func TestCsplitLineNumberRepeatToEOFCleansUp(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("l1\nl2\nl3\nl4\nl5\nl6\nl7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "in", "2", "{*}")
	if code == 0 || !strings.Contains(errb, "line number out of range") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "xx00")); !os.IsNotExist(err) {
		t.Fatalf("xx00 exists or stat failed unexpectedly: %v", err)
	}
}

// An explicit repeat count that runs past EOF is an error.
func TestCsplitLineNumberRepeatOutOfRange(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("l1\nl2\nl3\nl4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "in", "3", "{5}")
	if code != 2 || !strings.Contains(errb, "line number out of range") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}

// csplit patterns are BREs: \(...\) groups, \{n\} intervals.
func TestCsplitPatternsAreBRE(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("aa\nxbbz\ncc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "in", `/xb\{2\}z/`)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	assertFile(t, dir, "xx00", "aa\n")
	assertFile(t, dir, "xx01", "xbbz\ncc\n")
	// In a BRE, ( is a literal — this must not be a regex syntax error.
	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, "in"), []byte("aa\nx(y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code = runTool(t, dir2, "", "in", "/x(y/")
	if code != 0 {
		t.Fatalf("literal paren: code=%d err=%q", code, errb)
	}
	assertFile(t, dir2, "xx01", "x(y\n")
}

// --suppress-matched also suppresses line-number split lines.
func TestCsplitSuppressMatchedLineNumber(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("l1\nl2\nl3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "--suppress-matched", "in", "2")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	assertFile(t, dir, "xx00", "l1\n")
	assertFile(t, dir, "xx01", "l3\n")
}

type fakeCsplitCType struct{ closeCalls int }

func (p *fakeCsplitCType) classify(b byte, member bool) (bool, error) { return member, nil }
func (p *fakeCsplitCType) IsAlpha(b byte) (bool, error) {
	return p.classify(b, b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b == 0xe9)
}
func (p *fakeCsplitCType) IsAlnum(b byte) (bool, error) {
	alpha, err := p.IsAlpha(b)
	return alpha || b >= '0' && b <= '9', err
}
func (p *fakeCsplitCType) IsBlank(b byte) (bool, error) { return p.classify(b, b == ' ' || b == '\t') }
func (p *fakeCsplitCType) IsCntrl(b byte) (bool, error) {
	return p.classify(b, b < 0x20 || b == 0x7f)
}
func (p *fakeCsplitCType) IsDigit(b byte) (bool, error) { return p.classify(b, b >= '0' && b <= '9') }
func (p *fakeCsplitCType) IsGraph(b byte) (bool, error) { return p.classify(b, b > 0x20 && b != 0x7f) }
func (p *fakeCsplitCType) IsLower(b byte) (bool, error) {
	return p.classify(b, b >= 'a' && b <= 'z' || b == 0xe9)
}
func (p *fakeCsplitCType) IsPrint(b byte) (bool, error) {
	return p.classify(b, b >= 0x20 && b != 0x7f)
}
func (p *fakeCsplitCType) IsPunct(b byte) (bool, error) { return p.classify(b, b == '.' || b == '!') }
func (p *fakeCsplitCType) IsSpace(b byte) (bool, error) {
	return p.classify(b, b == ' ' || b == '\t' || b == '\n')
}
func (p *fakeCsplitCType) IsUpper(b byte) (bool, error) { return p.classify(b, b >= 'A' && b <= 'Z') }
func (p *fakeCsplitCType) IsXDigit(b byte) (bool, error) {
	return p.classify(b, b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F')
}
func (p *fakeCsplitCType) ToLower(in []byte) ([]byte, error) {
	out := append([]byte(nil), in...)
	for i, b := range out {
		if b >= 'A' && b <= 'Z' {
			out[i] = b + 32
		}
	}
	return out, nil
}
func (p *fakeCsplitCType) Close() error { p.closeCalls++; return nil }

type fakeCsplitCollate struct{ closeCalls int }

func (p *fakeCsplitCollate) Equivalents(value byte) ([]byte, error) {
	if value == 0 {
		return nil, nil
	}
	if value == 'e' || value == 0xe9 {
		return []byte{'e', 0xe9}, nil
	}
	return []byte{value}, nil
}
func (p *fakeCsplitCollate) EquivalenceClasses() ([]bool, error) {
	result := make([]bool, 256)
	for i := 1; i < len(result); i++ {
		result[i] = true
	}
	return result, nil
}
func (p *fakeCsplitCollate) CollationWeights() ([]byte, error) {
	result := make([]byte, 256)
	for i := range result {
		result[i] = byte(i)
	}
	result[0xe9] = 'e'
	return result, nil
}
func (p *fakeCsplitCollate) CollatingElements() ([]bool, error) {
	result := make([]bool, 256)
	for i := 1; i < len(result); i++ {
		result[i] = true
	}
	return result, nil
}
func (p *fakeCsplitCollate) Close() error { p.closeCalls++; return nil }

func runCsplitLocale(t *testing.T, env []string, stdin string, args []string, ctypeOpen ctypeOpener, collateOpen collateOpener) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code := runWithLocales(rc, args, ctypeOpen, collateOpen)
	return out.String(), errb.String(), code
}

func TestCsplitLocaleRegexClassesEquivalenceAndRanges(t *testing.T) {
	for _, pattern := range []string{"/[[:alpha:]]/", "/[[=e=]]/", "/[d-f]/"} {
		t.Run(pattern, func(t *testing.T) {
			ctypeFake := &fakeCsplitCType{}
			collateFake := &fakeCsplitCollate{}
			stdout, errb, code := runCsplitLocale(t,
				[]string{"LC_CTYPE=de_DE.iso88591", "LC_COLLATE=de_DE.iso88591"},
				string([]byte{'1', '\n', 0xe9, '\n', 'z', '\n'}),
				[]string{"-s", "-", pattern},
				func(string) (ctypeProvider, error) { return ctypeFake, nil },
				func(string) (collateProvider, error) { return collateFake, nil },
			)
			if code != 0 || stdout != "" || errb != "" || ctypeFake.closeCalls != 1 || collateFake.closeCalls != 1 {
				t.Fatalf("csplit %s = (%q,%q,%d), ctypeClose=%d collateClose=%d", pattern, stdout, errb, code, ctypeFake.closeCalls, collateFake.closeCalls)
			}
		})
	}
}

func TestCsplitLocaleInitFailsBeforeInputOpen(t *testing.T) {
	want := errors.New("ctype unavailable")
	_, errb, code := runCsplitLocale(t,
		[]string{"LC_ALL=de_DE.iso88591"},
		"",
		[]string{"missing-input", "/x/"},
		func(string) (ctypeProvider, error) { return nil, want },
		func(string) (collateProvider, error) { return nil, errors.New("collator should not open") },
	)
	if code != 2 || !strings.Contains(errb, want.Error()) || strings.Contains(errb, "missing-input") {
		t.Fatalf("locale init failure = (%q,%d), want ctype diagnostic before input open", errb, code)
	}
}
