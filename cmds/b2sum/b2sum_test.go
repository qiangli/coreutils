package b2sumcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

const (
	abcB2 = "ba80a53f981c4d0d6a2797b69f12f6e94c212f14685ac4b74b12bb6fdbffa2d17d87c5392aab792dc252d5de4533cc9518d38aa8dbf1925ab92386edd4009923"
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

func TestB2SumStdinAndFiles(t *testing.T) {
	out, _, code := runTool(t, "", "abc")
	if out != abcB2+"  -\n" || code != 0 {
		t.Fatalf("stdin = (%q, %d)", out, code)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code = runTool(t, dir, "", "-b", "a.txt")
	if out != abcB2+" *a.txt\n" || code != 0 {
		t.Fatalf("file -b = (%q, %d)", out, code)
	}
}

func TestB2SumCheckAndErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sums.txt"), []byte(abcB2+"  a.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "", "-c", "sums.txt")
	if out != "a.txt: OK\n" || errb != "" || code != 0 {
		t.Fatalf("check = (%q, %q, %d)", out, errb, code)
	}

	_, errb, code = runTool(t, dir, "", "--status")
	if code != 2 || !strings.Contains(errb, "status") || !strings.Contains(errb, "pure-Go") {
		t.Fatalf("unsupported flag = (%q, %d)", errb, code)
	}
}
