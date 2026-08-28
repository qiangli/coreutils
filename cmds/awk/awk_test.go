package awkcmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, input string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestAwk(t *testing.T) {
	cases := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{"field", "a b c\n", []string{`{print $2}`}, "b\n"},
		{"sum", "1\n2\n3\n", []string{`{s+=$1} END{print s}`}, "6\n"},
		{"separator", "x:y\n", []string{"-F", ":", `{print $1}`}, "x\n"},
		{"empty-separator", "abc\n", []string{"-F", "", `{print NF, $1, $2, $3}`}, "3 a b c\n"},
		{"var", "", []string{"-v", "n=5", `BEGIN{print n*2}`}, "10\n"},
		{"record", "one\ntwo\nthree\n", []string{`NR==2`}, "two\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, errb, code := runTool(t, c.input, c.args...)
			if out != c.want || errb != "" || code != 0 {
				t.Errorf("awk %v = (%q, %q, %d), want (%q, %q, 0)", c.args, out, errb, code, c.want, "")
			}
		})
	}
}

func TestAwkAbsentArrayReferenceCreatesElement(t *testing.T) {
	program := `BEGIN { print ("missing" in a); x = a["missing"]; print ("missing" in a), x }`
	out, errb, code := runTool(t, "", program)
	if out != "0\n1 \n" || errb != "" || code != 0 {
		t.Fatalf("awk absent array reference = (%q, %q, %d), want (%q, %q, 0)", out, errb, code, "0\n1 \n", "")
	}
}

// POSIX defers awk string-to-number conversion to ISO C atof, which accepts
// the special spellings inf/infinity/nan with an optional sign. The
// conversions and the lowercase string forms of the resulting values are
// pinned here, along with overflow converting to infinity rather than
// failing and NaN comparing unequal to itself.
func TestAwkInfinityAndNaNSemantics(t *testing.T) {
	out, errb, code := runTool(t, "inf nan +inf -inf infinity\n", `{ print $1+0, $2+0, $3+0, $4+0, $5+0 }`)
	if out != "inf nan inf -inf inf\n" || errb != "" || code != 0 {
		t.Fatalf("awk special-value conversions = (%q, %q, %d), want (%q, %q, 0)", out, errb, code, "inf nan inf -inf inf\n", "")
	}
	program := `BEGIN {
		print 1e400, -1e400
		n = "nan" + 0
		print n, (n == n)
	}`
	out, errb, code = runTool(t, "", program)
	want := "inf -inf\nnan 0\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("awk overflow and NaN identity = (%q, %q, %d), want (%q, %q, 0)", out, errb, code, want, "")
	}
}

func TestAwkDivisionByZeroIsADiagnosedError(t *testing.T) {
	out, errb, code := runTool(t, "", `BEGIN { print 1/0 }`)
	if code == 0 || !strings.Contains(errb, "division by zero") || out != "" {
		t.Fatalf("awk division by zero = (%q, %q, %d), want empty stdout, a diagnostic, and a nonzero status", out, errb, code)
	}
}

func TestAwkPOSIXFloatFormats(t *testing.T) {
	program := `BEGIN {
		printf "<%a><%A><%a>\n", 0, 0.125, -0.125
		printf "<%#.0a><%.2A>\n", 0.125, 0.125
		printf "<%015.6a><%+015.6a><% 015.6a><%-15.6a>\n", 0.125, 0.125, 0.125, 0.125
		printf "<%F><%.2F><%#.0F><%010F><%+F><% F>\n", 0.125, 0.125, 1, 0.125, 0.125, 0.125
		print sprintf("<%*.*a><%*.*F><%*.*a>", 12, 3, 0.125, 8, 2, -0.125, -12, -1, 0.125)
		printf "<%.*a><%.*F><%.*f>\n", -0.5, 0.1, -0.5, 0.125, -0.5, 0.125
		printf "<%g><%G><%#g><%.*g>\n", 4.323232245, 0.00004323232245, 4.3, -1, 4.323232245
		print sprintf("<%12g><%.*g><%.3G><%.0g>", 4.323232245, 3, 4.323232245, 4.323232245, 4.323232245)
		OFMT = "%g"; print 4.323232245
		OFMT = "%G"; print 0.00004323232245
		CONVFMT = "%g"; print 4.323232245 ""
		CONVFMT = "%G"; print 0.00004323232245 ""
	}`
	want := "<0x0p+0><0X1P-3><-0x1p-3>\n" +
		"<0x1.p-3><0X1.00P-3>\n" +
		"<0x001.000000p-3><+0x01.000000p-3>< 0x01.000000p-3><0x1.000000p-3  >\n" +
		"<0.125000><0.12><1.><000.125000><+0.125000>< 0.125000>\n" +
		"<  0x1.000p-3><   -0.12><0x1p-3      >\n" +
		"<0x2p-4><0><0>\n" +
		"<4.32323><4.32323E-05><4.30000><4.32323>\n" +
		"<     4.32323><4.32><4.32><4>\n" +
		"4.32323\n4.32323E-05\n4.32323\n4.32323E-05\n"

	out, errb, code := runTool(t, "", program)
	if out != want || errb != "" || code != 0 {
		t.Fatalf("awk POSIX float formats = (%q, %q, %d), want (%q, %q, 0)", out, errb, code, want, "")
	}
}

