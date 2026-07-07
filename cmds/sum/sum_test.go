package sumcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestSumBSDAndSysV(t *testing.T) {
	out, _, code := runTool(t, "", "abc")
	if out != "16556     1\n" || code != 0 {
		t.Fatalf("bsd stdin = (%q, %d)", out, code)
	}
	out, _, code = runTool(t, "", "abc", "-r")
	if out != "16556     1\n" || code != 0 {
		t.Fatalf("-r stdin = (%q, %d)", out, code)
	}
	out, _, code = runTool(t, "", "abc", "-s")
	if out != "294 1\n" || code != 0 {
		t.Fatalf("-s stdin = (%q, %d)", out, code)
	}
	out, _, code = runTool(t, "", "abc", "--sysv")
	if out != "294 1\n" || code != 0 {
		t.Fatalf("--sysv stdin = (%q, %d)", out, code)
	}
	// -r and -s may be combined: the last one wins (GNU).
	out, _, code = runTool(t, "", "abc", "-r", "-s")
	if out != "294 1\n" || code != 0 {
		t.Fatalf("-r -s = (%q, %d)", out, code)
	}
	out, _, code = runTool(t, "", "abc", "-s", "-r")
	if out != "16556     1\n" || code != 0 {
		t.Fatalf("-s -r = (%q, %d)", out, code)
	}
	out, _, code = runTool(t, "", "abc", "-sr")
	if out != "16556     1\n" || code != 0 {
		t.Fatalf("-sr = (%q, %d)", out, code)
	}
	// An explicit "-" operand prints the name.
	out, _, code = runTool(t, "", "abc", "-")
	if out != "16556     1 -\n" || code != 0 {
		t.Fatalf("explicit dash = (%q, %d)", out, code)
	}
}

func TestSumFilesAndErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, "", "a.txt")
	if out != "16556     1 a.txt\n" || code != 0 {
		t.Fatalf("file = (%q, %d)", out, code)
	}
	_, errb, code := runTool(t, dir, "", "missing")
	if code != 1 || !strings.Contains(errb, "sum: missing: No such file or directory") {
		t.Fatalf("missing = (%q, %d)", errb, code)
	}
	// The invented --bsd long form is gone; -r has no long form in GNU.
	_, errb, code = runTool(t, dir, "", "--bsd")
	if code != 2 || !strings.Contains(errb, "bsd") {
		t.Fatalf("--bsd = (%q, %d)", errb, code)
	}
}
