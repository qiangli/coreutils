//go:build linux || darwin

package talkcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty/v2"
	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

func ptyTermios(t *testing.T, fd int) *unix.Termios {
	t.Helper()
	state, err := ioctlTermiosForTest(fd)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestTalkPTYUsesCharacterModeAndRestoresTerminal(t *testing.T) {
	ptm, tty, err := pty.Open()
	if err != nil {
		t.Skipf("cannot allocate pty: %v", err)
	}
	defer ptm.Close()
	defer tty.Close()
	if err := pty.Setsize(tty, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatal(err)
	}
	before := ptyTermios(t, int(tty.Fd()))
	rc := &tool.RunContext{Ctx: context.Background(), Env: []string{"TERM=xterm", "LC_ALL=C.UTF-8"},
		Stdio: tool.Stdio{In: tty, Out: tty, Err: &bytes.Buffer{}}}
	display, err := newScreenDisplay(rc, "alice", "bob")
	if err != nil {
		t.Fatal(err)
	}
	during := ptyTermios(t, int(tty.Fd()))
	if during.Lflag&(unix.ICANON|unix.ECHO) != 0 {
		t.Fatalf("terminal is not character-at-a-time/no-echo: lflag=%#x", during.Lflag)
	}
	if changed := uint64(before.Lflag ^ during.Lflag); changed & ^uint64(unix.ICANON|unix.ECHO|unix.ECHONL|unix.ISIG) != 0 {
		t.Fatalf("character mode changed unrelated local flags: before=%#x during=%#x", before.Lflag, during.Lflag)
	}
	if during.Iflag != before.Iflag || during.Oflag != before.Oflag || during.Cflag != before.Cflag {
		t.Fatalf("character mode changed unrelated termios words: before=%+v during=%+v", before, during)
	}
	input, err := newTerminalInput(tty)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ptm.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	event, ready := input.Poll(context.Background(), time.Second)
	if !ready || event.err != nil || string(event.data) != "x" {
		t.Fatalf("character was not immediately available: ready=%v event=%q err=%v", ready, event.data, event.err)
	}
	wires, terminate, err := display.Local(event.data, true)
	if err != nil || terminate || len(wires) != 1 {
		t.Fatalf("character event wires=%q terminate=%v err=%v", wires, terminate, err)
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	if err := display.Close(); err != nil {
		t.Fatal(err)
	}
	after := ptyTermios(t, int(tty.Fd()))
	if after.Lflag != before.Lflag || after.Iflag != before.Iflag || after.Oflag != before.Oflag || after.Cflag != before.Cflag {
		t.Fatalf("terminal state not restored: before=%+v after=%+v", before, after)
	}
}

func TestTalkTerminalCapabilityGateFailsClosed(t *testing.T) {
	ptm, tty, err := pty.Open()
	if err != nil {
		t.Skipf("cannot allocate pty: %v", err)
	}
	defer ptm.Close()
	defer tty.Close()
	if err := pty.Setsize(tty, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatal(err)
	}
	for _, termName := range []string{"dumb", "definitely-not-a-terminal"} {
		rc := &tool.RunContext{Env: []string{"TERM=" + termName}, Stdio: tool.Stdio{In: tty, Out: tty, Err: &bytes.Buffer{}}}
		if err := checkTerminalCapabilities(rc); err == nil {
			t.Errorf("TERM=%s unexpectedly passed capability gate", termName)
		}
	}
	if err := pty.Setsize(tty, &pty.Winsize{Rows: 5, Cols: 19}); err != nil {
		t.Fatal(err)
	}
	rc := &tool.RunContext{Env: []string{"TERM=xterm"}, Stdio: tool.Stdio{In: tty, Out: tty, Err: &bytes.Buffer{}}}
	if err := checkTerminalCapabilities(rc); err == nil || !strings.Contains(err.Error(), "too small") {
		t.Fatalf("undersized terminal gate=%v", err)
	}
}