func TestAwkPOSIXOctalAlternateFormZeroPrecision(t *testing.T) {
	program := `BEGIN {
		printf "<%#.o><%#.0o><%#.00o><%#.*o><%.0o>\n", 0, 0, 0, 0, 0, 0
		print sprintf("<%#5.0o><%-#5.0o><%#.1o><%#.0o>", 0, 0, 0, 7)
	}`
	want := "<0><0><0><0><>\n<    0><0    ><0><07>\n"

	out, errb, code := runTool(t, "", program)
	if out != want || errb != "" || code != 0 {
		t.Fatalf("awk POSIX octal alternate form = (%q, %q, %d), want (%q, %q, 0)", out, errb, code, want, "")
	}
}

func TestAwkPOSIXEREBackendExpressions(t *testing.T) {
	input := strings.Repeat("a", 252) + "\n" +
		strings.Repeat("b", 253) + "\n" +
		strings.Repeat("c", 254) + "\n" +
		strings.Repeat("d", 255) + "\n"
	program := `/^a{252}$/ { print "literal" }
$0 ~ "^b{253}$" { print "dynamic-match" }
$0 !~ "^z{254}$" && /^c/ { print "dynamic-not-match" }
match($0, "d{255}") { print "match", RSTART, RLENGTH }`

	out, errb, code := runTool(t, input, program)
	want := "literal\ndynamic-match\ndynamic-not-match\nmatch 1 255\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("awk ERE expressions = (%q, %q, %d), want (%q, %q, 0)", out, errb, code, want, "")
	}
}

func TestAwkPOSIXEREDotNewlineAndLeftmostLongest(t *testing.T) {
	program := `BEGIN {
		print ("a\nb" ~ /a.b/), ("a\nb" ~ "a.b")
		print match("ab", "a|ab"), RSTART, RLENGTH
	}`
	out, errb, code := runTool(t, "", program)
	want := "1 1\n1 1 2\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("awk ERE semantics = (%q, %q, %d), want (%q, %q, 0)", out, errb, code, want, "")
	}
}

func TestAwkPOSIXEREOpenIntervalMatches(t *testing.T) {
	program := `BEGIN {
		print match("aabcaab", /([a-c]*){0,}/), RSTART, RLENGTH
		print match("abababccccccd", /(ab){2,}/), RSTART, RLENGTH
		print match("abababccccccd", /(ab){4,}/), RSTART, RLENGTH
	}`
	out, errb, code := runTool(t, "", program)
	want := "1 1 7\n1 1 6\n0 0 -1\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("awk open interval EREs = (%q, %q, %d), want (%q, %q, 0)", out, errb, code, want, "")
	}
}

func TestAwkPOSIXERERejectsAboveAdvertisedLimit(t *testing.T) {
	program := `BEGIN { print match("", /c{256,}/) }`
	_, errb, code := runTool(t, "", program)
	if code == 0 || errb == "" {
		t.Fatalf("awk above-limit interval = (err %q, code %d), want diagnostic failure", errb, code)
	}
}

