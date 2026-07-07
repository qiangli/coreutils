package nlcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runNL(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
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

func TestNLDefaultsNumberNonEmptyLines(t *testing.T) {
	out, errb, code := runNL(t, t.TempDir(), "a\n\nb\n")
	want := "     1\ta\n      \t\n     2\tb\n"
	if out != want || errb != "" || code != 0 {
		t.Fatalf("nl default = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want)
	}
}

func TestNLStylesAndFormatting(t *testing.T) {
	out, _, code := runNL(t, t.TempDir(), "a\n\n", "-b", "a", "-n", "rz", "-s", ":", "-w", "3")
	if want := "001:a\n002:\n"; out != want || code != 0 {
		t.Fatalf("nl formatted = (%q, %d), want (%q, 0)", out, code, want)
	}
	out, _, code = runNL(t, t.TempDir(), "a\n", "-b", "n")
	if out != "a\n" || code != 0 {
		t.Fatalf("nl -bn = (%q, %d), want unnumbered", out, code)
	}
}

func TestNLReadsFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b"), []byte("y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runNL(t, dir, "", "-w", "1", "a", "b")
	if want := "1\tx\n2\ty\n"; out != want || code != 0 {
		t.Fatalf("nl files = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestNLRejectsBadStyle(t *testing.T) {
	_, errb, code := runNL(t, t.TempDir(), "", "-b", "pREGEXP")
	if code != 2 || !strings.Contains(errb, "invalid body numbering style") {
		t.Fatalf("nl bad style code=%d err=%q", code, errb)
	}
}
