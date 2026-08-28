package sedcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

type errorSimplePattern struct{ err error }

func (p errorSimplePattern) FindAllSubmatchIndex([]byte, int) ([][]int, error) {
	return nil, p.err
}
func (errorSimplePattern) Expand([]byte, []byte, []byte, []int) ([]byte, error) {
	panic("Expand called after matcher error")
}

type expansionErrorSimplePattern struct{ err error }

func (expansionErrorSimplePattern) FindAllSubmatchIndex(s []byte, _ int) ([][]int, error) {
	return [][]int{{0, len(s)}}, nil
}
func (p expansionErrorSimplePattern) Expand(dst, _ []byte, _ []byte, _ []int) ([]byte, error) {
	return append(dst, "partial"...), p.err
}

func runSed(t *testing.T, in string, args ...string) (out, errOut string, code int) {
	t.Helper()
	return runSedInDir(t, t.TempDir(), in, args...)
}

func runSedInDir(t *testing.T, dir, in string, args ...string) (out, errOut string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(in), Out: &o, Err: &e},
	}
	code = cmd.Run(rc, args)
	return o.String(), e.String(), code
}

func TestSedBasicSubstitution(t *testing.T) {
	if out, _, _ := runSed(t, "hello\n", "s/l/L/g"); out != "heLLo\n" {
		t.Errorf("s/l/L/g = %q, want heLLo", out)
	}

	t.Run("fast matcher error", func(t *testing.T) {
		wantErr := errors.New("matcher failed")
		subst := &simpleSubstitution{pattern: errorSimplePattern{err: wantErr}, replacement: []byte("x")}
		dst := []byte("guard")
		got, err := applySimpleSubstitutionLine(dst, subst, []byte("subject"))
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if string(got) != "guard" {
			t.Fatalf("destination changed on matcher error: %q", got)
		}

		var out bytes.Buffer
		if err := applySimpleSubstitution(subst, strings.NewReader("subject\n"), &out); !errors.Is(err, wantErr) {
			t.Fatalf("stream error = %v, want %v", err, wantErr)
		}
		if out.Len() != 0 {
			t.Fatalf("stream wrote %q after matcher error", out.String())
		}
	})

	t.Run("fast expansion error", func(t *testing.T) {
		wantErr := errors.New("expansion failed")
		subst := &simpleSubstitution{pattern: expansionErrorSimplePattern{err: wantErr}, replacement: []byte("x")}
		backing := make([]byte, len("guard"), 64)
		copy(backing, "guard")
		got, err := applySimpleSubstitutionLine(backing, subst, []byte("subject"))
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if string(got) != "guard" || string(backing) != "guard" {
			t.Fatalf("destination changed on expansion error: got=%q backing=%q", got, backing)
		}

		var out bytes.Buffer
		if err := applySimpleSubstitution(subst, strings.NewReader("subject\n"), &out); !errors.Is(err, wantErr) {
			t.Fatalf("stream error = %v, want %v", err, wantErr)
		}
		if out.Len() != 0 {
			t.Fatalf("stream wrote %q after expansion error", out.String())
		}
	})
}

func TestSedPreservesMixedExpressionFileOrder(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.sed")
	last := filepath.Join(dir, "last.sed")
	if err := os.WriteFile(first, []byte("s/A/a/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(last, []byte("s/z/Z/p\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errOut, code := runSedInDir(t, dir, "A\nB\n", "-n", "-e", "s/A/a/", "-f", filepath.Base(first), "-e", "s/a/Z/p")
	if code != 0 || errOut != "" || out != "Z\n" {
		t.Errorf("-e/-f/-e order = (%q, %q, %d), want Z", out, errOut, code)
	}
	out, errOut, code = runSedInDir(t, dir, "A\nB\n", "-n", "-f", filepath.Base(first), "-e", "s/a/z/", "-f", filepath.Base(last))
	if code != 0 || errOut != "" || out != "Z\n" {
		t.Errorf("-f/-e/-f order = (%q, %q, %d), want Z", out, errOut, code)
	}
}

func TestSedOptionLookingNamesAfterOperandAreFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "empty"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data string
	}{
		{"-n", "abc\n"},
		{"--", "def\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, tc.name), []byte(tc.data), 0o644); err != nil {
				t.Fatal(err)
			}
			out, errOut, code := runSedInDir(t, dir, "", "1p;q", "empty", tc.name)
			if code != 0 || errOut != "" || out != tc.data+tc.data {
				t.Errorf("file operand %q = (%q, %q, %d), want two prints of %q", tc.name, out, errOut, code, strings.TrimSpace(tc.data))
			}
		})
	}
}

func TestSedQuitLeavesSeekableInputRemainder(t *testing.T) {
	in := strings.NewReader("A\nB\nC\n")
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: in, Out: &out, Err: &errOut},
	}
	if code := cmd.Run(rc, []string{"-n", "/B/q"}); code != 0 || out.Len() != 0 || errOut.Len() != 0 {
		t.Fatalf("sed -n /B/q = (%q, %q, %d), want quiet success", out.String(), errOut.String(), code)
	}
	rest, err := io.ReadAll(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(rest) != "C\n" {
		t.Errorf("input after q = %q, want C\\n", rest)
	}
}

func TestSedPreservesMissingFinalNewline(t *testing.T) {
	if out, errOut, code := runSed(t, "hello", "s/l/L/g"); code != 0 || errOut != "" || out != "heLLo" {
		t.Errorf("s/l/L/g without final newline = (%q, %q, %d), want (%q, empty, 0)", out, errOut, code, "heLLo")
	}
	if out, errOut, code := runSed(t, "hello", "-n", "s/l/L/gp"); code != 0 || errOut != "" || out != "heLLo" {
		t.Errorf("-n s/l/L/gp without final newline = (%q, %q, %d), want (%q, empty, 0)", out, errOut, code, "heLLo")
	}
}

// The headline GNU-compat case: BRE \(...\) groups + \1/\2 backrefs, which the
// upstream (Go/ERE) engine could not do — proves the translation layer.
func TestSedBREGroupsAndBackrefs(t *testing.T) {
	if out, _, _ := runSed(t, "ab\n", `s/\(a\)\(b\)/\2\1/`); out != "ba\n" {
		t.Errorf(`s/\(a\)\(b\)/\2\1/ = %q, want ba`, out)
	}
}

