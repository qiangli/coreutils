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

func TestMoreRejectsInteractiveFlags(t *testing.T) {
	_, errb, code := runMore(t, t.TempDir(), "", "-d")
	if code != 2 || !strings.Contains(errb, "not every GNU flag is implemented") {
		t.Fatalf("more -d code=%d err=%q", code, errb)
	}
}