func TestAwkEREBackendLeavesRecordAndSubstitutionRegexesUnchanged(t *testing.T) {
	program := `BEGIN { FS = "[,:]"; RS = ";+" }
{
		n = split("a,b:c", parts, "[,:]")
		s = "aaabbb"
		sub("a+", "x", s)
		gsub("b+", "y", s)
		print NF, n, s
}`
	out, errb, code := runTool(t, "one:two;;", program)
	want := "2 3 xy\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("awk FS/RS/split/sub/gsub = (%q, %q, %d), want (%q, %q, 0)", out, errb, code, want, "")
	}
}

func TestAwkEREAdapterPreservesPatternSource(t *testing.T) {
	source := `^([[:alpha:]]|\\.){00255}$`
	re, err := (awkERECompiler{}).Compile(source)
	if err != nil {
		t.Fatal(err)
	}
	if got := re.String(); got != source {
		t.Fatalf("Regexp.String() = %q, want exact source %q", got, source)
	}
}

func TestAwkProgramFile(t *testing.T) {
	var out, errb bytes.Buffer
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "print2.awk"), []byte("{print $2}\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader("a b c\n"), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{"-f", "print2.awk"})
	if out.String() != "b\n" || errb.String() != "" || code != 0 {
		t.Errorf("awk -f = (%q, %q, %d), want (%q, %q, 0)", out.String(), errb.String(), code, "b\n", "")
	}
}

func TestAwkPOSIXInterfaceProgramFileAndAssignments(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "first.awk"), []byte(`BEGIN { print prefix }`), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "second.awk"), []byte(`{ print tag ":" $1 }`), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "input"), []byte("row\n"), 0o666); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{"-v", "prefix=pre", "-f", "first.awk", "-f", "second.awk", "tag=late", "input"})
	if code != 0 || errb.String() != "" || out.String() != "pre\nlate:row\n" {
		t.Fatalf("awk -v/-f/file assignments = (%q, %q, %d), want POSIX ordering", out.String(), errb.String(), code)
	}
}

func TestAwkPOSIXProgramFromStdinAndEmptyProgram(t *testing.T) {
	out, errb, code := runTool(t, `{ print $1 }`, "-f", "-")
	if code != 0 || errb != "" || out != "" {
		t.Fatalf("awk -f - should read program source only, got (%q, %q, %d)", out, errb, code)
	}

	if out, errb, code := runTool(t, "", ""); code != 0 || out != "" || errb != "" {
		t.Fatalf("empty awk program = (%q, %q, %d), want no output and status 0", out, errb, code)
	}
}

func TestAwkARGV0UsesInvocationNameWhenAvailable(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:            context.Background(),
		Dir:            t.TempDir(),
		InvocationName: "/vsc/cushim/awk",
		Stdio:          tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{`BEGIN { print ARGV[0] }`})
	if code != 0 || errb.String() != "" || out.String() != "/vsc/cushim/awk\n" {
		t.Fatalf("awk ARGV[0] = (%q, %q, %d), want invocation path", out.String(), errb.String(), code)
	}

	out.Reset()
	errb.Reset()
	rc.InvocationName = ""
	code = cmd.Run(rc, []string{`BEGIN { print ARGV[0] }`})
	if code != 0 || errb.String() != "" || out.String() != "awk\n" {
		t.Fatalf("embedded awk ARGV[0] = (%q, %q, %d), want fallback command name", out.String(), errb.String(), code)
	}
}

func TestAwkInvalidAssignmentAndPOSIXMissingInput(t *testing.T) {
	if _, errb, code := runTool(t, "", "-v", "1bad=x", `BEGIN { print "bad" }`); code != 2 || !strings.Contains(errb, "invalid -v assignment") {
		t.Fatalf("invalid -v assignment = (%q, %d), want usage error", errb, code)
	}
	if out, errb, code := runTool(t, "", `END { print "must-not-run" }`, "missing-input"); code == 0 || !strings.Contains(errb, "missing-input") || out != "" {
		t.Fatalf("missing input = (%q, %q, %d), want fatal diagnostic without END action", out, errb, code)
	}
}

type awkErrorWriter struct{ err error }

func (w awkErrorWriter) Write([]byte) (int, error) { return 0, w.err }