func TestSedBREBackrefConformance(t *testing.T) {
	if out, _, _ := runSed(t, "a^bb\naXbb\n", `s/a^\(b\)\1/Z/`); out != "Z\naXbb\n" {
		t.Errorf(`s/a^\(b\)\1/Z/ = %q, want literal-caret replacement`, out)
	}
	if out, _, _ := runSed(t, "__\n!!\n", `s/\(\w\)\1/Z/`); out != "Z\n!!\n" {
		t.Errorf(`s/\(\w\)\1/Z/ = %q, want GNU word-class escape through backref matcher`, out)
	}
}

func TestSedAmpersandWholeMatch(t *testing.T) {
	if out, _, _ := runSed(t, "abc\n", `s/b/[&]/`); out != "a[b]c\n" {
		t.Errorf("s/b/[&]/ = %q, want a[b]c", out)
	}
	// Escaped & is a literal ampersand.
	if out, _, _ := runSed(t, "abc\n", `s/b/\&/`); out != "a&c\n" {
		t.Errorf(`s/b/\&/ = %q, want a&c`, out)
	}
}

func TestSedBREInterval(t *testing.T) {
	if out, _, _ := runSed(t, "aaaa\n", `s/a\{2\}/X/`); out != "Xaa\n" {
		t.Errorf(`s/a\{2\}/X/ = %q, want Xaa`, out)
	}
}

func TestSedBREIntervalsThroughAdvertisedDupMax(t *testing.T) {
	long := strings.Repeat("a", 255)
	cases := []struct {
		name   string
		input  string
		script string
		want   string
	}{
		{"exact address", long + "\nshort\n", `/^a\{255\}$/p`, long + "\n"},
		{"exact substitution", long + "\n", `s/^a\{255\}$/MATCH/p`, "MATCH\n"},
		{"bounded substitution", long + "\n", `s/^a\{1,255\}$/MATCH/p`, "MATCH\n"},
		{"open interval no match", strings.Repeat("a", 254) + "\n", `/a\{255,\}/p`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runSed(t, tc.input, "-n", tc.script)
			if code != 0 || errOut != "" || out != tc.want {
				t.Errorf("sed -n %q = (%q, %q, %d), want (%q, empty, 0)", tc.script, out, errOut, code, tc.want)
			}
		})
	}
	if out, errOut, code := runSed(t, "a\n", `s/a\{256\}/x/`); code != 2 || out != "" || errOut == "" {
		t.Errorf("above-limit BRE = (%q, %q, %d), want diagnostic exit 2", out, errOut, code)
	}
}

func TestSedBracketBackslashIsLiteral(t *testing.T) {
	out, errOut, code := runSed(t, "abc\na\\c\nn\nq\n", "-n", `/[\a]/p`)
	if code != 0 || errOut != "" || out != "abc\na\\c\n" {
		t.Errorf(`address /[\a]/ = (%q, %q, %d), want lines containing a or backslash`, out, errOut, code)
	}
	out, errOut, code = runSed(t, "abc\n", "-n", `s/[\a]/MATCH/p`)
	if code != 0 || errOut != "" || out != "MATCHbc\n" {
		t.Errorf(`s/[\a]/MATCH/p = (%q, %q, %d), want MATCHbc`, out, errOut, code)
	}
	out, errOut, code = runSed(t, "a\\c\nn\nq\n", "-n", `/[\n]/p`)
	if code != 0 || errOut != "" || out != "a\\c\nn\n" {
		t.Errorf(`address /[\n]/ = (%q, %q, %d), want backslash and n lines`, out, errOut, code)
	}
}

func TestSedEscapedAlphanumericDelimiter(t *testing.T) {
	const substitutions = "sA\\AA\\AZ\\AA\ns@\\@@\\@Z\\@@"
	out, errOut, code := runSed(t, "A\n@\n", substitutions)
	if code != 0 || errOut != "" || out != "AZA\n@Z@\n" {
		t.Errorf("escaped substitution delimiters = (%q, %q, %d), want wrapped literals", out, errOut, code)
	}
	const address = `\xabc\xdefxs/A/z/p`
	out, errOut, code = runSed(t, "abcxdefA\nabcdefA\n", "-n", address)
	if code != 0 || errOut != "" || out != "abcxdefz\n" {
		t.Errorf("escaped address delimiter = (%q, %q, %d), want abcxdefz", out, errOut, code)
	}
}

func TestSedBREWordEdgeAnchors(t *testing.T) {
	if out, _, _ := runSed(t, "sword word words\n", `s/\<word\>/X/g`); out != "sword X words\n" {
		t.Errorf(`s/\<word\>/X/g = %q, want sword X words`, out)
	}
	if out, _, _ := runSed(t, "sword\nword\nwords\n", "-n", `/\<word\>/p`); out != "word\n" {
		t.Errorf(`-n /\<word\>/p = %q, want word`, out)
	}
}

func TestSedEREMode(t *testing.T) {
	if out, _, _ := runSed(t, "aaa\n", "-E", "s/a+/X/"); out != "X\n" {
		t.Errorf("-E s/a+/X/ = %q, want X", out)
	}
	if out, _, _ := runSed(t, "sword word words\n", "-E", `s/\<word\>/X/g`); out != "sword X words\n" {
		t.Errorf(`-E s/\<word\>/X/g = %q, want sword X words`, out)
	}
	if out, _, _ := runSed(t, "aa\nab\n", "-E", `s/(a)\1/X/`); out != "X\nab\n" {
		t.Errorf(`-E s/(a)\1/X/ = %q, want X then ab`, out)
	}
	if out, _, _ := runSed(t, ".\n\\\na\n", "-E", `s/[\.]/X/`); out != "X\nX\na\n" {
		t.Errorf(`-E s/[\.]/X/ = %q, want dot and backslash replaced`, out)
	}
	if out, _, _ := runSed(t, "e\nx\n", "-E", `s/[[=e=]]/X/`); out != "X\nx\n" {
		t.Errorf(`-E s/[[=e=]]/X/ = %q, want e replaced`, out)
	}
}

func TestSedCaseInsensitiveFlag(t *testing.T) {
	if out, _, _ := runSed(t, "Hello\n", "s/hello/hi/I"); out != "hi\n" {
		t.Errorf("s/hello/hi/I = %q, want hi", out)
	}
}

