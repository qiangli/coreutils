//go:build unix

package lpcmd

import (
	"io"
	"testing"
	"time"

	"github.com/creack/pty/v2"
	"golang.org/x/term"
)

func TestTerminalDeviceDelivery(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer master.Close()
	defer slave.Close()
	state, err := term.MakeRaw(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	defer term.Restore(int(slave.Fd()), state)
	message := "request office-42 completed\n"
	if err := writeTerminal(slave.Name(), message); err != nil {
		t.Fatal(err)
	}
	if err := master.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(message))
	_, err = io.ReadFull(master, buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf) != message {
		t.Fatalf("terminal received %q", buf)
	}
}
