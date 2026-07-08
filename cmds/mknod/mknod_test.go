package mknodcmd

import (
	"bytes"
	"context"
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

func TestMknodCreatesFIFO(t *testing.T) {
	dir := t.TempDir()
	out, errb, code := runTool(t, dir, "pipe", "p")
	if runtime.GOOS == "windows" {
		if code != 1 || !strings.Contains(strings.ToLower(errb), "not supported") {
			t.Fatalf("windows mknod p: code=%d err=%q", code, errb)
		}
		return
	}
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("mknod pipe p: code=%d out=%q err=%q", code, out, errb)
	}
	fi, err := os.Lstat(filepath.Join(dir, "pipe"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("mode=%v, want named pipe", fi.Mode())
	}
}

func TestMknodMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native special-file creation is unsupported on windows")
	}
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-m", "600", "pipe", "p")
	if code != 0 {
		t.Fatalf("mknod -m: code=%d err=%q", code, errb)
	}
	fi, err := os.Lstat(filepath.Join(dir, "pipe"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o, want 600", fi.Mode().Perm())
	}
}

func TestMknodUsageErrors(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no args", nil, "missing operand"},
		{"one arg", []string{"node"}, "missing operand"},
		{"bad type", []string{"node", "x"}, "invalid device type 'x'"},
		{"fifo extra", []string{"node", "p", "1"}, "extra operand '1'"},
		{"char missing minor", []string{"node", "c", "1"}, "missing operand after '1'"},
		{"block extra", []string{"node", "b", "1", "2", "3"}, "extra operand '3'"},
		{"bad major", []string{"node", "c", "nope", "2"}, "invalid major device number 'nope'"},
		{"bad minor", []string{"node", "c", "1", "nope"}, "invalid minor device number 'nope'"},
		{"bad mode", []string{"-m", "999", "node", "p"}, "invalid mode '999'"},
		{"symbolic mode", []string{"-m", "u+rw", "node", "p"}, "not supported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errb, code := runTool(t, dir, tt.args...)
			if code != 2 || !strings.Contains(errb, tt.want) {
				t.Fatalf("code=%d err=%q, want code 2 containing %q", code, errb, tt.want)
			}
		})
	}

	_, errb, code := runTool(t, dir, "--frobnicate", "node", "p")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestMknodHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: mknod") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "mknod") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