func TestSedDeleteAndRange(t *testing.T) {
	if out, _, _ := runSed(t, "a\nb\nc\n", "2d"); out != "a\nc\n" {
		t.Errorf("2d = %q, want a\\nc", out)
	}
	if out, _, _ := runSed(t, "1\n2\n3\n4\n", "2,3d"); out != "1\n4\n" {
		t.Errorf("2,3d = %q, want 1\\n4", out)
	}
}

func TestSedNumericRangeEndingAtOrBeforeStart(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		input string
		want  string
	}{
		{"equal addresses delete one line", []string{"2,2d"}, "1\n2\n3\n4\n", "1\n3\n4\n"},
		{"descending addresses delete one line", []string{"3,1d"}, "1\n2\n3\n4\n", "1\n2\n4\n"},
		{"equal addresses select one line", []string{"-n", "2,2s/A/z/p"}, "A\nA\nA\n", "z\n"},
		{"descending addresses select one line", []string{"-n", "2,1s/A/z/p"}, "A\nA\nA\n", "z\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runSed(t, tc.input, tc.args...)
			if code != 0 || errOut != "" || out != tc.want {
				t.Errorf("sed %q = (%q, %q, %d), want (%q, empty, 0)",
					tc.args, out, errOut, code, tc.want)
			}
		})
	}
}

func TestSedRelativeRangeAddress(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		script string
		want   string
	}{
		{"numeric start", "1\n2\n3\n4\n5\n", "2,+2p", "2\n3\n4\n"},
		{"zero extra lines", "1\n2\n3\n", "2,+0p", "2\n"},
		{"regexp start", "a\nstart\nb\nc\n", "/start/,+1p", "start\nb\n"},
		{"range can start again", "start\na\ngap\nstart\nb\n", "/start/,+1p", "start\na\nstart\nb\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runSed(t, tc.input, "-n", tc.script)
			if code != 0 || errOut != "" || out != tc.want {
				t.Errorf("sed -n %q = (%q, %q, %d), want (%q, empty, 0)",
					tc.script, out, errOut, code, tc.want)
			}
		})
	}

	out, errOut, code := runSed(t, "1\n", "-n", "1,+p")
	if code != 2 || out != "" || !strings.Contains(errOut, "expected a number after +") {
		t.Errorf("malformed +N address = (%q, %q, %d), want loud parse error", out, errOut, code)
	}
}

func TestSedMultilineTextEscapes(t *testing.T) {
	const appendProgram = `a\
one\ two\\\
three\!`
	out, errOut, code := runSed(t, "base\n", appendProgram)
	if code != 0 || errOut != "" || out != "base\none two\\\nthree!\n" {
		t.Errorf("escaped append text = (%q, %q, %d), want decoded multiline text", out, errOut, code)
	}

	const changeProgram = `c\
first\
second`
	out, errOut, code = runSed(t, "old\n", "-n", changeProgram)
	if code != 0 || errOut != "" || out != "first\nsecond\n" {
		t.Errorf("multiline change text = (%q, %q, %d), want two replacement lines", out, errOut, code)
	}
}

func TestSedCompoundCommandParsing(t *testing.T) {
	// The c command is deliberately unreachable. It still has to parse inside
	// an addressed, inverted brace list.
	const guarded = `1,$!{
c\
changed
}`
	out, errOut, code := runSed(t, "a\nb\n", guarded)
	if code != 0 || errOut != "" || out != "a\nb\n" {
		t.Errorf("guarded compound command = (%q, %q, %d), want unchanged input", out, errOut, code)
	}

	out, errOut, code = runSed(t, "a\nb\nc\n", "-n", `{1d;2s/b/B/;p;}`)
	if code != 0 || errOut != "" || out != "B\nc\n" {
		t.Errorf("semicolon brace list = (%q, %q, %d), want B then c", out, errOut, code)
	}
}

func TestSedScriptHashNDirective(t *testing.T) {
	const program = "#n\n\n\ns/a/A/p\n"
	out, errOut, code := runSed(t, "a\nb\n", program)
	if code != 0 || errOut != "" || out != "A\n" {
		t.Errorf("#n script = (%q, %q, %d), want only explicit print", out, errOut, code)
	}
}

func TestSedAppendNextAtEOFTerminatesScript(t *testing.T) {
	out, errOut, code := runSed(t, "x\n", "N;s/x/X/")
	if code != 0 || errOut != "" || out != "" {
		t.Errorf("N at EOF = (%q, %q, %d), want successful termination without output", out, errOut, code)
	}

	const program = "1,3s/a/b/\nN\ns/b/c/"
	out, errOut, code = runSed(t, "a\nb\nc\nc\nb\n", program)
	if code != 0 || errOut != "" || out != "c\nb\nc\nc\n" {
		t.Errorf("N across input pairs = (%q, %q, %d), want completed pairs only", out, errOut, code)
	}
}

func TestSedReadCommandUsesRunDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "include.txt"), []byte("included\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runSedInDir(t, dir, "1\n2\n", "1r include.txt")
	if code != 0 || errOut != "" || out != "1\nincluded\n2\n" {
		t.Errorf("1r include.txt: out=%q err=%q code=%d", out, errOut, code)
	}
}

func TestSedReadCommandMissingFileIsEmpty(t *testing.T) {
	// GNU sed documents r filename as treating unreadable files as empty,
	// without an error indication.
	out, errOut, code := runSed(t, "1\n2\n", "1r missing.txt")
	if code != 0 || errOut != "" || out != "1\n2\n" {
		t.Errorf("1r missing.txt: out=%q err=%q code=%d", out, errOut, code)
	}
}

