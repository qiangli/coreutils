package ptxcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir string, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestPtxStdin(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "beta alpha\n", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	want := "beta                            \talpha\t\n" +
		"                                \tbeta\talpha\n"
	if out != want {
		t.Fatalf("out=%q", out)
	}
}

func TestPtxIgnoreCaseSortKeepsOriginalText(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "Zulu alpha\n", "-f", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	want := "Zulu                            \talpha\t\n" +
		"                                \tZulu\talpha\n"
	if out != want {
		t.Fatalf("out=%q", out)
	}
}

func TestPtxIgnoreFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ignore"), []byte("beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "alpha beta\n", "-i", "ignore", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if strings.Contains(out, "\tbeta\t") || !strings.Contains(out, "\talpha\t") {
		t.Fatalf("out=%q", out)
	}
}

func TestPtxReferencesAndAutoReference(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "ref1 alpha beta\nref2 gamma alpha\n", "-r", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if !strings.Contains(out, "ref1") || !strings.Contains(out, "\talpha\tbeta") {
		t.Fatalf("out=%q", out)
	}

	out, errb, code = runTool(t, t.TempDir(), "alpha beta\n", "-A", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if !strings.Contains(out, ":1") || !strings.Contains(out, "\tbeta\t") {
		t.Fatalf("out=%q", out)
	}
}

func TestPtxGapWidthAndWordRegexp(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "aa bbb cccc\n", "-g", "2", "-w", "6", "-W", "[bc]+", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if !strings.Contains(out, "aa  bbb  ccc") || !strings.Contains(out, "bbb  cccc") {
		t.Fatalf("out=%q", out)
	}
	if strings.Count(out, "\n") != 2 {
		t.Fatalf("out=%q", out)
	}
}

func TestPtxTraditionalTypesetAndRightRefs(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "r1 alpha beta\n", "-r", "-R", "-G", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if !strings.Contains(out, "alpha beta r1") || !strings.Contains(out, "alpha beta r1") {
		t.Fatalf("out=%q", out)
	}

	out, errb, code = runTool(t, t.TempDir(), "alpha beta\n", "-t", "-A", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if !strings.Contains(out, ".xx \"alpha\"") || !strings.Contains(out, "\":1\"") {
		t.Fatalf("out=%q", out)
	}
}

func TestPtxBreakFileAndSentenceRegexp(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "breaks"), []byte("-"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "alpha-beta\n", "-b", "breaks", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if !strings.Contains(out, "\talpha\tbeta") || !strings.Contains(out, "\tbeta\t") {
		t.Fatalf("out=%q", out)
	}

	out, errb, code = runTool(t, dir, "alpha beta. gamma delta.\n", "-S", "[^.]+\\.", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if strings.Contains(out, "beta. gamma") {
		t.Fatalf("sentence boundary was not honored: %q", out)
	}
}

func TestPtxMissingFile(t *testing.T) {
	_, errb, code := runTool(t, t.TempDir(), "", "missing")
	if code != 1 || !strings.Contains(errb, "cannot open") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}
