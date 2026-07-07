package prcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestPROmitHeaderPassesContent(t *testing.T) {
	out, errb, code := runPR(t, t.TempDir(), "a\nb\n", "-t")
	if out != "a\nb\n" || errb != "" || code != 0 {
		t.Fatalf("pr -t = (%q, %q, %d)", out, errb, code)
	}
}

func TestPRPaginatesWithHeaders(t *testing.T) {
	out, _, code := runPR(t, t.TempDir(), "a\nb\nc\n", "-l", "3", "-w", "40")
	if code != 0 {
		t.Fatalf("pr exited %d: %q", code, out)
	}
	if strings.Count(out, "Page ") != 3 || !strings.Contains(out, "standard input") {
		t.Fatalf("pr headers not found in %q", out)
	}
}

func TestPRReadsFilesAndTruncatesWidth(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("abcdef\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runPR(t, dir, "", "-t", "-w", "3", "in")
	if out != "abc\n" || code != 0 {
		t.Fatalf("pr file width = (%q, %d), want abc", out, code)
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
	out, _, code := runPR(t, t.TempDir(), "a\n", "-h", "TITLE", "-l", "3", "-w", "50")
	if code != 0 || !strings.Contains(out, "TITLE") {
		t.Fatalf("pr custom header = (%q, %d), want TITLE", out, code)
	}

	out, _, code = runPR(t, t.TempDir(), "a\nb\nc\n", "-T", "-l", "2")
	if out != "a\nb\nc\n" || code != 0 {
		t.Fatalf("pr -T = (%q, %d), want passthrough", out, code)
	}
}

func TestPRColumnsTabsPagesAndDateFormat(t *testing.T) {
	out, _, code := runPR(t, t.TempDir(), "a\nb\nc\nd\n", "-t", "--columns", "2", "-s", "|", "-W", "20")
	if want := "a|c\nb|d\n"; out != want || code != 0 {
		t.Fatalf("pr columns = (%q, %d), want (%q, 0)", out, code, want)
	}

	out, _, code = runPR(t, t.TempDir(), "a\nb\nc\nd\n", "-t", "--columns", "2", "-a", "-S", ":", "-W", "20")
	if want := "a:b\nc:d\n"; out != want || code != 0 {
		t.Fatalf("pr across columns = (%q, %d), want (%q, 0)", out, code, want)
	}

	out, _, code = runPR(t, t.TempDir(), "a\tb\n", "-t", "-e")
	if want := "a       b\n"; out != want || code != 0 {
		t.Fatalf("pr expand tabs = (%q, %d), want (%q, 0)", out, code, want)
	}

	out, _, code = runPR(t, t.TempDir(), "a\nb\nc\n", "--pages", "2", "-l", "3", "-w", "60", "-D", "%Y")
	if code != 0 || strings.Contains(out, " a\n") || !strings.Contains(out, "Page 2") {
		t.Fatalf("pr pages/date = (%q, %d), want only page 2", out, code)
	}
}

func TestPRMergeAndFormFeed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("a1\na2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b"), []byte("b1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runPR(t, dir, "", "-m", "-s", "|", "a", "b")
	if want := "a1|b1\na2|\n"; out != want || code != 0 {
		t.Fatalf("pr merge = (%q, %d), want (%q, 0)", out, code, want)
	}

	out, _, code = runPR(t, t.TempDir(), "a\nb\n", "-l", "3", "-F")
	if code != 0 || !strings.Contains(out, "\f") {
		t.Fatalf("pr form feed = (%q, %d), want form feed", out, code)
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