func TestSedSubstitutionWriteFlag(t *testing.T) {
	dir := t.TempDir()
	out, errOut, code := runSedInDir(t, dir, "cat cat\nbird\ncat\n", "-n", "s/cat/dog/gw changed.txt")
	if code != 0 || errOut != "" || out != "" {
		t.Fatalf("s///gw = (%q, %q, %d), want quiet success", out, errOut, code)
	}
	got, err := os.ReadFile(filepath.Join(dir, "changed.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "dog dog\ndog\n"; string(got) != want {
		t.Errorf("changed.txt = %q, want %q", got, want)
	}

	dir = t.TempDir()
	out, errOut, code = runSedInDir(t, dir, "bird\n", "-n", "s/cat/dog/gw unchanged.txt")
	if code != 0 || errOut != "" || out != "" {
		t.Fatalf("non-matching s///gw = (%q, %q, %d), want quiet success", out, errOut, code)
	}
	got, err = os.ReadFile(filepath.Join(dir, "unchanged.txt"))
	if err != nil {
		t.Fatalf("non-matching s///gw did not create its wfile: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("unchanged.txt = %q, want empty", got)
	}
}

func TestSedPreparesWriteFilesBeforeProcessing(t *testing.T) {
	dir := t.TempDir()
	out, errOut, code := runSedInDir(t, dir, "input\n", "-n", "q\nw first.txt\nw second.txt")
	if code != 0 || errOut != "" || out != "" {
		t.Fatalf("unreached w commands = (%q, %q, %d), want quiet success", out, errOut, code)
	}
	for _, name := range []string{"first.txt", "second.txt"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s was not created before q: %v", name, err)
		} else if len(got) != 0 {
			t.Errorf("%s = %q, want empty", name, got)
		}
	}
}

func TestSedWriteFileTruncatedOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.txt")
	if err := os.WriteFile(path, []byte("stale contents\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runSedInDir(t, dir, "first\nsecond\n", "-n", "w capture.txt")
	if code != 0 || errOut != "" || out != "" {
		t.Fatalf("w capture.txt = (%q, %q, %d), want quiet success", out, errOut, code)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "first\nsecond\n"; string(got) != want {
		t.Errorf("capture.txt = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o640 {
		t.Errorf("capture.txt mode = %o, want existing mode 640 preserved", gotMode)
	}
}

func TestSedQuietRegexAddressPrint(t *testing.T) {
	if out, _, _ := runSed(t, "a\nbb\nc\n", "-n", "/b/p"); out != "bb\n" {
		t.Errorf("-n /b/p = %q, want bb", out)
	}
}

func TestSedTransliterate(t *testing.T) {
	if out, _, _ := runSed(t, "abc\n", "y/abc/xyz/"); out != "xyz\n" {
		t.Errorf("y/abc/xyz/ = %q, want xyz", out)
	}
}

func TestSedList(t *testing.T) {
	if out, _, code := runSed(t, "a\tb\\c\x01\n", "-n", "l"); code != 0 || out != "a\\tb\\\\c\\001$\n" {
		t.Errorf("-n l = %q, code %d; want escaped listing", out, code)
	}
	if out, _, code := runSed(t, "a\nb\n", "-n", "N;l"); code != 0 || out != "a$\nb$\n" {
		t.Errorf("-n N;l = %q, code %d; want one listing per embedded line", out, code)
	}
}

func TestSedFileArgumentsMayContainBlanks(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input")
	include := filepath.Join(dir, "include file")
	output := filepath.Join(dir, "output file")
	if err := os.WriteFile(input, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(include, []byte("included\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runSedInDir(t, dir, "", "1r include file", input)
	if code != 0 || errOut != "" || out != "one\nincluded\n" {
		t.Errorf("r filename with blanks: out=%q err=%q code=%d", out, errOut, code)
	}
	out, errOut, code = runSedInDir(t, dir, "", "1w output file", input)
	if code != 0 || errOut != "" || out != "one\n" {
		t.Errorf("w filename with blanks: out=%q err=%q code=%d", out, errOut, code)
	}
	if b, err := os.ReadFile(output); err != nil || string(b) != "one\n" {
		t.Errorf("written file = %q, err %v; want one newline", b, err)
	}
}

func TestSedRejectsZeroAddress(t *testing.T) {
	if out, errOut, code := runSed(t, "x\n", "0p"); code == 0 || out != "" || errOut == "" {
		t.Errorf("zero address: out=%q err=%q code=%d; want parse failure", out, errOut, code)
	}
}

func TestSedInPlace(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	os.WriteFile(f, []byte("foo\nbar\n"), 0o644)
	var o, e bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &o, Err: &e}}
	if code := cmd.Run(rc, []string{"-i", "s/o/0/g", f}); code != 0 {
		t.Fatalf("-i exit %d: %s", code, e.String())
	}
	b, _ := os.ReadFile(f)
	if string(b) != "f00\nbar\n" {
		t.Errorf("in-place result = %q, want f00\\nbar", b)
	}
}

func TestSedInPlaceBackup(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	os.WriteFile(f, []byte("x\n"), 0o644)
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}}
	cmd.Run(rc, []string{"-i.bak", "s/x/y/", f})
	if b, _ := os.ReadFile(f); string(b) != "y\n" {
		t.Errorf("edited = %q, want y", b)
	}
	if b, _ := os.ReadFile(f + ".bak"); string(b) != "x\n" {
		t.Errorf("backup = %q, want x (original)", b)
	}
}

func TestSedInPlaceFormsAndPerFileStreams(t *testing.T) {
	cases := []struct {
		name   string
		option string
		backup string
	}{
		{"short", "-i", ""},
		{"short attached suffix", "-i.bak", "f.txt.bak"},
		{"long", "--in-place", ""},
		{"long suffix", "--in-place=.old", "f.txt.old"},
		{"asterisk suffix", "-i*.orig", "f.txt.orig"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("one\ntwo\n"), 0o640); err != nil {
				t.Fatal(err)
			}
			beforeInfo, err := os.Stat(filepath.Join(dir, "f.txt"))
			if err != nil {
				t.Fatal(err)
			}
			out, errOut, code := runSedInDir(t, dir, "", tc.option, "s/one/ONE/", "f.txt")
			if code != 0 || errOut != "" || out != "" {
				t.Fatalf("sed %s = (%q, %q, %d), want no output and exit 0", tc.option, out, errOut, code)
			}
			if got, err := os.ReadFile(filepath.Join(dir, "f.txt")); err != nil || string(got) != "ONE\ntwo\n" {
				t.Errorf("edited file = %q, err %v", got, err)
			}
			if got, err := os.Stat(filepath.Join(dir, "f.txt")); err != nil {
				t.Errorf("stat edited file: %v", err)
			} else if got.Mode().Perm() != beforeInfo.Mode().Perm() {
				t.Errorf("edited mode = %v, want original %v", got.Mode().Perm(), beforeInfo.Mode().Perm())
			}
			if tc.backup != "" {
				if got, err := os.ReadFile(filepath.Join(dir, tc.backup)); err != nil || string(got) != "one\ntwo\n" {
					t.Errorf("backup = %q, err %v; want original contents", got, err)
				}
			}
		})
	}

	dir := t.TempDir()
	for _, name := range []string{"one", "two"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("first\nsecond\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, errOut, code := runSedInDir(t, dir, "", "-ni", "1p", "one", "two")
	if code != 0 || errOut != "" || out != "" {
		t.Fatalf("sed -ni on two files = (%q, %q, %d)", out, errOut, code)
	}
	for _, name := range []string{"one", "two"} {
		if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(got) != "first\n" {
			t.Errorf("%s after -ni 1p = %q, err %v; want first line", name, got, err)
		}
	}
}

func TestSedHelpDocumentsGNUAdditions(t *testing.T) {
	out, errOut, code := runSed(t, "", "--help")
	if code != 0 || errOut != "" || !strings.Contains(out, "addr,+N") ||
		!strings.Contains(out, "-i, --in-place") || strings.ContainsRune(out, '\x00') {
		t.Errorf("sed --help = (%q, %q, %d), want addr,+N and clean -i line", out, errOut, code)
	}
}

func TestSedPatternBackref(t *testing.T) {
	out, errOut, code := runSed(t, "aa\n", `s/\(a\)\1/X/`)
	if code != 0 || errOut != "" || out != "X\n" {
		t.Errorf("sed pattern backref: out=%q err=%q code=%d", out, errOut, code)
	}
}

// POSIX behaviors that sed's regex layer previously got wrong. Each expectation
// is the byte-for-byte output GNU sed produces.
func TestSedPOSIXRegexConformance(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		script string
		want   string
	}{
		// POSIX XCU sed: "the escape sequence '\n' shall match a <newline>
		// embedded in the pattern space". This used to be a hard parse error
		// ("unsupported escape \n"), so no N;s/…\n…/ script could run at all.
		{"backslash-n matches embedded newline", "a\nb\n", `N;s/a\nb/X/`, "X\n"},
		// Inside a bracket expression, backslash is literal: [\n] contains
		// backslash and n, not an embedded newline.
		{"backslash-n in a bracket expression is literal", "a\nb\n", `N;s/a[\n]b/X/`, "a\nb\n"},
		{"backslash-n in the replacement is unchanged", "ab\n", `s/ab/a\nb/`, "a\nb\n"},
		{"backslash-t matches a tab", "a\tb\n", `s/a\tb/X/`, "X\n"},
		// An escaped backslash must not turn the next byte into an escape:
		// \\n is a literal backslash followed by 'n'.
		{"escaped backslash then n is literal", "a\\nb\n", `s/a\\nb/X/`, "X\n"},

		// POSIX: a period matches any character — including the newlines sed's
		// pattern space holds after N.
		{"dot matches embedded newline", "a\nb\n", `N;s/a.b/X/`, "X\n"},
		{"dot-star spans embedded newline", "a\nb\n", `N;s/a.*b/X/`, "X\n"},
		// ...except under the M modifier, where GNU documents the opposite:
		// "the dot character does not match a new-line character in multi-line
		// mode".
		{"M modifier turns dot-all back off", "a\nb\n", `N;s/a.b/X/M`, "a\nb\n"},
		// M also makes ^/$ match at the embedded newlines, which still works.
		{"M anchors match at embedded newlines", "a\nb\n", `N;s/^b$/X/M`, "a\nX\n"},

		// POSIX XBD 9.1: leftmost-longest. The shorter alternative is written
		// first, but the longest match at the leftmost offset is the one
		// substituted.
		{"leftmost-longest alternation", "ab\n", `s/a\|ab/X/`, "X\n"},
		{"leftmost-longest, longer alternative later", "foobar\n", `s/foo\|foobar/X/`, "X\n"},

		// POSIX XBD 9.3.6: a back-reference matches the same string the group
		// matched — the empty string, when the group matched empty. \(a*\)
		// matches empty before "b", so \1 matches empty and the whole pattern
		// matches "b".
		{"backref to a group that matched empty", "b\n", `s/\(a*\)b\1/X/`, "X\n"},
		// And the match reported is the leftmost one: "aba" at offset 1
		// (group 1 = "a"), not the empty-capture match "b" at offset 2.
		{"leftmost match beats a later empty-capture match", "aaba\n", `s/\(a*\)b\1/X/`, "aX\n"},
	}
	for _, c := range cases {
		if out, errOut, code := runSed(t, c.in, c.script); out != c.want || code != 0 {
			t.Errorf("%s: sed %q on %q = (%q, code %d, err %q), want %q",
				c.name, c.script, c.in, out, code, errOut, c.want)
		}
	}
}

func TestSedBSDInPlaceHintOnlyOnFailure(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(f, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := []string{"BASHY_AGENTIC=1", "BASHY_HINTS=on"}
	_, errOut, code := runSedInDirEnv(t, dir, env, "", "-i", "", "-e", "s/x/y/", f)
	const hint = "{\"schema_version\":\"bashy-hint-v1\",\"kind\":\"hint\",\"tool\":\"sed\",\"suggest\":\"GNU sed takes -i with no argument; BSD's `-i ''` is just `-i`\",\"off\":\"BASHY_HINTS=off\"}\n"
	if code == 0 || !strings.HasSuffix(errOut, hint) {
		t.Fatalf("BSD sed failure = (code %d, stderr %q), want exact hint suffix %q", code, errOut, hint)
	}

	_, errOut, code = runSedInDirEnv(t, dir, env, "", "-i", "-e", "s/y/z/", f)
	if code != 0 || errOut != "" {
		t.Fatalf("GNU sed in-place use = (code %d, stderr %q), want success with no hint", code, errOut)
	}

	_, errOut, code = runSedInDirEnv(t, dir, env, "", "-i", "-e", "s/x/y/", "missing")
	if code == 0 || strings.Contains(errOut, "bashy-hint-v1") {
		t.Fatalf("GNU sed in-place failure = (code %d, stderr %q), want failure without BSD hint", code, errOut)
	}
}

func TestSedBSDInPlaceHintRespectsOptOut(t *testing.T) {
	_, errOut, code := runSedInDirEnv(t, t.TempDir(), []string{"BASHY_AGENTIC=1", "BASHY_HINTS=off"}, "", "-i", "", "-e", "s/x/y/", "missing")
	if code == 0 || strings.Contains(errOut, "bashy-hint-v1") {
		t.Fatalf("opted-out BSD sed failure = (code %d, stderr %q), want failure without hint", code, errOut)
	}
}

func runSedInDirEnv(t *testing.T, dir string, env []string, in string, args ...string) (out, errOut string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(in), Out: &o, Err: &e},
	}
	code = cmd.Run(rc, args)
	return o.String(), e.String(), code
}

// ---------------------------------------------------------------------------
// POSIX null RE and \cREc context addresses (VSC residual, 2026-08).
// ---------------------------------------------------------------------------

// A null RE stands for the last RE used, in both places one can appear: the
// s command's pattern and a context address. Before this, `//` compiled as an
// empty pattern, so it matched at every position — s//X/ prefixed every line
// and //d deleted the file.
func TestSedNullRERepeatsLastRE(t *testing.T) {
	cases := []struct {
		name    string
		program string
		in      string
		want    string
	}{
		{"s reuses address RE", `/abc/s//X/`, "abc\nzabcz\nq\n", "X\nzXz\nq\n"},
		// Line 2 substitutes only its first "abc", so the // address still
		// finds one and deletes the line; line 1 has none left.
		{"address reuses s RE", `s/abc/X/;//d`, "abc\nabcabc\nq\n", "X\nq\n"},
		{"s reuses previous s RE", `s/a/1/;s//2/`, "aa\n", "12\n"},
		{"null RE keeps its own flags", `/b/s//X/g`, "bbb\n", "XXX\n"},
		{"alternate delimiter address", `\%abc%s%%Y%`, "abc\nq\n", "Y\nq\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runSed(t, tc.in, tc.program)
			if code != 0 || errOut != "" {
				t.Fatalf("%s = (%q, %q, %d), want success", tc.program, out, errOut, code)
			}
			if out != tc.want {
				t.Errorf("%s = %q, want %q", tc.program, out, tc.want)
			}
		})
	}
}

