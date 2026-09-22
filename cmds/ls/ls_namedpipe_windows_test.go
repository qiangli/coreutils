//go:build windows

package lscmd

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// A long listing must inspect a process-substitution pipe without becoming
// its first client. The shell keeps serving the same name for a second ls;
// an ACL lookup on the path used to connect and consume that first client.
func TestLongListingDoesNotConnectNamedPipe(t *testing.T) {
	name := fmt.Sprintf("ls-procsub-%d-%d", windows.GetCurrentProcessId(), time.Now().UnixNano())
	native := `\\.\pipe\` + name
	name16, err := windows.UTF16PtrFromString(native)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateNamedPipe(name16,
		windows.PIPE_ACCESS_INBOUND|windows.FILE_FLAG_FIRST_PIPE_INSTANCE,
		windows.PIPE_TYPE_BYTE|windows.PIPE_WAIT,
		1, 4096, 4096, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	connected := make(chan error, 1)
	go func() { connected <- windows.ConnectNamedPipe(h, nil) }()

	shellPath := "//./pipe/" + name
	for _, flag := range []string{"-l", "-al"} {
		_, stderr, code := runToolAt(t, t.TempDir(), flag, shellPath)
		if code != 0 || stderr != "" {
			t.Fatalf("ls %s on live pipe: code=%d stderr=%q", flag, code, stderr)
		}
		select {
		case err := <-connected:
			t.Fatalf("ls %s connected to the pipe: %v", flag, err)
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Prove the listener was actually waiting: a deliberate client must now
	// complete ConnectNamedPipe, which neither long listing did.
	client, err := windows.CreateFile(name16, windows.GENERIC_WRITE, 0, nil,
		windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatalf("deliberate client connect: %v", err)
	}
	windows.CloseHandle(client)
	select {
	case err := <-connected:
		if err != nil && err != windows.ERROR_PIPE_CONNECTED {
			t.Fatalf("ConnectNamedPipe: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("deliberate client did not complete ConnectNamedPipe")
	}
}
