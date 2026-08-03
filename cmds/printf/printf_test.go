package printfcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/multicall"
	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runToolEnv(t, nil, args...)
}

func runToolEnv(t *testing.T, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestPrintfBasicConversions(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"%s\n", "hi"}, "hi\n"},
		{[]string{"%d\n", "42"}, "42\n"},
		{[]string{"%i\n", "-7"}, "-7\n"},
		{[]string{"%o\n", "8"}, "10\n"},
		{[]string{"%u\n", "5"}, "5\n"},
		{[]string{"%x\n", "255"}, "ff\n"},
		{[]string{"%X\n", "255"}, "FF\n"},
		{[]string{"%c\n", "abc"}, "a\n"},
		{[]string{"%b\n", "a\\tb"}, "a\tb\n"},
		{[]string{"%%\n"}, "%\n"},
		{[]string{"literal\n"}, "literal\n"},
		{[]string{"%d %s\n", "1", "two"}, "1 two\n"},
		// Unsigned conversions accept a leading '-' and wrap two's complement.
		{[]string{"%u\n", "-1"}, "18446744073709551615\n"},
		{[]string{"%x\n", "-1"}, "ffffffffffffffff\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("printf %v = (%q, err=%q, code=%d), want (%q, 0)", c.args, out, errb, code, c.want)
		}
	}
}

func TestPrintfFloatingConversions(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"%f\n", "3.5"}, "3.500000\n"},
		{[]string{"%.2f\n", "3.14159"}, "3.14\n"},
		{[]string{"%e\n", "12345.6789"}, "1.234568e+04\n"},
		{[]string{"%.3g\n", "0.0001234"}, "0.000123\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("printf %v = (%q, err=%q, code=%d), want (%q, 0)", c.args, out, errb, code, c.want)
		}
	}
}

func TestPrintfWidthAndPrecision(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"%5d|\n", "3"}, "    3|\n"},
		{[]string{"%-5d|\n", "3"}, "3    |\n"},
		{[]string{"%05d\n", "3"}, "00003\n"},
		{[]string{"%.5d\n", "3"}, "00003\n"},
		{[]string{"%.0d\n", "0"}, "\n"},
		{[]string{"%10s|\n", "hi"}, "        hi|\n"},
		{[]string{"%-10s|\n", "hi"}, "hi        |\n"},
		{[]string{"%.2s\n", "hello"}, "he\n"},
		// Argument forms ('*') for width and precision.
		{[]string{"%*d|\n", "5", "3"}, "    3|\n"},
		{[]string{"%.*f\n", "2", "3.14159"}, "3.14\n"},
		{[]string{"%*.*f\n", "8", "2", "3.14159"}, "    3.14\n"},
		// Negative width argument means left-justify.
		{[]string{"%*d|\n", "-5", "3"}, "3    |\n"},
		// Negative precision argument means "precision omitted".
		{[]string{"%.*s\n", "-1", "hello"}, "hello\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("printf %v = (%q, err=%q, code=%d), want (%q, 0)", c.args, out, errb, code, c.want)
		}
	}
}

func TestPrintfBackslashEscapes(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{`a\tb\n`}, "a\tb\n"},
		{[]string{`\101\102\103\n`}, "ABC\n"}, // \NNN octal, no leading zero required
		{[]string{`\x41\x42\n`}, "AB\n"},
		{[]string{`\\\n`}, "\\\n"},
		{[]string{`\e[0m`}, "\x1b[0m"},
		// \c stops ALL further output immediately, including the trailing newline.
		{[]string{`before\cafter\n`}, "before"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("printf %v = (%q, err=%q, code=%d), want (%q, 0)", c.args, out, errb, code, c.want)
		}
	}
}

func TestPrintfPercentBEscapes(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		// %b interprets escapes in the ARGUMENT, not the format string.
		{[]string{"%b\n", `a\tb`}, "a\tb\n"},
		// %b octal form requires the leading zero (\0ddd), unlike the format string's \NNN.
		{[]string{"%b\n", `\0101`}, "A\n"},
		{[]string{"%b\n", `\101`}, "\\101\n"},
		// \c inside %b halts the ENTIRE printf invocation, not just this argument.
		{[]string{"%s-%b-%s\n", "x", `a\cb`, "z"}, "x-a"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("printf %v = (%q, err=%q, code=%d), want (%q, 0)", c.args, out, errb, code, c.want)
		}
	}
}

func TestPrintfNumericCharacterConstants(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"%d\n", "'A"}, "65\n"},
		{[]string{"%d\n", `"A`}, "65\n"},
		{[]string{"%x\n", "'A"}, "41\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("printf %v = (%q, err=%q, code=%d), want (%q, 0)", c.args, out, errb, code, c.want)
		}
	}
	// Trailing characters after the constant are ignored, with a warning.
	out, errb, code := runTool(t, "%d\n", "'AB")
	if out != "65\n" || code != 0 {
		t.Errorf("char constant with trailing chars: out=%q code=%d", out, code)
	}
	if !strings.Contains(errb, "ignored") {
		t.Errorf("char constant with trailing chars: expected warning, got stderr=%q", errb)
	}
}

func TestPrintfNumericBases(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"%d\n", "0x1A"}, "26\n"},
		{[]string{"%d\n", "017"}, "15\n"},
		{[]string{"%d\n", "10"}, "10\n"},
	}
	for _, c := range cases {
		out, _, code := runTool(t, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("printf %v = (%q, code=%d), want (%q, 0)", c.args, out, code, c.want)
		}
	}
}