// "The last RE used" is dynamic: the RE the program most recently APPLIED,
// not the one lexically nearest. The same trailing s//+/ resolves to /a/ on
// line 1 and to /b/ on line 2, because a different s command ran on each.
func TestSedNullREIsTheLastREApplied(t *testing.T) {
	const program = `1s/a/-/;2s/b/-/;s//+/`
	out, errOut, code := runSed(t, "1aab\n2abb\n", program)
	if code != 0 || errOut != "" {
		t.Fatalf("%s = (%q, %q, %d), want success", program, out, errOut, code)
	}
	if want := "1-+b\n2a-+\n"; out != want {
		t.Errorf("%s = %q, want %q", program, out, want)
	}
}

// Evaluating a context address counts as USING its RE even when it does not
// match — so an address that fails still becomes what a later // stands for.
// Here /^2/ is evaluated on line 1, so the trailing s//+/ resolves to ^2 (no
// match) rather than to the /a/ of the s command that did run.
func TestSedNullRECountsUnmatchedAddresses(t *testing.T) {
	const program = `/^1/s/a/-/;/^2/s/b/-/;s//+/`
	out, errOut, code := runSed(t, "1aab\n2abb\n", program)
	if code != 0 || errOut != "" {
		t.Fatalf("%s = (%q, %q, %d), want success", program, out, errOut, code)
	}
	if want := "1-ab\n2a-+\n"; out != want {
		t.Errorf("%s = %q, want %q", program, out, want)
	}
}

