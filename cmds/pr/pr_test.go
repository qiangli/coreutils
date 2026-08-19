package prcmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

type firstWriteSignal struct{ ch chan struct{} }

func (w firstWriteSignal) Write(p []byte) (int, error) {
	select {
	case w.ch <- struct{}{}:
	default:
	}
	return len(p), nil
}

func runPR(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

// writeFixed writes a file and pins its mtime so pr headers are
// deterministic in tests.
func writeFixed(t *testing.T, dir, name, content string) time.Time {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2020, 1, 2, 3, 4, 0, 0, time.Local)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	return stamp
}

func TestPROmitHeaderPassesContent(t *testing.T) {
	out, errb, code := runPR(t, t.TempDir(), "a\nb\n", "-t")
	if out != "a\nb\n" || errb != "" || code != 0 {
		t.Fatalf("pr -t = (%q, %q, %d)", out, errb, code)
	}
}

func TestPRDefaultPageStructure(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "in", "l1\nl2\nl3\n")
	out, errb, code := runPR(t, dir, "", "in")
	if errb != "" || code != 0 {
		t.Fatalf("pr default = (%q, %d)", errb, code)
	}
	header := "2020-01-02 03:04" + strings.Repeat(" ", 24) + "in" + strings.Repeat(" ", 24) + "Page 1"
	want := "\n\n" + header + "\n\n\n" + "l1\nl2\nl3\n" + strings.Repeat("\n", 58)
	if out != want {
		t.Fatalf("pr default page = %q, want %q", out, want)
	}
	if n := strings.Count(out, "\n"); n != 66 {
		t.Fatalf("pr default page has %d lines, want 66", n)
	}
}

func TestPRStreamsCompletePageBeforeEOF(t *testing.T) {
	// A pipe or FIFO producer may remain open while consuming pr's output.
	// GNU pr emits each completed page without waiting for input EOF.
	reader, writer := io.Pipe()
	wrote := make(chan struct{}, 1)
	done := make(chan int, 1)
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: reader, Out: firstWriteSignal{ch: wrote}, Err: io.Discard},
	}
	go func() { done <- cmd.Run(rc, nil) }()

	// Default pages contain 56 body lines. Keep the input open after one page
	// to prove output does not depend on EOF.
	if _, err := io.WriteString(writer, strings.Repeat("x\n", 56)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-wrote:
	case <-time.After(time.Second):
		writer.Close()
		t.Fatal("pr produced no complete page while pipe remained open")
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("pr exited %d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("pr did not finish after pipe EOF")
	}
}

func TestPRMultiColumnStreamsCompletePageBeforeEOF(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "vertical", args: []string{"-t", "-l", "3", "-2"}},
		{name: "across", args: []string{"-t", "-l", "3", "-a", "-2"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reader, writer := io.Pipe()
			wrote := make(chan struct{}, 1)
			done := make(chan int, 1)
			rc := &tool.RunContext{
				Ctx: context.Background(), Dir: t.TempDir(),
				Stdio: tool.Stdio{In: reader, Out: firstWriteSignal{ch: wrote}, Err: io.Discard},
			}
			go func() { done <- cmd.Run(rc, tt.args) }()

			// Three rows by two columns complete one page.  Keep the producer
			// open: output must depend on a full page, not on EOF.
			if _, err := io.WriteString(writer, strings.Repeat("x\n", 6)); err != nil {
				t.Fatal(err)
			}
			select {
			case <-wrote:
			case <-time.After(time.Second):
				writer.Close()
				t.Fatal("pr produced no multi-column page while pipe remained open")
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case code := <-done:
				if code != 0 {
					t.Fatalf("pr exited %d", code)
				}
			case <-time.After(time.Second):
				t.Fatal("pr did not finish after pipe EOF")
			}
		})
	}
}

