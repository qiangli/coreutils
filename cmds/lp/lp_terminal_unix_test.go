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
	// Darwin PTYs do not support SetReadDeadline. Bound the read in the
	// test instead; closing master also releases a blocked read on failure.
	received := make(chan struct {
		text string
		err  error
	}, 1)
	go func() {
		buf := make([]byte, len(message))
		_, err := io.ReadFull(master, buf)
		received <- struct {
			text string
			err  error
		}{string(buf), err}
	}()
	select {
	case result := <-received:
		if result.err != nil || result.text != message {
			t.Fatalf("terminal received %q, error %v", result.text, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("terminal did not receive completion message")
	}
}