// A null RE with no RE before it anywhere is an error, not a match-everything.
// The one-command program also covers the s/// fast path in sed.go, which must
// decline an empty pattern rather than compile it.
func TestSedNullREWithoutPreviousREIsAnError(t *testing.T) {
	for _, program := range []string{`s//X/`, `s//X/g`, `//d`, `//p`} {
		out, errOut, code := runSed(t, "abc\n", program)
		if code == 0 {
			t.Errorf("%s = (%q, %q, %d), want a non-zero exit", program, out, errOut, code)
		}
		if !strings.Contains(errOut, "no previous regular expression") {
			t.Errorf("%s stderr = %q, want it to name the missing previous RE", program, errOut)
		}
	}
}

// Modifiers would change which RE // stands for, so GNU rejects them outright.
func TestSedNullRERejectsModifiers(t *testing.T) {
	const program = `s/a/X/;s//Y/I`
	_, errOut, code := runSed(t, "abc\n", program)
	if code == 0 || !strings.Contains(errOut, "modifiers on empty regexp") {
		t.Errorf("%s stderr = %q (exit %d), want a modifiers-on-empty-regexp error", program, errOut, code)
	}
}

// POSIX \cREc: a context address may be delimited by any character except
// backslash and newline, and an escaped delimiter inside is that character.
func TestSedContextAddressAlternateDelimiter(t *testing.T) {
	cases := []struct {
		name    string
		program string
		in      string
		want    string
	}{
		{"comma delimiter", `\,abc,d`, "abc\nq\n", "q\n"},
		{"escaped delimiter is literal", `\,a\,c,d`, "a,c\nabc\n", "abc\n"},
		{"delimiter frees the slash", `\,a/c,d`, "a/c\nabc\n", "abc\n"},
		{"percent delimiter", `\%abc%d`, "abc\nq\n", "q\n"},
		{"negated", `\,abc,!d`, "abc\nq\n", "abc\n"},
		{"as a range endpoint", `\,a,,\,c,d`, "a\nb\nc\nd\n", "d\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runSed(t, tc.in, tc.program)
			if code != 0 || errOut != "" {
				t.Fatalf("%s = (%q, %q, %d), want success", tc.program, out, errOut, code)
			}
			if out != tc.want {
				t.Errorf("%s = %q, want %q", tc.program, out, tc.want)
			}
		})
	}
}

func TestSedContextAddressRejectsBackslashDelimiter(t *testing.T) {
	_, errOut, code := runSed(t, "abc\n", `\\abc\d`)
	if code == 0 {
		t.Errorf(`\\abc\d = exit %d, want a non-zero exit`, code)
	}
	if !strings.Contains(errOut, "delimit an address") {
		t.Errorf(`\\abc\d stderr = %q, want it to reject the delimiter`, errOut)
	}
}

