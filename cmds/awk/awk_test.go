package awkcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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

func TestAwkPOSIXFloatFormats(t *testing.T) {
	program := `BEGIN {
		printf "<%a><%A><%a>\n", 0, 0.125, -0.125
		printf "<%#.0a><%.2A>\n", 0.125, 0.125
		printf "<%015.6a><%+015.6a><% 015.6a><%-15.6a>\n", 0.125, 0.125, 0.125, 0.125
		printf "<%F><%.2F><%#.0F><%010F><%+F><% F>\n", 0.125, 0.125, 1, 0.125, 0.125, 0.125
		print sprintf("<%*.*a><%*.*F><%*.*a>", 12, 3, 0.125, 8, 2, -0.125, -12, -1, 0.125)
		printf "<%.*a><%.*F><%.*f>\n", -0.5, 0.1, -0.5, 0.125, -0.5, 0.125
	}`
	want := "<0x0p+0><0X1P-3><-0x1p-3>\n" +
		"<0x1.p-3><0X1.00P-3>\n" +
		"<0x001.000000p-3><+0x01.000000p-3>< 0x01.000000p-3><0x1.000000p-3  >\n" +
		"<0.125000><0.12><1.><000.125000><+0.125000>< 0.125000>\n" +
		"<  0x1.000p-3><   -0.12><0x1p-3      >\n" +
		"<0x2p-4><0><0>\n"

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
	input := strings.Repeat("a", 1001) + "\n" +
		strings.Repeat("b", 1002) + "\n" +
		strings.Repeat("c", 1003) + "\n" +
		strings.Repeat("d", 1004) + "\n"
	program := `/^a{1001}$/ { print "literal" }
$0 ~ "^b{1002}$" { print "dynamic-match" }
$0 !~ "^z{1003}$" && /^c/ { print "dynamic-not-match" }
match($0, "d{1004}") { print "match", RSTART, RLENGTH }`

	out, errb, code := runTool(t, input, program)
	want := "literal\ndynamic-match\ndynamic-not-match\nmatch 1 1004\n"
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
	source := `^([[:alpha:]]|\\.){01001}$`
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

func TestResolveFilesPreservesStandaloneOperandSpelling(t *testing.T) {
	files := []string{" 123456789 ", "1.234", "+12345", "-12345", "x=1", "-"}
	rc := &tool.RunContext{Dir: filepath.Join(string(filepath.Separator), "work"), DirIsProcessCwd: true}
	if got := resolveFiles(rc, files); strings.Join(got, "\x00") != strings.Join(files, "\x00") {
		t.Fatalf("standalone resolveFiles() = %q, want original POSIX-visible operands %q", got, files)
	}
}

func TestResolveFilesResolvesEmbeddedOperands(t *testing.T) {
	dir := filepath.Join(string(filepath.Separator), "virtual", "work")
	rc := &tool.RunContext{Dir: dir}
	files := []string{"input", "x=1", "-"}
	want := []string{filepath.Join(dir, "input"), "x=1", "-"}
	if got := resolveFiles(rc, files); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("embedded resolveFiles() = %q, want %q", got, want)
	}
}
