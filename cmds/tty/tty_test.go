package ttycmd

import (
	"bytes"
	"context"
	"errors"
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

func TestTTYSilent(t *testing.T) {
	// All three spellings (-s, --silent, --quiet) are the same flag:
	// suppress output and report only the terminal/non-terminal status.
	for _, flag := range []string{"-s", "--silent", "--quiet"} {
		out, errb, code := runTool(t, strings.NewReader("data"), flag)
		if code != 1 || out != "" || errb != "" {
			t.Errorf("tty %s = (%q, %q, %d), want quiet status 1", flag, out, errb, code)
		}
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

// failWriter fails every write, simulating a full or closed stdout.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("no space left on device") }

func TestTTYWriteError(t *testing.T) {
	// GNU manual: exit status 3 if a write error occurs — it takes
	// precedence over the not-a-terminal status 1.
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: failWriter{}, Err: &errb},
	}
	code := cmd.Run(rc, nil)
	if code != 3 {
		t.Errorf("write error: code=%d, want 3", code)
	}
	if !strings.Contains(errb.String(), "tty: write error") {
		t.Errorf("write error diagnostic = %q, want it to name the write error", errb.String())
	}

	// -s writes nothing, so a broken stdout cannot fail: status is
	// still the plain not-a-terminal 1.
	errb.Reset()
	rc.Stdio = tool.Stdio{In: strings.NewReader(""), Out: failWriter{}, Err: &errb}
	if code := cmd.Run(rc, []string{"-s"}); code != 1 {
		t.Errorf("tty -s with broken stdout: code=%d, want 1", code)
	}
	if errb.Len() != 0 {
		t.Errorf("tty -s with broken stdout: stderr=%q, want empty", errb.String())
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

func TestTTYVersion(t *testing.T) {
	for _, flag := range []string{"--version", "-V"} {
		out, _, code := runTool(t, strings.NewReader(""), flag)
		if code != 0 || !strings.Contains(out, "tty") || !strings.Contains(out, "coreutils") {
			t.Errorf("%s: code=%d out=%q, want a 'tty ... coreutils' line and 0", flag, code, out)
		}
	}
}
