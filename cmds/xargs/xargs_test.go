package xargscmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runXargs(t *testing.T, in string, args ...string) (out, errOut string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   os.Environ(),
		Stdio: tool.Stdio{In: strings.NewReader(in), Out: &o, Err: &e},
	}
	code = cmd.Run(rc, args)
	return o.String(), e.String(), code
}

func TestXargsDefaultEcho(t *testing.T) {
	// No command ⇒ echo; all items on one line.
	if out, _, _ := runXargs(t, "a b c\n"); out != "a b c\n" {
		t.Errorf("default echo = %q, want 'a b c'", out)
	}
}

func TestXargsMaxArgsBatches(t *testing.T) {
	// -n2 ⇒ two items per echo invocation.
	out, _, _ := runXargs(t, "1 2 3 4 5\n", "-n2", "echo")
	if out != "1 2\n3 4\n5\n" {
		t.Errorf("-n2 = %q, want batches of 2", out)
	}
}

func TestXargsClusteredShortOptionsWithAttachedArguments(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		clustered []string
		split     []string
	}{
		{name: "max-args", input: "a b\n", clustered: []string{"-txn1", "echo"}, split: []string{"-t", "-x", "-n1", "echo"}},
		{name: "size", input: "an-item-that-exceeds-the-limit\n", clustered: []string{"-txs24", "echo"}, split: []string{"-t", "-x", "-s24", "echo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantOut, wantErr, wantCode := runXargs(t, tt.input, tt.split...)
			gotOut, gotErr, gotCode := runXargs(t, tt.input, tt.clustered...)
			if gotCode != wantCode || gotOut != wantOut || gotErr != wantErr {
				t.Fatalf("clustered=(%d,%q,%q), split=(%d,%q,%q)", gotCode, gotOut, gotErr, wantCode, wantOut, wantErr)
			}
		})
	}
}

func TestXargsNullDelimited(t *testing.T) {
	out, _, _ := runXargs(t, "a\x00b c\x00", "-0", "echo")
	if out != "a b c\n" { // "b c" stays one item (NUL only)
		t.Errorf("-0 = %q, want 'a b c'", out)
	}
}

func TestXargsQuotesAndBackslash(t *testing.T) {
	out, _, _ := runXargs(t, `'a b' c\ d`, "-n1", "echo")
	// items: "a b", "c d" (quote groups; backslash escapes the space)
	if out != "a b\nc d\n" {
		t.Errorf("quote/backslash split = %q, want 'a b' then 'c d'", out)
	}
}

func TestXargsReplace(t *testing.T) {
	out, _, _ := runXargs(t, "x\ny\n", "-I", "{}", "echo", "[{}]")
	if out != "[x]\n[y]\n" {
		t.Errorf("-I {} = %q, want [x] then [y]", out)
	}
}

func TestXargsDelimiter(t *testing.T) {
	out, _, _ := runXargs(t, "a,b,c", "-d", ",", "echo")
	if out != "a b c\n" {
		t.Errorf("-d , = %q, want 'a b c'", out)
	}
}

func TestXargsEOFString(t *testing.T) {
	out, _, _ := runXargs(t, "a\nb\n_END_\nc\n", "-E", "_END_", "echo")
	if out != "a b\n" {
		t.Errorf("-E stop = %q, want 'a b'", out)
	}
}

func TestXargsNoRunIfEmpty(t *testing.T) {
	// Without -r, empty input still runs the command once (no extra args).
	if out, _, _ := runXargs(t, "   \n", "echo", "ran"); out != "ran\n" {
		t.Errorf("empty without -r = %q, want 'ran'", out)
	}
	// With -r, empty input runs nothing.
	if out, _, _ := runXargs(t, "   \n", "-r", "echo", "ran"); out != "" {
		t.Errorf("empty with -r = %q, want no output", out)
	}
}

func TestXargsParallelRunsAll(t *testing.T) {
	// -P4 -n1: every item runs; order is unspecified, so compare as a set.
	out, _, _ := runXargs(t, "1 2 3 4 5 6\n", "-P4", "-n1", "echo")
	got := strings.Fields(strings.TrimSpace(out))
	sort.Strings(got)
	want := []string{"1", "2", "3", "4", "5", "6"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("-P4 -n1 ran %v, want all of %v", got, want)
	}
}

func TestXargsCommandNotFound(t *testing.T) {
	_, errOut, code := runXargs(t, "x\n", "definitely-not-a-real-cmd-xyz")
	if code != 127 {
		t.Errorf("missing command exit = %d, want 127", code)
	}
	if !strings.Contains(errOut, "command not found") {
		t.Errorf("error wording = %q", errOut)
	}
}

func TestXargsTrace(t *testing.T) {
	_, errOut, _ := runXargs(t, "a\n", "-t", "echo")
	if !strings.Contains(errOut, "echo a") {
		t.Errorf("-t trace = %q, want the command echoed to stderr", errOut)
	}
}

