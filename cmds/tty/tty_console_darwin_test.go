//go:build darwin

package ttycmd

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// TestTTYDevConsole reproduces a real bug: /dev/console is a terminal
// whose device name does not begin with "tty". A /dev scan limited to
// tty-prefixed entries misses it and reports "not a tty" (exit 1) even
// though the termios ioctl confirms the fd is a terminal.
//
// The terminal check uses the ioctl directly because ttyName's own
// result is the subject under test — it cannot be the oracle.
func TestTTYDevConsole(t *testing.T) {
	f, err := os.Open("/dev/console")
	if err != nil {
		t.Skipf("cannot open /dev/console: %v", err)
	}
	defer f.Close()
	if _, err := unix.IoctlGetTermios(int(f.Fd()), unix.TIOCGETA); err != nil {
		t.Skip("/dev/console is not a terminal in this environment")
	}

	name, isTTY := ttyName(f)
	if !isTTY {
		t.Fatalf("ttyName(/dev/console) = (\"\", false); want the device name — " +
			"a terminal whose /dev entry does not begin with \"tty\" was " +
			"misreported as not-a-tty")
	}

	out, errb, code := runTool(t, f)
	if code != 0 || errb != "" {
		t.Fatalf("tty </dev/console = code %d, stderr %q, want 0 with no stderr", code, errb)
	}
	if got, want := out, name+"\n"; got != want {
		t.Errorf("tty </dev/console printed %q, want %q", got, want)
	}
}
