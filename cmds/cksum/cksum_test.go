package cksumcmd

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

func TestCKSumStdinAndFiles(t *testing.T) {
	out, _, code := runTool(t, "", "abc")
	if out != "1219131554 3\n" || code != 0 {
		t.Fatalf("stdin = (%q, %d)", out, code)
	}
	out, _, code = runTool(t, "", "")
	if out != "4294967295 0\n" || code != 0 {
		t.Fatalf("empty stdin = (%q, %d)", out, code)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code = runTool(t, dir, "", "a.txt")
	if out != "1219131554 3 a.txt\n" || code != 0 {
		t.Fatalf("file = (%q, %d)", out, code)
	}
}

func TestCKSumErrors(t *testing.T) {
	_, errb, code := runTool(t, "", "", "missing")
	if code != 1 || !strings.Contains(errb, "cksum: missing: No such file or directory") {
		t.Fatalf("missing = (%q, %d)", errb, code)
	}
	_, errb, code = runTool(t, "", "", "--algorithm=sha1")
	if code != 2 || !strings.Contains(errb, "algorithm") {
		t.Fatalf("unsupported flag = (%q, %d)", errb, code)
	}
}
