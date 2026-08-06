package uuencodecmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &err}}
	code := cmd.Run(rc, args)
	return out.String(), err.String(), code
}

func TestEncodeKnownVectorFromStdin(t *testing.T) {
	out, err, code := runTool(t, t.TempDir(), "Cat", "cat.txt")
	if code != 0 || err != "" || out != "begin 666 cat.txt\n#0V%T\n`\nend\n" {
		t.Fatalf("got (%q, %q, %d)", out, err, code)
	}
}

func TestEncodeFileAndMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in")
	if err := os.WriteFile(path, []byte("hello\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "ignored", "in", "remote name")
	if code != 0 || errb != "" || !strings.HasPrefix(out, "begin 640 remote name\n&:&5L;&\\*\n") {
		t.Fatalf("got (%q, %q, %d)", out, errb, code)
	}
}

func TestErrorsAndUnsupportedVariant(t *testing.T) {
	for _, tc := range [][]string{{}, {"a", "b", "c"}, {"-m", "name"}, {"missing", "name"}} {
		_, err, code := runTool(t, t.TempDir(), "", tc...)
		if code == 0 || err == "" {
			t.Errorf("args %v: err=%q code=%d", tc, err, code)
		}
	}
}
