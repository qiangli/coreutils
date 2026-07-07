package numfmtcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestNumfmtToSI(t *testing.T) {
	out, errb, code := runTool(t, "", "--to=si", "--format=%.1f", "1500")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "1.5K\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestNumfmtFromIEC(t *testing.T) {
	out, errb, code := runTool(t, "", "--from=iec", "--format=%.0f", "2K")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "2048\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestNumfmtFromAutoAndToIECI(t *testing.T) {
	out, errb, code := runTool(t, "", "--from=auto", "--to=iec-i", "--format=%.1f", "1536")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "1.5Ki\n" {
		t.Fatalf("out=%q", out)
	}

	out, errb, code = runTool(t, "", "--from=auto", "--format=%.0f", "2KiB")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "2048\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestNumfmtStdinFields(t *testing.T) {
	// GNU default field is 1, for stdin as for operands.
	out, errb, code := runTool(t, "1K 2K\n", "--from=si", "--format=%.0f")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "1000 2K\n" {
		t.Fatalf("out=%q", out)
	}
	out, errb, code = runTool(t, "1K 2K\n", "--from=si", "--field=1,2", "--format=%.0f")
	if code != 0 || errb != "" {
		t.Fatalf("fields 1,2: code=%d err=%q", code, errb)
	}
	if out != "1000 2000\n" {
		t.Fatalf("fields 1,2: out=%q", out)
	}
}

func TestNumfmtFieldDelimiterSuffixAndSeparator(t *testing.T) {
	out, errb, code := runTool(t, "a,1000,x\nb,2000,y\n", "-d", ",", "--field=2", "--to=si", "--format=%.1f", "--suffix=B", "--unit-separator= ")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	// An explicit --format precision is honored verbatim (no trimming).
	if out != "a,1.0 KB,x\nb,2.0 KB,y\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestNumfmtUnitsPaddingAndInvalidFail(t *testing.T) {
	out, errb, code := runTool(t, "", "--from-unit=2", "--to-unit=4", "--padding=4", "--format=%.0f", "10")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "   5\n" {
		t.Fatalf("out=%q", out)
	}

	out, errb, code = runTool(t, "x 2\n", "--field=-", "--invalid=fail", "--format=%.0f")
	if code != 1 || !strings.Contains(errb, "invalid number") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "x 2\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestNumfmtHeaderZeroRoundAndGrouping(t *testing.T) {
	out, errb, code := runTool(t, "name\x0012345.67\x00", "-z", "--header=1", "--grouping", "--round=down", "--format=%.1f")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	// --grouping has no effect in the C locale (GNU behavior).
	if out != "name\x0012345.6\x00" {
		t.Fatalf("out=%q", out)
	}

	out, errb, code = runTool(t, "", "--round=towards-zero", "--format=%.0f", "--", "-12.9")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "-12\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestNumfmtErrors(t *testing.T) {
	_, errb, code := runTool(t, "", "--to=bad", "1")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "", "x")
	if code != 1 || !strings.Contains(errb, "invalid number") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "", "--from=si", "1Ki")
	if code != 1 || !strings.Contains(errb, "invalid number") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "", "--to=auto", "1")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "", "--round=sideways", "1")
	if code != 2 || !strings.Contains(errb, "invalid --round") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}

// GNU human formatting: scaled values below 10 get one decimal
// (rounded per --round, from-zero by default), 10 and above round to
// an integer; rounding can carry into the next unit.
func TestNumfmtHumanDefaultPrecision(t *testing.T) {
	cases := []struct{ in, want string }{
		{"1000", "1.0K"},
		{"1001", "1.1K"},
		{"1500", "1.5K"},
		{"9999", "10K"},
		{"15000", "15K"},
		{"999999", "1.0M"},
		{"999", "999"},
		{"5", "5"},
		{"5.5", "5.5"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, "", "--to=si", c.in)
		if code != 0 || errb != "" {
			t.Fatalf("%s: code=%d err=%q", c.in, code, errb)
		}
		if out != c.want+"\n" {
			t.Fatalf("--to=si %s = %q, want %q", c.in, out, c.want)
		}
	}
	out, _, code := runTool(t, "", "--to=iec", "1536")
	if code != 0 || out != "1.5K\n" {
		t.Fatalf("--to=iec 1536 = (%q, %d)", out, code)
	}
}

// A bare scaling suffix with no digits must be an invalid-number error,
// not a crash.
func TestNumfmtBareSuffixDoesNotPanic(t *testing.T) {
	for _, from := range []string{"iec-i", "auto"} {
		_, errb, code := runTool(t, "", "--from="+from, "1i")
		if code != 1 || !strings.Contains(errb, "invalid number") {
			t.Fatalf("--from=%s 1i: code=%d err=%q", from, code, errb)
		}
	}
}

// GNU validates --format: only a single %[0]['][-][N][.N]f directive.
func TestNumfmtFormatValidation(t *testing.T) {
	_, errb, code := runTool(t, "", "--format=%d", "5")
	if code != 2 || !strings.Contains(errb, "invalid format") {
		t.Fatalf("%%d: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "", "--format=no directive", "5")
	if code != 2 || !strings.Contains(errb, "has no % directive") {
		t.Fatalf("no directive: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "", "--format=%f%f", "5")
	if code != 2 || !strings.Contains(errb, "too many % directives") {
		t.Fatalf("two directives: code=%d err=%q", code, errb)
	}
	// The ' grouping flag is accepted (a no-op in the C locale).
	out, errb, code := runTool(t, "", "--format=%'f", "12345")
	if code != 0 || errb != "" || out != "12345\n" {
		t.Fatalf("%%'f: (%q, %q, %d)", out, errb, code)
	}
	// A width in --format pads the number.
	out, _, code = runTool(t, "", "--format=%10f", "--to=si", "1500")
	if code != 0 || out != "       1.5K\n" {
		t.Fatalf("%%10f: (%q, %d)", out, code)
	}
}

// Whitespace-shaped input keeps its shape: a field with leading
// whitespace is implicitly padded to its original width.
func TestNumfmtImplicitPadding(t *testing.T) {
	out, errb, code := runTool(t, "  5000  x\n", "--to=si")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "  5.0K  x\n" {
		t.Fatalf("out=%q", out)
	}
	out, _, code = runTool(t, "1000 hello\n", "--to=si")
	if code != 0 || out != "1.0K hello\n" {
		t.Fatalf("field 1 default: (%q, %d)", out, code)
	}
}
