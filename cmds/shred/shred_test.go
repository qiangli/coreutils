package shredcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestShredOverwritesAndZeroes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("secret data"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errb, code := runTool(t, dir, "-n", "1", "-z", "file")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("shred: code=%d out=%q err=%q", code, out, errb)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bytes.Repeat([]byte{0}, len(got))) {
		t.Fatalf("file was not zeroed: %x", got)
	}
}

func TestShredRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("secret data"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, errb, code := runTool(t, dir, "-n", "0", "-u", "file")
	if code != 0 {
		t.Fatalf("shred -u: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still exists or unexpected stat error: %v", err)
	}
}

func TestShredErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing file operand") {
		t.Fatalf("missing operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "subdir")
	if code != 1 || !strings.Contains(errb, "not a regular file") {
		t.Fatalf("directory operand: code=%d err=%q", code, errb)
	}
}
