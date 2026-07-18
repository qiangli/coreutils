//go:build linux || darwin

package ttycmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/creack/pty/v2"

	"github.com/qiangli/coreutils/tool"
)

// openTTY returns the terminal side of a fresh pseudo-terminal pair,
// skipping when the environment cannot allocate one.
func openTTY(t *testing.T) *os.File {
	t.Helper()
	ptm, tty, err := pty.Open()
	if err != nil {
		t.Skipf("cannot open pty: %v", err)
	}
	t.Cleanup(func() { ptm.Close(); tty.Close() })
	return tty
}

func TestTTYTerminalName(t *testing.T) {
	tty := openTTY(t)
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: tty, Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, nil)
	if code != 0 || errb.Len() != 0 {
		t.Fatalf("tty on a pty = code %d, stderr %q, want 0 with no stderr", code, errb.String())
	}
	if got, want := out.String(), tty.Name()+"\n"; got != want {
		t.Errorf("tty printed %q, want %q", got, want)
	}
}

func TestTTYTerminalSilent(t *testing.T) {
	tty := openTTY(t)
	for _, flag := range []string{"-s", "--silent", "--quiet"} {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Stdio: tool.Stdio{In: tty, Out: &out, Err: &errb},
		}
		code := cmd.Run(rc, []string{flag})
		if code != 0 || out.Len() != 0 || errb.Len() != 0 {
			t.Errorf("tty %s on a pty = (%q, %q, %d), want silent success", flag, out.String(), errb.String(), code)
		}
	}
}

func TestTTYTerminalWriteError(t *testing.T) {
	// Even when stdin IS a terminal, a stdout write error is exit
	// status 3 (GNU manual), not the success status 0.
	tty := openTTY(t)
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: tty, Out: failWriter{}, Err: &errb},
	}
	code := cmd.Run(rc, nil)
	if code != 3 {
		t.Errorf("tty on a pty with broken stdout: code=%d, want 3", code)
	}
	if !strings.Contains(errb.String(), "tty: write error") {
		t.Errorf("diagnostic = %q, want a tty: write error line", errb.String())
	}
}
