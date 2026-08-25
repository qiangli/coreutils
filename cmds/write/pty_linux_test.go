//go:build linux

package writecmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func openPTYLinux() (*os.File, *os.File, string, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, "", err
	}
	if err := unix.IoctlSetInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		master.Close()
		return nil, nil, "", err
	}
	ptyNum, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		master.Close()
		return nil, nil, "", err
	}
	slavePath := fmt.Sprintf("/dev/pts/%d", ptyNum)
	slave, err := os.OpenFile(slavePath, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, nil, "", err
	}
	return master, slave, slavePath, nil
}

func TestPTYBackedWriteLinux(t *testing.T) {
	master, slave, slavePath, err := openPTYLinux()
	if err != nil {
		t.Skipf("PTY creation skipped: %v", err)
	}
	defer master.Close()
	defer slave.Close()

	relLine := strings.TrimPrefix(slavePath, "/dev/")
	w := install(t, fixture{
		sender: "alice", uid: 1000, myTTY: "pts/999",
		logins: []login{{user: "bob", line: relLine, mode: 0o620, when: epoch}},
	})

	oldOpen := openTTYFn
	openTTYFn = func(path string) (io.WriteCloser, error) {
		return slave, nil
	}
	defer func() { openTTYFn = oldOpen }()

	out, errOut, code := exec(t, "pty test\n", "bob")
	if code != 0 {
		t.Fatalf("exit code %d: %s", code, errOut)
	}
	if out != "" {
		t.Errorf("stdout must be empty, got %q", out)
	}

	buf := make([]byte, 1024)
	_ = master.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	n, _ := master.Read(buf)
	got := string(buf[:n])
	if !strings.Contains(got, "Message from alice (pts/999)") || !strings.Contains(got, "pty test\r\n") {
		t.Errorf("master PTY received: %q", got)
	}
	_ = w
}

func TestPTYCanonicalVEOLDiscoveryLinux(t *testing.T) {
	master, slave, _, err := openPTYLinux()
	if err != nil {
		t.Skipf("PTY creation skipped: %v", err)
	}
	defer master.Close()
	defer slave.Close()
	term, err := getTermios(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	const veol = byte(0x1d)
	term.Cc[unix.VEOL] = veol
	if err := unix.IoctlSetTermios(int(slave.Fd()), unix.TCSETS, term); err != nil {
		t.Fatal(err)
	}
	if got := defaultGetVEOL(slave); got != veol {
		t.Fatalf("defaultGetVEOL(real PTY) = %#x, want %#x", got, veol)
	}
}
