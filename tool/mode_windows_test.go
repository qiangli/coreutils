//go:build windows

package tool

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestLstatNamedPipeTracksServerLifetime proves the non-connecting contract
// used for a process-substitution operand. A name is visible only while a
// server instance owns it; reporting a closed name as a pipe would turn a
// stale process-substitution path into a fictitious file.
func TestLstatNamedPipeTracksServerLifetime(t *testing.T) {
	name := fmt.Sprintf("coreutils-lstat-%d-%d", windows.GetCurrentProcessId(), time.Now().UnixNano())
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

	shellPath := `//./pipe/` + name
	fi, err := Lstat(shellPath)
	if err != nil {
		windows.CloseHandle(h)
		t.Fatalf("Lstat(%q) while server is live: %v", shellPath, err)
	}
	if fi.Name() != name || fi.Mode()&fs.ModeNamedPipe == 0 {
		windows.CloseHandle(h)
		t.Fatalf("Lstat(%q) = name %q mode %v, want live named pipe", shellPath, fi.Name(), fi.Mode())
	}
	// Stat must not connect either: diff stats each process-substitution
	// operand before it opens the body, and this server offers one instance.
	fi, err = Stat(shellPath)
	if err != nil || fi.Name() != name || fi.Mode()&fs.ModeNamedPipe == 0 {
		windows.CloseHandle(h)
		t.Fatalf("Stat(%q) = %v, %v; want live named pipe", shellPath, fi, err)
	}
	client, err := windows.CreateFile(name16, windows.GENERIC_WRITE, 0, nil,
		windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		windows.CloseHandle(h)
		t.Fatalf("Stat(%q) consumed the sole pipe instance: %v", shellPath, err)
	}
	windows.CloseHandle(client)
	if err := windows.CloseHandle(h); err != nil {
		t.Fatal(err)
	}

	if _, err := Lstat(shellPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Lstat(%q) after final server close = %v, want not exist", shellPath, err)
	}
	if _, err := Stat(shellPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Stat(%q) after final server close = %v, want not exist", shellPath, err)
	}
}
