package ddcmd

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

func TestDdCopiesFileWithStatusNone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "", "if=in", "of=out", "bs=2", "count=2", "status=none")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("dd: code=%d out=%q err=%q", code, out, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abcd" {
		t.Fatalf("out=%q want abcd", got)
	}
}

func TestDdStdinStdout(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "hello", "bs=2", "count=2", "status=none")
	if code != 0 || errb != "" {
		t.Fatalf("dd stdin: code=%d err=%q", code, errb)
	}
	if out != "hell" {
		t.Fatalf("stdout=%q want hell", out)
	}
}

func TestDdSkipSeekNotrunc(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out"), []byte("abcdefghij"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "if=in", "of=out", "bs=2", "skip=2", "seek=1", "count=2", "conv=notrunc", "status=none")
	if code != 0 {
		t.Fatalf("dd skip seek: code=%d err=%q", code, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ab4567ghij" {
		t.Fatalf("out=%q want ab4567ghij", got)
	}
}

func TestDdErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "", "conv=sync")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Fatalf("conv unsupported: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "", "bad")
	if code != 2 || !strings.Contains(errb, "unrecognized operand") {
		t.Fatalf("bad operand: code=%d err=%q", code, errb)
	}
}