func TestPRSingleColumnNeverTruncatedByDefault(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "in", "abcdef\n")
	out, _, code := runPR(t, dir, "", "-t", "-w", "3", "in")
	if out != "abcdef\n" || code != 0 {
		t.Fatalf("pr -w must not truncate single-column output = (%q, %d)", out, code)
	}
	out, _, code = runPR(t, dir, "", "-t", "-W", "3", "in")
	if out != "abc\n" || code != 0 {
		t.Fatalf("pr -W truncates = (%q, %d), want abc", out, code)
	}
}

func TestPRShortPageLengthImpliesOmitHeader(t *testing.T) {
	out, _, code := runPR(t, t.TempDir(), "a\nb\nc\n", "-l", "3")
	if out != "a\nb\nc\n" || code != 0 {
		t.Fatalf("pr -l3 (<=10 implies -t) = (%q, %d), want passthrough", out, code)
	}
}

func TestPRNumberIndentAndDoubleSpace(t *testing.T) {
	out, errb, code := runPR(t, t.TempDir(), "a\nb\n", "-t", "-n", "-o", "2", "-d")
	want := "      1\ta\n\n      2\tb\n\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("pr line controls = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestPRCustomHeaderAndTOmitPagination(t *testing.T) {
	out, _, code := runPR(t, t.TempDir(), "a\n", "-h", "TITLE", "-w", "50")
	if code != 0 || !strings.Contains(out, "TITLE") {
		t.Fatalf("pr custom header = (%q, %d), want TITLE", out, code)
	}

	out, _, code = runPR(t, t.TempDir(), "a\nb\nc\n", "-T", "-l", "2")
	if out != "a\nb\nc\n" || code != 0 {
		t.Fatalf("pr -T = (%q, %d), want passthrough", out, code)
	}
}

func TestPRPagesRangeAndDateFormat(t *testing.T) {
	out, _, code := runPR(t, t.TempDir(), "a\nb\nc\nd\n", "--pages", "2", "-l", "13", "-D", "%Y")
	if code != 0 {
		t.Fatalf("pr pages exited %d", code)
	}
	if strings.Contains(out, "Page 1") || !strings.Contains(out, "Page 2") || strings.Contains(out, "a\n") || !strings.Contains(out, "d\n") {
		t.Fatalf("pr pages = %q, want only page 2", out)
	}
	if n := strings.Count(out, "\n"); n != 13 {
		t.Fatalf("pr page 2 has %d lines, want 13", n)
	}
}

func TestPRPlusOperandPageRange(t *testing.T) {
	out, _, code := runPR(t, t.TempDir(), "a\nb\nc\nd\n", "-l", "13", "+2")
	if code != 0 || strings.Contains(out, "Page 1") || !strings.Contains(out, "Page 2") {
		t.Fatalf("pr +2 = (%q, %d), want only page 2", out, code)
	}

	_, errb, code := runPR(t, t.TempDir(), "", "+0")
	if code != 2 || !strings.Contains(errb, "invalid page range") {
		t.Fatalf("pr +0 code=%d err=%q", code, errb)
	}
}

func TestPRFormFeedTrailer(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"-F", []string{"-F"}},
		{"-f", []string{"-f"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _, code := runPR(t, t.TempDir(), "a\nb\n", tt.args...)
			if code != 0 || strings.Count(out, "\f") != 1 || !strings.HasSuffix(out, "b\n\f") {
				t.Fatalf("pr %s = (%q, %d), want single trailing form feed", tt.name, out, code)
			}
		})
	}
}

func TestPRFormFeedLowerEqualsUpper(t *testing.T) {
	outF, _, codeF := runPR(t, t.TempDir(), "a\nb\n", "-F")
	outf, _, codef := runPR(t, t.TempDir(), "a\nb\n", "-f")
	if codeF != 0 || codef != 0 {
		t.Fatalf("pr -F/-f exit codes: %d, %d", codeF, codef)
	}
	if outF != outf {
		t.Fatalf("pr -f output differs from pr -F:\n-F: %q\n-f: %q", outF, outf)
	}
}

