package ttycmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, in io.Reader, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: in, Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestTTYNotAFile(t *testing.T) {
	out, errb, code := runTool(t, strings.NewReader("data"))
	if code != 1 || out != "not a tty\n" || errb != "" {
		t.Errorf("strings.Reader stdin = (%q, %q, %d), want (\"not a tty\\n\", \"\", 1)", out, errb, code)
	}
}

func TestTTYRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out, _, code := runTool(t, f)
	if code != 1 || out != "not a tty\n" {
		t.Errorf("regular file stdin = (%q, %d), want (\"not a tty\\n\", 1)", out, code)
	}
}

func TestTTYDevNull(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no /dev/null")
	}
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out, _, code := runTool(t, f)
	if code != 1 || out != "not a tty\n" {
		t.Errorf("/dev/null stdin = (%q, %d), want (\"not a tty\\n\", 1)", out, code)
	}
}

func TestTTYRealTerminal(t *testing.T) {
	// Positive path: only meaningful when the test itself has a
	// controlling terminal (developer machines; headless CI skips).
	f, ok := interface{}(os.Stdin).(*os.File)
	if !ok {
		t.Skip("no *os.File stdin")
	}
	if _, isTTY := ttyName(f); !isTTY {
		t.Skip("stdin is not a terminal in this environment")
	}
	out, _, code := runTool(t, f)
	if code != 0 || !strings.HasSuffix(out, "\n") || out == "not a tty\n" {
		t.Errorf("terminal stdin = (%q, %d), want a device name and 0", out, code)
	}
	if runtime.GOOS != "windows" && !strings.HasPrefix(out, "/dev/") {
		t.Errorf("terminal name %q does not start with /dev/", out)
	}
}

func TestTTYErrors(t *testing.T) {
	_, errb, code := runTool(t, strings.NewReader(""), "extra")
	if code != 2 || !strings.Contains(errb, "extra operand") {
		t.Errorf("operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, strings.NewReader(""), "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestTTYHelp(t *testing.T) {
	out, _, code := runTool(t, strings.NewReader(""), "--help")
	if code != 0 || !strings.Contains(out, "Usage: tty") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
}