func TestXargsInteractiveReadsControllingTerminal(t *testing.T) {
	original := ttyOpener
	t.Cleanup(func() { ttyOpener = original })
	opened := 0
	ttyOpener = func() (io.ReadCloser, error) {
		opened++
		return io.NopCloser(strings.NewReader("yes\nno\nY\n")), nil
	}

	out, errOut, code := runXargs(t, "a b c\n", "-p", "-n1", "echo")
	if code != 0 || out != "a\nc\n" {
		t.Fatalf("-p: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if opened != 1 {
		t.Fatalf("controlling terminal opened %d times, want once", opened)
	}
	if strings.Count(errOut, "?...") != 3 || !strings.Contains(errOut, "echo a?...") {
		t.Fatalf("-p prompts=%q, want traced command and one prompt per batch", errOut)
	}
}

func TestXargsInteractiveUsesLCMessagesYesexpr(t *testing.T) {
	original := ttyOpener
	t.Cleanup(func() { ttyOpener = original })
	ttyOpener = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("ja\nnein\nY\n")), nil
	}

	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Env:   append(os.Environ(), "LC_ALL=", "LANG=C", "LC_MESSAGES=de_DE.UTF-8"),
		Stdio: tool.Stdio{In: strings.NewReader("a b c\n"), Out: &out, Err: &errOut},
	}
	code := cmd.Run(rc, []string{"-p", "-n1", "echo"})
	if code != 0 || out.String() != "a\nc\n" {
		t.Fatalf("-p German replies: code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
}

func TestXargsInteractiveYesexprPrecedenceAndAnchoring(t *testing.T) {
	tests := []struct {
		name  string
		reply string
		env   []string
		want  bool
	}{
		{name: "POSIX yes", reply: "yes", env: []string{"LC_ALL=C"}, want: true},
		{name: "POSIX leading blank", reply: " y", env: []string{"LC_ALL=C"}, want: false},
		{name: "German category", reply: "ja", env: []string{"LANG=C", "LC_MESSAGES=de_DE.UTF-8"}, want: true},
		{name: "LC_ALL overrides category", reply: "ja", env: []string{"LC_MESSAGES=de_DE.UTF-8", "LC_ALL=C"}, want: false},
		{name: "empty LC_ALL falls through", reply: "J", env: []string{"LC_MESSAGES=de_DE.UTF-8", "LC_ALL="}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := affirmativeReply(tt.reply, tt.env); got != tt.want {
				t.Errorf("affirmativeReply(%q, %v) = %v, want %v", tt.reply, tt.env, got, tt.want)
			}
		})
	}
}

func TestXargsLogicalLines(t *testing.T) {
	out, _, code := runXargs(t, "a b\nc\nd e\n", "-L", "2", "echo")
	if code != 0 || out != "a b c\nd e\n" {
		t.Fatalf("-L 2: out=%q code=%d", out, code)
	}
}

func TestXargsRejectsZeroLogicalLineLimit(t *testing.T) {
	_, errOut, code := runXargs(t, "a\n", "-L0", "echo")
	if code == 0 || !strings.Contains(errOut, "-L requires a positive number") {
		t.Fatalf("-L0: code=%d stderr=%q", code, errOut)
	}
}