// The y command's operands are compared for length AFTER escape decoding, and
// a backslash-escaped delimiter is that delimiter as an ordinary character.
func TestSedTransliterateEscapes(t *testing.T) {
	cases := []struct {
		name    string
		program string
		in      string
		want    string
	}{
		{"escaped backslash counts once", `y/abc/x\\z/`, "abc\n", "x\\z\n"},
		{"newline escape", `y/abc/x\nz/`, "abc\n", "x\nz\n"},
		{"alternate delimiter", `y,abc,xyz,`, "abc\n", "xyz\n"},
		{"escaped delimiter in operands", `y,a\,c,x\,z,`, "a,c\n", "x,z\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runSed(t, tc.in, tc.program)
			if code != 0 || errOut != "" {
				t.Fatalf("%s = (%q, %q, %d), want success", tc.program, out, errOut, code)
			}
			if out != tc.want {
				t.Errorf("%s = %q, want %q", tc.program, out, tc.want)
			}
		})
	}
}

// The blanks between an a/i/c command letter and its text on the SAME line are
// a separator; a backslash there is the POSIX escape that makes the next
// character ordinary, which is how leading blanks are kept when they are text.
func TestSedTextCommandLeadingBlanks(t *testing.T) {
	cases := []struct {
		name    string
		program string
		want    string
	}{
		{"blanks are a separator", `1a   hello`, "x\nhello\n"},
		{"backslash keeps the blanks", "1a\\   hello", "x\n   hello\n"},
		{"backslash then text", `1a\hello`, "x\nhello\n"},
		{"classic two-line form", "1a\\\nhello", "x\nhello\n"},
		{"continuation keeps its blanks", "1a\\\n  hello", "x\n  hello\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runSed(t, "x\n", tc.program)
			if code != 0 || errOut != "" {
				t.Fatalf("%q = (%q, %q, %d), want success", tc.program, out, errOut, code)
			}
			if out != tc.want {
				t.Errorf("%q = %q, want %q", tc.program, out, tc.want)
			}
		})
	}
}

func TestSedConcurrentBREAndERE(t *testing.T) {
	const iters = 50
	var wg sync.WaitGroup
	wg.Add(2)

	// Worker 1: BRE mode
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			// Fast path
			out, errOut, code := runSed(t, "a+ aa\n", "s/a+/X/g")
			if code != 0 || errOut != "" || out != "X aa\n" {
				t.Errorf("BRE s/a+/X/g iter %d = (%q, %q, %d), want %q", i, out, errOut, code, "X aa\n")
			}
			// Full engine address
			out, errOut, code = runSed(t, "a+\naa\n", "/a+/d")
			if code != 0 || errOut != "" || out != "aa\n" {
				t.Errorf("BRE /a+/d iter %d = (%q, %q, %d), want %q", i, out, errOut, code, "aa\n")
			}
			// Null RE reuse
			out, errOut, code = runSed(t, "a+\naa\n", "/a+/ { s//X/ }")
			if code != 0 || errOut != "" || out != "X\naa\n" {
				t.Errorf("BRE null-RE iter %d = (%q, %q, %d), want %q", i, out, errOut, code, "X\naa\n")
			}
		}
	}()

	// Worker 2: ERE mode (-E)
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			// Fast path
			out, errOut, code := runSed(t, "a+ aa\n", "-E", "s/a+/X/g")
			if code != 0 || errOut != "" || out != "X+ X\n" {
				t.Errorf("ERE s/a+/X/g iter %d = (%q, %q, %d), want %q", i, out, errOut, code, "X+ X\n")
			}
			// Full engine address
			out, errOut, code = runSed(t, "a+\naa\n", "-E", "/a+/d")
			if code != 0 || errOut != "" || out != "" {
				t.Errorf("ERE /a+/d iter %d = (%q, %q, %d), want empty", i, out, errOut, code)
			}
			// Null RE reuse
			out, errOut, code = runSed(t, "a+\naa\n", "-E", "/a+/ { s//X/ }")
			if code != 0 || errOut != "" || out != "X+\nX\n" {
				t.Errorf("ERE null-RE iter %d = (%q, %q, %d), want %q", i, out, errOut, code, "X+\nX\n")
			}
		}
	}()

	wg.Wait()
}

func TestSedFastPathRegressions(t *testing.T) {
	cases := []struct {
		name string
		args []string
		in   string
		want string
	}{
		{"BRE literal plus", []string{"s/a+/X/g"}, "a+ aa\n", "X aa\n"},
		{"BRE escaped plus", []string{"s/a\\+/X/g"}, "a+ aa\n", "X+ X\n"},
		{"ERE flag -E plus", []string{"-E", "s/a+/X/g"}, "a+ aa\n", "X+ X\n"},
		{"ERE flag -r plus", []string{"-r", "s/a+/X/g"}, "a+ aa\n", "X+ X\n"},
		{"ERE escaped plus is literal", []string{"-E", "s/a\\+/X/g"}, "a+ aa\n", "X aa\n"},
		{"BRE literal parens", []string{"s/(ab)/X/g"}, "(ab) ab\n", "X ab\n"},
		{"BRE capture group", []string{"s/\\(ab\\)/[\\1]/g"}, "(ab) ab\n", "([ab]) [ab]\n"},
		{"ERE capture group", []string{"-E", "s/(ab)/[\\1]/g"}, "(ab) ab\n", "([ab]) [ab]\n"},
		{"Fast path custom delimiter", []string{"-E", "s|a+|X|g"}, "a+ aa\n", "X+ X\n"},
		{"Fast path leading/trailing spaces", []string{"-E", "  s/a+/X/g  "}, "a+ aa\n", "X+ X\n"},
		{"Fast path single replacement", []string{"-E", "s/a+/X/"}, "aa aa\n", "X aa\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runSed(t, tc.in, tc.args...)
			if code != 0 || errOut != "" {
				t.Fatalf("%v = (%q, %q, %d), want success", tc.args, out, errOut, code)
			}
			if out != tc.want {
				t.Errorf("%v = %q, want %q", tc.args, out, tc.want)
			}
		})
	}
}

