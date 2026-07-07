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

func TestPtxMissingFile(t *testing.T) {
	_, errb, code := runTool(t, t.TempDir(), "", "missing")
	if code != 1 || !strings.Contains(errb, "cannot open") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}
