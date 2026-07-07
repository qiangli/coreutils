package prcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

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
	out, _, code := runPR(t, t.TempDir(), "a\nb\n", "-F")
	if code != 0 || strings.Count(out, "\f") != 1 || !strings.HasSuffix(out, "b\n\f") {
		t.Fatalf("pr -F = (%q, %d), want single trailing form feed", out, code)
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
}

func TestPRColumnsAndMergeNotSupported(t *testing.T) {
	_, errb, code := runPR(t, t.TempDir(), "a\n", "--columns", "2")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Fatalf("pr --columns 2 code=%d err=%q", code, errb)
	}
	_, errb, code = runPR(t, t.TempDir(), "a\n", "-a")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Fatalf("pr -a code=%d err=%q", code, errb)
	}
	_, errb, code = runPR(t, t.TempDir(), "a\n", "-m")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Fatalf("pr -m code=%d err=%q", code, errb)
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