func TestXargsLogicalLinesContinueAfterTrailingBlank(t *testing.T) {
	input := "one \n\n two\nthree\nfour \t\n\nfive\nsix\n"
	out, errOut, code := runXargs(t, input, "-L2", "echo")
	if code != 0 || errOut != "" || out != "one two three\nfour five six\n" {
		t.Fatalf("-L trailing-blank continuation: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestXargsLogicalLinesDoNotContinueEscapedTrailingBlank(t *testing.T) {
	out, errOut, code := runXargs(t, "one\\ \ntwo\nthree\n", "-L2", "echo")
	if code != 0 || errOut != "" || out != "one  two\nthree\n" {
		t.Fatalf("-L escaped trailing blank: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestXargsLastInputLimitOptionWins(t *testing.T) {
	out, _, code := runXargs(t, "a b\nc d\n", "-n", "1", "-L", "2", "echo")
	if code != 0 || out != "a b c d\n" {
		t.Fatalf("last input limit option: out=%q code=%d", out, code)
	}
}

func TestXargsReplaceUsesWholeLine(t *testing.T) {
	out, _, code := runXargs(t, "a b\nc d\n", "-I", "{}", "echo", "[{}]")
	if code != 0 || out != "[a b]\n[c d]\n" {
		t.Fatalf("-I line replacement: out=%q code=%d", out, code)
	}
}

func TestXargsReplaceIssue7Limits(t *testing.T) {
	_, errOut, code := runXargs(t, "value\n", "-I{}", "echo", "{}", "{}", "{}", "{}", "{}", "{}")
	if code == 0 || !strings.Contains(errOut, "five arguments") {
		t.Fatalf("-I replacement argument limit: code=%d stderr=%q", code, errOut)
	}

	long := strings.Repeat("x", 256)
	_, errOut, code = runXargs(t, long+"\n", "-I{}", "echo", "{}")
	if code == 0 || !strings.Contains(errOut, "255 bytes") {
		t.Fatalf("-I constructed argument limit: code=%d stderr=%q", code, errOut)
	}
}

func TestXargsReplaceAppliesOnlyToArgumentOperands(t *testing.T) {
	command := []string{"utility{}", "{}", "pre{}", "{}post", "x{}y", "{}"}
	batches, err := plan(command, []inputItem{{value: "item", line: 1}}, options{replace: "{}", maxChars: 1024})
	if err != nil {
		t.Fatalf("plan with five replacement arguments: %v", err)
	}
	want := []string{"utility{}", "item", "preitem", "itempost", "xitemy", "item"}
	if len(batches) != 1 || strings.Join(batches[0], "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("-I planned argv=%v, want %v", batches, want)
	}
}

func TestXargsReplaceIgnoresOnlyLeadingUnquotedUnescapedBlanks(t *testing.T) {
	input := " \tplain\n'  'quoted\n\\ escaped\n"
	out, errOut, code := runXargs(t, input, "-I{}", "echo", "[{}]")
	if code != 0 || errOut != "" || out != "[plain]\n[  quoted]\n[ escaped]\n" {
		t.Fatalf("-I leading blanks: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestXargsReplaceUnquotesEachLogicalLineBeforeEOF(t *testing.T) {
	for _, input := range []string{"aaaa\n\"end\"\ncccc\n", "aaaa\n'end'\ncccc\n", "aaaa\n\\e\\n\\d\ncccc\n"} {
		out, errOut, code := runXargs(t, input, "-E", "end", "-I", "{}", "echo", "{}")
		if code != 0 || errOut != "" || out != "aaaa\n" {
			t.Errorf("input %q: code=%d stdout=%q stderr=%q", input, code, out, errOut)
		}
	}
}

func TestXargsRejectsMalformedQuotedInput(t *testing.T) {
	for _, input := range []string{"'unterminated", "trailing\\"} {
		_, errOut, code := runXargs(t, input, "echo")
		if code == 0 || !strings.Contains(errOut, "unmatched") {
			t.Errorf("input %q: code=%d stderr=%q, want unmatched-input failure", input, code, errOut)
		}
	}
}

func TestXargsSizeLimitAndExactMode(t *testing.T) {
	out, _, code := runXargs(t, "a bb ccc\n", "-s", "10", "echo")
	if code != 0 || out != "a bb\nccc\n" {
		t.Fatalf("-s batching: out=%q code=%d", out, code)
	}
	_, errOut, code := runXargs(t, "oversized\n", "-s", "5", "-x", "echo")
	if code == 0 || !strings.Contains(errOut, "size") {
		t.Fatalf("-s -x oversize: code=%d stderr=%q", code, errOut)
	}
}

func TestXargsDefaultSizeBatchesBeforeExecLimit(t *testing.T) {
	original := systemArgMax
	t.Cleanup(func() { systemArgMax = original })
	systemArgMax = func() int { return argMaxHeadroom + 12 }

	out, errOut, code := runXargsEnv(t, t.TempDir(), nil, "aaa bbb ccc\n", "echo")
	if code != 0 || errOut != "" || out != "aaa\nbbb\nccc\n" {
		t.Fatalf("default size batching: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestXargsSizeLimitAppliesToEmptyInvocation(t *testing.T) {
	original := systemArgMax
	t.Cleanup(func() { systemArgMax = original })
	systemArgMax = func() int { return argMaxHeadroom + 3 }

	_, errOut, code := runXargsEnv(t, t.TempDir(), nil, "", "echo", "fixed")
	if code == 0 || !strings.Contains(errOut, "command exceeds -s size limit") {
		t.Fatalf("empty-input command size: code=%d stderr=%q", code, errOut)
	}
}

func TestXargsSizeClampAccountsForEnvironment(t *testing.T) {
	original := systemArgMax
	t.Cleanup(func() { systemArgMax = original })
	systemArgMax = func() int { return argMaxHeadroom + 17 }

	// PAD=1234 occupies nine bytes including its terminating NUL, leaving an
	// eight-byte argv budget even though the explicit -s value is larger.
	out, errOut, code := runXargsEnv(t, t.TempDir(), []string{"PAD=1234"}, "a b\n", "-s100", "echo")
	if code != 0 || errOut != "" || out != "a\nb\n" {
		t.Fatalf("environment-aware -s clamp: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestXargsReplaceHonorsForcedExactSize(t *testing.T) {
	original := systemArgMax
	t.Cleanup(func() { systemArgMax = original })
	systemArgMax = func() int { return 1 << 20 }

	_, errOut, code := runXargsEnv(t, t.TempDir(), nil, "abcd\n", "-s8", "-I{}", "echo", "{}")
	if code == 0 || !strings.Contains(errOut, "size") {
		t.Fatalf("oversized -I command: code=%d stderr=%q", code, errOut)
	}
}
