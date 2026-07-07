package morecmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runMore(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
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

func TestMoreReadsStdin(t *testing.T) {
	out, errb, code := runMore(t, t.TempDir(), "a\nb\n")
	if out != "a\nb\n" || errb != "" || code != 0 {
		t.Fatalf("more stdin = (%q, %q, %d)", out, errb, code)
	}
}

func TestMoreConcatenatesFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runMore(t, dir, "", "a", "b")
	if out != "a\nb\n" || code != 0 {
		t.Fatalf("more files = (%q, %d)", out, code)
	}
}

func TestMoreSqueezeAndFromLine(t *testing.T) {
	out, errb, code := runMore(t, t.TempDir(), "one\n\n\ntwo\n", "-s", "-F", "2")
	if want := "\ntwo\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("more squeeze/from-line = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestMoreAcceptsDisplayOnlyFlags(t *testing.T) {
	out, errb, code := runMore(t, t.TempDir(), "a\nb\n", "-d", "-f", "-p", "-c", "-n", "5")
	if out != "a\nb\n" || errb != "" || code != 0 {
		t.Fatalf("more display flags = (%q, %q, %d)", out, errb, code)
	}

	out, errb, code = runMore(t, t.TempDir(), "a\nb\n", "-l", "-e", "--number", "5")
	if out != "a\nb\n" || errb != "" || code != 0 {
		t.Fatalf("more alias flags = (%q, %q, %d)", out, errb, code)
	}

	out, errb, code = runMore(t, t.TempDir(), "a\n", "-10")
	if out != "a\n" || errb != "" || code != 0 {
		t.Fatalf("more numeric screen size = (%q, %q, %d)", out, errb, code)
	}
}

func TestMorePatternStartsAtMatch(t *testing.T) {
	out, errb, code := runMore(t, t.TempDir(), "alpha\nbeta\ngamma\n", "-P", "^bet")
	if want := "beta\ngamma\n"; out != want || errb != "" || code != 0 {
		t.Fatalf("more pattern = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestMoreRejectsBadLineCounts(t *testing.T) {
	_, errb, code := runMore(t, t.TempDir(), "", "-F", "0")
	if code != 2 || !strings.Contains(errb, "invalid starting line") {
		t.Fatalf("more bad from-line code=%d err=%q", code, errb)
	}

	_, errb, code = runMore(t, t.TempDir(), "", "-P", "[")
	if code != 2 || !strings.Contains(errb, "invalid pattern") {
		t.Fatalf("more bad pattern code=%d err=%q", code, errb)
	}
}