// TestSedExitCodeSeverityTiers pins the three GNU sed exit-status tiers
// observed against GNU sed 4.10 (public oracle): 0 success, 2 an input DATA
// file could not be opened, 4 a serious processing error once execution was
// under way (a script FILE that can't be opened, or a read failure on an
// already-opened operand such as a directory).
func TestSedExitCodeSeverityTiers(t *testing.T) {
	t.Run("q does not open later operands", func(t *testing.T) {
		dir := t.TempDir()
		first := filepath.Join(dir, "first.txt")
		if err := os.WriteFile(first, []byte("first\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		out, errOut, code := runSedInDir(t, dir, "", "q", filepath.Base(first), "missing.txt")
		if code != 0 || out != "first\n" || errOut != "" {
			t.Fatalf("q later operand = (%q, %q, %d)", out, errOut, code)
		}
	})

	t.Run("missing -f script file exits 4", func(t *testing.T) {
		dir := t.TempDir()
		missing := filepath.Join(dir, "does-not-exist.sed")
		out, errOut, code := runSedInDir(t, dir, "hello\n", "-f", missing)
		if code != 4 || out != "" || errOut == "" {
			t.Fatalf("-f %s = (%q, %q, %d), want (\"\", non-empty, 4)", missing, out, errOut, code)
		}
	})

	t.Run("missing input data file exits 2", func(t *testing.T) {
		dir := t.TempDir()
		missing := filepath.Join(dir, "does-not-exist.txt")
		out, errOut, code := runSedInDir(t, dir, "", "s/x/y/", missing)
		if code != 2 || out != "" || errOut == "" {
			t.Fatalf("s/x/y/ %s = (%q, %q, %d), want (\"\", non-empty, 2)", missing, out, errOut, code)
		}
	})

	t.Run("directory operand exits 4 (continuous stream)", func(t *testing.T) {
		dir := t.TempDir()
		out, errOut, code := runSedInDir(t, dir, "", "s/x/y/", dir)
		if code != 4 || out != "" || errOut == "" {
			t.Fatalf("s/x/y/ %s = (%q, %q, %d), want (\"\", non-empty, 4)", dir, out, errOut, code)
		}
	})

	t.Run("directory operand exits 4 (-s separate)", func(t *testing.T) {
		dir := t.TempDir()
		out, errOut, code := runSedInDir(t, dir, "", "-s", "s/x/y/", dir)
		if code != 4 || out != "" || errOut == "" {
			t.Fatalf("-s s/x/y/ %s = (%q, %q, %d), want (\"\", non-empty, 4)", dir, out, errOut, code)
		}
	})

	t.Run("separate files preserve highest error severity", func(t *testing.T) {
		dir := t.TempDir()
		missing := filepath.Join(dir, "does-not-exist.txt")
		for _, operands := range [][]string{{dir, missing}, {missing, dir}} {
			args := append([]string{"-s", "s/x/y/"}, operands...)
			out, errOut, code := runSedInDir(t, dir, "", args...)
			if code != 4 || out != "" || errOut == "" {
				t.Fatalf("%v = (%q, %q, %d), want (\"\", non-empty, 4)", args, out, errOut, code)
			}
		}
	})

	t.Run("missing operand does not suppress later input", func(t *testing.T) {
		dir := t.TempDir()
		missing := filepath.Join(dir, "missing")
		later := filepath.Join(dir, "later")
		if err := os.WriteFile(later, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		out, errOut, code := runSedInDir(t, dir, "", "s/x/y/", missing, later)
		if code != 2 || out != "y\n" || errOut == "" {
			t.Fatalf("missing then later = (%q, %q, %d), want (\"y\\n\", non-empty, 2)", out, errOut, code)
		}
	})

	t.Run("q advances past an empty operand", func(t *testing.T) {
		dir := t.TempDir()
		empty := filepath.Join(dir, "empty")
		later := filepath.Join(dir, "later")
		if err := os.WriteFile(empty, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(later, []byte("later\nsecond\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		out, errOut, code := runSedInDir(t, dir, "", "q", empty, later)
		if code != 0 || out != "later\n" || errOut != "" {
			t.Fatalf("q empty later = (%q, %q, %d), want (\"later\\n\", \"\", 0)", out, errOut, code)
		}
	})

	t.Run("separate stream stops after serious read failure", func(t *testing.T) {
		dir := t.TempDir()
		missing := filepath.Join(dir, "missing")
		out, errOut, code := runSedInDir(t, dir, "", "-s", "s/x/y/", dir, missing)
		if code != 4 || out != "" || errOut == "" || strings.Contains(errOut, missing) {
			t.Fatalf("-s dir missing = (%q, %q, %d), want serious-error only and status 4", out, errOut, code)
		}
	})
}

func TestSedAddressRegressions(t *testing.T) {
	cases := []struct {
		name string
		args []string
		in   string
		want string
	}{
		{"BRE single address literal", []string{"/a+/d"}, "a+\naa\nb\n", "aa\nb\n"},
		{"BRE single address escaped", []string{"/a\\+/d"}, "a+\naa\nb\n", "b\n"},
		{"ERE single address", []string{"-E", "/a+/d"}, "a+\naa\nb\n", "b\n"},
		{"ERE single address escaped literal", []string{"-E", "/a\\+/d"}, "a+\naa\nb\n", "aa\nb\n"},
		{"BRE range address", []string{"/a+/,/b+/d"}, "top\na+\naa\nb+\nend\n", "top\nend\n"},
		{"ERE range address", []string{"-E", "/a+/,/b+/d"}, "top\na\nx\nb\nend\n", "top\nend\n"},
		{"BRE null-RE after address", []string{"/a+/ { s//X/; }"}, "a+\naa\n", "X\naa\n"},
		{"ERE null-RE after address", []string{"-E", "/a+/ { s//X/; }"}, "a+\naa\n", "X+\nX\n"},
		{"BRE null-RE after substitution", []string{"s/a+/X/; s//Y/"}, "a+ a+\n", "X Y\n"},
		{"ERE null-RE after substitution", []string{"-E", "s/a+/X/; s//Y/"}, "aa aa\n", "X Y\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runSed(t, tc.in, tc.args...)
			if code != 0 || errOut != "" {
				t.Fatalf("%v = (%q, %q, %d), want success", tc.args, out, errOut, code)
			}
			if out != tc.want {
				t.Errorf("%v = %q, want %q", tc.args, out, tc.want)
			}
		})
	}
}