func TestPRInputFormFeedsBreakPages(t *testing.T) {
	// -t keeps input form feeds as page breaks.
	out, _, code := runPR(t, t.TempDir(), "a\fb\n", "-t")
	if out != "a\n\fb\n" || code != 0 {
		t.Fatalf("pr -t form feed = (%q, %d), want %q", out, code, "a\n\fb\n")
	}
	// -T eliminates them.
	out, _, code = runPR(t, t.TempDir(), "a\fb\n", "-T")
	if out != "a\nb\n" || code != 0 {
		t.Fatalf("pr -T form feed = (%q, %d), want %q", out, code, "a\nb\n")
	}
	// Paginated: the form feed starts a new page.
	out, _, code = runPR(t, t.TempDir(), "a\n\fb\n", "-l", "20")
	if code != 0 || !strings.Contains(out, "Page 2") || strings.Contains(out, "Page 3") {
		t.Fatalf("pr paginated form feed = (%q, %d), want 2 pages", out, code)
	}
	if n := strings.Count(out, "\n"); n != 40 {
		t.Fatalf("pr paginated form feed has %d lines, want 40", n)
	}
	// Consecutive form feeds produce an empty page.
	out, _, code = runPR(t, t.TempDir(), "a\n\f\fb\n", "-l", "20")
	if code != 0 || !strings.Contains(out, "Page 3") {
		t.Fatalf("pr double form feed = (%q, %d), want 3 pages", out, code)
	}
	if n := strings.Count(out, "\n"); n != 60 {
		t.Fatalf("pr double form feed has %d lines, want 60", n)
	}
	// A leading form feed represents an empty first page once data follows.
	out, _, code = runPR(t, t.TempDir(), "\fb\n", "-l", "20")
	if code != 0 || !strings.Contains(out, "Page 2") || strings.Count(out, "\n") != 40 {
		t.Fatalf("pr leading form feed = (%q, %d), want empty page then data page", out, code)
	}
}

func TestPRVerticalColumns(t *testing.T) {
	input := "a\nbb\nc\ndd\ne\n"
	want := "a    dd\nbb   e\nc\n"
	for _, args := range [][]string{{"-t", "-w", "10", "-2"}, {"-t", "-w", "10", "--columns=2"}, {"-t", "-w", "10", "--column", "2"}} {
		out, errb, code := runPR(t, t.TempDir(), input, args...)
		if out != want || errb != "" || code != 0 {
			t.Errorf("pr %v = (%q, %q, %d), want (%q, \"\", 0)", args, out, errb, code, want)
		}
	}
}

func TestPRVerticalColumnsUnevenFinalPage(t *testing.T) {
	out, errb, code := runPR(t, t.TempDir(), "a\nb\nc\nd\ne\nf\ng\n", "-t", "-l", "3", "-w", "10", "--columns=2")
	want := "a    d\nb    e\nc    f\ng\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("pr vertical uneven final page = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestPRAcrossColumns(t *testing.T) {
	input := "a\nbb\nc\ndd\ne\n"
	out, errb, code := runPR(t, t.TempDir(), input, "-t", "-w", "10", "-2", "-a")
	if want := "a    bb\nc    dd\ne\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("pr -a = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
	_, errb, code = runPR(t, t.TempDir(), "a\n", "-a", "-m")
	if code != 1 || !strings.Contains(errb, "cannot specify both printing across and printing in parallel") {
		t.Fatalf("pr -am code=%d err=%q", code, errb)
	}
}

func TestPRVerticalColumnsInteractions(t *testing.T) {
	out, errb, code := runPR(t, t.TempDir(), "a\nb\n", "-t", "-w", "10", "-o", "2", "-s:", "-2")
	if want := "  a:b\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("pr vertical separator/margin = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}

	out, errb, code = runPR(t, t.TempDir(), "a\nb\nc\nd\n", "-t", "-w", "10", "-d", "-2")
	if want := "a    c\n\nb    d\n\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("pr vertical double-space = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestPRMerge(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "left", "a\nbb\nc\n")
	writeFixed(t, dir, "right", "1\n")
	out, errb, code := runPR(t, dir, "", "-m", "-t", "-w", "20", "-s:", "left", "right")
	if want := "a\t :1\nbb\t :\nc\t :\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("pr -m = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
	_, errb, code = runPR(t, dir, "", "-m", "-2", "left", "right")
	if code != 1 || !strings.Contains(errb, "cannot specify number of columns") {
		t.Fatalf("pr -m -2 code=%d err=%q", code, errb)
	}
}