func TestPrintfFormatReuse(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"%s\n", "a", "b", "c"}, "a\nb\nc\n"},
		{[]string{"%s %s\n", "a", "b", "c"}, "a b\nc \n"},
		// A format with no consuming directive is used exactly once, even
		// with leftover ARGUMENTs — reuse would loop forever otherwise.
		{[]string{"abc\n", "1", "2", "3"}, "abc\n"},
		// Zero ARGUMENTs: FORMAT is still used once, missing values default.
		{[]string{"%s-%d\n"}, "-0\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("printf %v = (%q, err=%q, code=%d), want (%q, 0)", c.args, out, errb, code, c.want)
		}
	}
}

func TestPrintfDiagnosticsAndStatus(t *testing.T) {
	// Missing operand.
	_, errb, code := runTool(t)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}

	// Invalid number: nonzero status, diagnostic naming the bad value, but
	// output still proceeds with 0.
	out, errb, code := runTool(t, "%d\n", "abc")
	if code != 1 || out != "0\n" || !strings.Contains(errb, "abc") {
		t.Errorf("invalid number: out=%q err=%q code=%d", out, errb, code)
	}

	// Invalid conversion specification: fatal format error, nonzero status,
	// partial output already produced is preserved.
	out, errb, code = runTool(t, "ok %z bad")
	if code != 1 || out != "ok " || errb == "" {
		t.Errorf("invalid conversion: out=%q err=%q code=%d", out, errb, code)
	}

	// %b rejects flags/width/precision (not part of the documented grammar).
	_, errb, code = runTool(t, "%5b\n", "x")
	if code != 1 || errb == "" {
		t.Errorf("%%5b: err=%q code=%d, want fatal", errb, code)
	}
}

func TestPrintfBinarySafe(t *testing.T) {
	// %s/%b/%c must round-trip raw, non-UTF8 bytes and embedded NULs
	// unchanged (LC_ALL=C / byte semantics, never rune-based).
	raw := "\xff\x00\xfe\x01"
	out, _, code := runTool(t, "%s", raw)
	if code != 0 || out != raw {
		t.Errorf("%%s binary passthrough: out=%q, want %q", out, raw)
	}

	out, _, code = runTool(t, "%b", raw)
	if code != 0 || out != raw {
		t.Errorf("%%b binary passthrough: out=%q, want %q", out, raw)
	}

	out, _, code = runTool(t, "%c", "\xffrest")
	if code != 0 || out != "\xff" {
		t.Errorf("%%c binary first byte: out=%q, want %q", out, "\xff")
	}

	// Precision on %s truncates by byte count, not rune count.
	out, _, code = runTool(t, "%.1s", "\xff\xfe")
	if code != 0 || out != "\xff" {
		t.Errorf("%%.1s byte-precision: out=%q, want %q", out, "\xff")
	}
}

func TestPrintfDoubleDashAndOperands(t *testing.T) {
	// A leading "--" ends option scanning so a literal FORMAT beginning
	// with "-" is not mistaken for an option.
	out, _, code := runTool(t, "--", "-n%s\n", "x")
	if code != 0 || out != "-nx\n" {
		t.Errorf("-- operand handling: out=%q code=%d", out, code)
	}
	// Without "--", the same FORMAT text is still just an operand — printf
	// (unlike echo) has no single-argument option other than --help/--version.
	out, _, code = runTool(t, "-n%s\n", "x")
	if code != 0 || out != "-nx\n" {
		t.Errorf("bare -n operand: out=%q code=%d", out, code)
	}
}

func TestPrintfHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: printf") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "-h")
	if code != 0 || !strings.Contains(out, "Usage: printf") {
		t.Errorf("-h: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "--version")
	if code != 0 || !strings.Contains(out, "printf") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "-V")
	if code != 0 || !strings.Contains(out, "printf") {
		t.Errorf("-V: code=%d out=%q", code, out)
	}
	// --help/--version are only special-cased as the SOLE argument: printf
	// treats "%s\n" --help" as ordinary FORMAT+ARGUMENT operands.
	out, _, code = runTool(t, "%s\n", "--help")
	if code != 0 || out != "--help\n" {
		t.Errorf("--help as ARGUMENT: out=%q code=%d, want literal operand", out, code)
	}
}

func TestPrintfMulticallResolution(t *testing.T) {
	name, toolArgs, listOnly := multicall.Resolve("printf", []string{"%s\n", "hi"}, "coreutils")
	if listOnly || name != "printf" {
		t.Fatalf("Resolve(argv0=printf) = (name=%q, listOnly=%v), want (printf, false)", name, listOnly)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb}}
	if code := multicall.Dispatch(rc, name, toolArgs); code != 0 || out.String() != "hi\n" {
		t.Fatalf("Dispatch via argv0 symlink: out=%q err=%q code=%d", out.String(), errb.String(), code)
	}

	// Also reachable through the "coreutils printf …" front door.
	out.Reset()
	errb.Reset()
	name, toolArgs, listOnly = multicall.Resolve("coreutils", []string{"printf", "%d\n", "42"}, "coreutils")
	if listOnly || name != "printf" {
		t.Fatalf("Resolve(coreutils printf) = (name=%q, listOnly=%v), want (printf, false)", name, listOnly)
	}
	if code := multicall.Dispatch(rc, name, toolArgs); code != 0 || out.String() != "42\n" {
		t.Fatalf("Dispatch via coreutils front door: out=%q err=%q code=%d", out.String(), errb.String(), code)
	}
}