func TestAwkOutputErrorsAndExplicitExitStatus(t *testing.T) {
	wantErr := errors.New("output failed")
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   []string{"LC_ALL=POSIX"},
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: awkErrorWriter{wantErr}, Err: &errb},
	}
	code := cmd.Run(rc, []string{`BEGIN { print "data" }`})
	if code != 2 || !strings.Contains(errb.String(), wantErr.Error()) {
		t.Fatalf("output error = (%q,%d), want diagnostic and status 2", errb.String(), code)
	}

	out, stderr, code := runTool(t, "", `BEGIN { exit 7 }`)
	if code != 7 || out != "" || stderr != "" {
		t.Fatalf("explicit exit = (%q,%q,%d), want (empty,empty,7)", out, stderr, code)
	}
}

func TestResolveFilesPreservesStandaloneOperandSpelling(t *testing.T) {
	files := []string{" 123456789 ", "1.234", "+12345", "-12345", "x=1", "-"}
	rc := &tool.RunContext{Dir: filepath.Join(string(filepath.Separator), "work"), DirIsProcessCwd: true}
	if got := resolveFiles(rc, files); strings.Join(got, "\x00") != strings.Join(files, "\x00") {
		t.Fatalf("standalone resolveFiles() = %q, want original POSIX-visible operands %q", got, files)
	}
}

func TestResolveFilesPreservesEmbeddedOperandSpelling(t *testing.T) {
	dir := filepath.Join(string(filepath.Separator), "virtual", "work")
	rc := &tool.RunContext{Dir: dir}
	files := []string{"input", "x=1", "-"}
	want := []string{"input", "x=1", "-"}
	if got := resolveFiles(rc, files); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("embedded resolveFiles() = %q, want POSIX-visible operands %q", got, want)
	}
}

func TestAwkFilenameUsesLocaleNumericStringRules(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "4,5"), []byte("row\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:             context.Background(),
		Dir:             dir,
		DirIsProcessCwd: true,
		Env:             []string{"LANG=C", "LC_NUMERIC=de_DE.iso88591"},
		Stdio:           tool.Stdio{Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{`{ print (FILENAME == 4.5) }`, "4,5"})
	if code != 0 || errb.String() != "" || out.String() != "1\n" {
		t.Fatalf("numeric FILENAME = (%q,%q,%d), want (1,empty,0)", out.String(), errb.String(), code)
	}
}

func TestAwkRedirectedIOUsesInvocationContext(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "input"), []byte("from-context\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:      context.Background(),
		Dir:      dir,
		Env:      []string{"LC_ALL=POSIX"},
		Umask:    0o027,
		UmaskSet: true,
		Stdio:    tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	program := `BEGIN { if ((getline line < "input") != 1) exit 9; print line > "output"; close("output") }`
	code := cmd.Run(rc, []string{program})
	if code != 0 || errb.String() != "" || out.String() != "" {
		t.Fatalf("context I/O = (%q,%q,%d)", out.String(), errb.String(), code)
	}
	data, err := os.ReadFile(filepath.Join(dir, "output"))
	if err != nil || string(data) != "from-context\n" {
		t.Fatalf("redirected output = (%q,%v)", data, err)
	}
	info, err := os.Stat(filepath.Join(dir, "output"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("redirected output mode = %03o, want 640", got)
	}
}

func TestAwkEmbeddedInputPreservesFilenameSpelling(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "input"), []byte("record\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"LC_ALL=POSIX"},
		Stdio: tool.Stdio{Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{`{ print ARGV[1], FILENAME, $0 }`, "input"})
	if code != 0 || errb.String() != "" || out.String() != "input input record\n" {
		t.Fatalf("embedded input = (%q,%q,%d), want preserved visible filename", out.String(), errb.String(), code)
	}
}

func TestAwkCommandsUseInvocationDirectoryAndEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell command semantics")
	}
	dir := t.TempDir()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"LC_ALL=POSIX", "MARK=from-context"},
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	program := `BEGIN { "pwd" | getline cwd; close("pwd"); "printf %s \"$MARK\"" | getline mark; close("printf %s \"$MARK\""); print cwd; print mark }`
	code := cmd.Run(rc, []string{program})
	wantDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || errb.String() != "" || out.String() != wantDir+"\nfrom-context\n" {
		t.Fatalf("command context = (%q,%q,%d), want cwd and environment", out.String(), errb.String(), code)
	}
}
