package mkfifocmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

func TestMkfifoCreatesFIFO(t *testing.T) {
	dir := t.TempDir()
	out, errb, code := runTool(t, dir, "pipe")
	if runtime.GOOS == "windows" {
		if code != 1 || !strings.Contains(strings.ToLower(errb), "not supported") {
			t.Fatalf("windows mkfifo: code=%d err=%q", code, errb)
		}
		return
	}
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("mkfifo pipe: code=%d out=%q err=%q", code, out, errb)
	}
	fi, err := os.Lstat(filepath.Join(dir, "pipe"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("mode=%v, want named pipe", fi.Mode())
	}
}

func TestMkfifoMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native FIFO creation is unsupported on windows")
	}
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-m", "600", "pipe")
	if code != 0 {
		t.Fatalf("mkfifo -m: code=%d err=%q", code, errb)
	}
	fi, err := os.Lstat(filepath.Join(dir, "pipe"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o, want 600", fi.Mode().Perm())
	}
}

func TestMkfifoSymbolicMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native FIFO creation is unsupported on windows")
	}
	tests := []struct {
		mode string
		want os.FileMode
	}{
		{"u=rw,go=", 0o600},
		{"a=rw,u+x", 0o766},
		{"+x", 0o777},
		{"a+X", 0o666},
		{"u=rw,g=u,o=g", 0o666},
	}
	for i, tc := range tests {
		name := fmt.Sprintf("pipe%d", i)
		dir := t.TempDir()
		_, errb, code := runTool(t, dir, "-m", tc.mode, name)
		if code != 0 {
			t.Fatalf("mkfifo -m %q: code=%d err=%q", tc.mode, code, errb)
		}
		fi, err := os.Lstat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != tc.want {
			t.Errorf("mkfifo -m %q mode=%03o, want %03o", tc.mode, got, tc.want)
		}
	}
}

func TestMkfifoErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-m", "999", "pipe")
	if code != 2 || !strings.Contains(errb, "invalid mode '999'") {
		t.Errorf("invalid mode: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-m", "u+q", "pipe")
	if code != 2 || !strings.Contains(errb, "invalid mode 'u+q'") {
		t.Errorf("invalid symbolic mode: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "pipe")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestMkfifoHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: mkfifo") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "mkfifo") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