func TestPRMergeThreeFilesAndPagination(t *testing.T) {
	dir := t.TempDir()
	writeFixed(t, dir, "one", "a\nb\nc\nd\n")
	writeFixed(t, dir, "two", "1\n2\n")
	writeFixed(t, dir, "three", "x\ny\nz\n")
	out, errb, code := runPR(t, dir, "", "-m", "-t", "-w", "18", "one", "two", "three")
	if want := "a     1\t    x\nb     2\t    y\nc     \t    z\nd     \t    \n"; out != want || errb != "" || code != 0 {
		t.Fatalf("pr -m three files = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
	out, errb, code = runPR(t, dir, "", "-m", "-l", "13", "-D", "%Y", "one", "two")
	if errb != "" || code != 0 || !strings.Contains(out, "Page 1") || !strings.Contains(out, "Page 2") || strings.Count(out, "\n") != 26 {
		t.Fatalf("paginated pr -m = (%q, %q, %d)", out, errb, code)
	}
}

func TestPRExpandTabs(t *testing.T) {
	out, _, code := runPR(t, t.TempDir(), "a\tb\n", "-t", "-e")
	if want := "a       b\n"; out != want || code != 0 {
		t.Fatalf("pr expand tabs = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestPRNoFileWarnings(t *testing.T) {
	out, errb, code := runPR(t, t.TempDir(), "", "-r", "missing")
	if out != "" || errb != "" || code != 1 {
		t.Fatalf("pr -r missing = (%q, %q, %d), want quiet exit 1", out, errb, code)
	}
}

func TestPRRejectsBadLength(t *testing.T) {
	_, errb, code := runPR(t, t.TempDir(), "", "-l", "0")
	if code != 2 || !strings.Contains(errb, "invalid page length") {
		t.Fatalf("pr bad length code=%d err=%q", code, errb)
	}
}

func TestPRRejectsBadIndent(t *testing.T) {
	_, errb, code := runPR(t, t.TempDir(), "", "-o", "-1")
	if code != 2 || !strings.Contains(errb, "invalid indent") {
		t.Fatalf("pr bad indent code=%d err=%q", code, errb)
	}
}

func TestPRFirstLineNumber(t *testing.T) {
	out, _, code := runPR(t, t.TempDir(), "a\nb\n", "-t", "-n", "-N", "10")
	want := "   10\ta\n   11\tb\n"
	if out != want || code != 0 {
		t.Fatalf("pr -N 10 = (%q, %d), want (%q, 0)", out, code, want)
	}

	_, errb, code := runPR(t, t.TempDir(), "", "-N", "0", "a")
	if code != 2 || !strings.Contains(errb, "invalid first line number") {
		t.Fatalf("pr -N 0 code=%d err=%q", code, errb)
	}
}

func TestPRNewFlagAliases(t *testing.T) {
	// --column is an alias for --columns
	out, errb, code := runPR(t, t.TempDir(), "a\nb\n", "-t", "-w", "10", "--column", "2")
	if out != "a    b\n" || errb != "" || code != 0 {
		t.Fatalf("pr --column 2 = (%q, %q, %d)", out, errb, code)
	}

	// -J is accepted (join-lines, no-op)
	out, _, code = runPR(t, t.TempDir(), "a\nb\n", "-J", "-t")
	if out != "a\nb\n" || code != 0 {
		t.Fatalf("pr -J = (%q, %d)", out, code)
	}

	// -i is accepted (indent style, no-op)
	out, _, code = runPR(t, t.TempDir(), "a\n", "-i", "-t")
	if out != "a\n" || code != 0 {
		t.Fatalf("pr -i = (%q, %d)", out, code)
	}
}
