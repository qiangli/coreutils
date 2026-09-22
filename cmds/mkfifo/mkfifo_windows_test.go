//go:build windows

package mkfifocmd

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/windows"
)

// These tests round-trip the docs/windows-fifo.md contract inside coreutils:
// mkfifo writes the marker, and the opener protocol the shell will implement
// from that document (readers are pipe servers, writers are clients) is
// exercised here directly against the created FIFO.

// createFIFO runs the applet and returns the marker path and its pipe leaf.
func createFIFO(t *testing.T, name string) (path, leaf string) {
	t.Helper()
	dir := t.TempDir()
	out, errb, code := runTool(t, dir, name)
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("mkfifo %s: code=%d out=%q err=%q", name, code, out, errb)
	}
	path = filepath.Join(dir, name)
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.Mode().IsRegular() || fi.Size() > MaxMarkerSize {
		t.Fatalf("marker mode=%v size=%d, want small regular file", fi.Mode(), fi.Size())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	leaf, ok := ParseMarker(content)
	if !ok {
		t.Fatalf("marker content does not parse: %q", content)
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		t.Fatal(err)
	}
	if attrs&windows.FILE_ATTRIBUTE_SYSTEM == 0 {
		t.Fatalf("marker lacks FILE_ATTRIBUTE_SYSTEM (attrs %#x)", attrs)
	}
	return path, leaf
}

// listen creates one reader-side pipe instance per docs/windows-fifo.md.
func listen(t *testing.T, leaf string) windows.Handle {
	t.Helper()
	name16, err := windows.UTF16PtrFromString(PipePath(leaf))
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateNamedPipe(name16,
		windows.PIPE_ACCESS_INBOUND,
		windows.PIPE_TYPE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS,
		windows.PIPE_UNLIMITED_INSTANCES, 65536, 65536, 0, nil)
	if err != nil {
		t.Fatalf("CreateNamedPipe(%s): %v", PipePath(leaf), err)
	}
	return h
}

// dialWriter opens the writer side with the documented retry loop.
func dialWriter(t *testing.T, leaf string) *os.File {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		f, err := os.OpenFile(PipePath(leaf), os.O_WRONLY, 0)
		if err == nil {
			return f
		}
		if time.Now().After(deadline) {
			t.Fatalf("writer could not connect to %s: %v", PipePath(leaf), err)
		}
		// ERROR_FILE_NOT_FOUND: no reader listening yet. ERROR_PIPE_BUSY:
		// instances exist but are taken. Both mean wait and retry.
		time.Sleep(10 * time.Millisecond)
	}
}

// readAll drains a server instance; a broken pipe is the documented EOF.
func readAll(t *testing.T, h windows.Handle, leaf string) string {
	t.Helper()
	if err := windows.ConnectNamedPipe(h, nil); err != nil && err != windows.ERROR_PIPE_CONNECTED {
		t.Fatalf("ConnectNamedPipe: %v", err)
	}
	f := os.NewFile(uintptr(h), PipePath(leaf))
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil && !tool.IsClosedPipeError(err) {
		t.Fatalf("read: %v", err)
	}
	return string(data)
}

// The builtins fixture's shape: the writer is started first (echo … > fifo &),
// the reader arrives second (. fifo). The writer must block until the reader
// creates an instance, then the bytes must flow.
func TestMkfifoConnectWriterFirst(t *testing.T) {
	_, leaf := createFIFO(t, "fifo-writer-first")
	const payload = "echo four - OK\n"
	done := make(chan error, 1)
	go func() {
		w := dialWriter(t, leaf)
		_, err := w.WriteString(payload)
		if cerr := w.Close(); err == nil {
			err = cerr
		}
		done <- err
	}()
	// Give the writer a head start so the retry loop is actually exercised.
	time.Sleep(50 * time.Millisecond)
	h := listen(t, leaf)
	if got := readAll(t, h, leaf); got != payload {
		t.Fatalf("read %q, want %q", got, payload)
	}
	if err := <-done; err != nil {
		t.Fatalf("writer: %v", err)
	}
}

// The reverse order: a reader blocks in ConnectNamedPipe until a writer opens.
func TestMkfifoConnectReaderFirst(t *testing.T) {
	_, leaf := createFIFO(t, "fifo-reader-first")
	const payload = "hello through the fifo\n"
	h := listen(t, leaf)
	got := make(chan string, 1)
	go func() { got <- readAll(t, h, leaf) }()
	w := dialWriter(t, leaf)
	if _, err := w.WriteString(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case data := <-got:
		if data != payload {
			t.Fatalf("read %q, want %q", data, payload)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("reader did not complete")
	}
}

// The read2 fixture's shape (exec 9<> a.pipe): a read-write open is the pair
// of one reader instance and one self-connected writer client; bytes written
// on the write half come back on the read half without any other peer.
func TestMkfifoReadWriteSelfPair(t *testing.T) {
	_, leaf := createFIFO(t, "a.pipe")
	h := listen(t, leaf)
	w := dialWriter(t, leaf)
	if err := windows.ConnectNamedPipe(h, nil); err != nil && err != windows.ERROR_PIPE_CONNECTED {
		t.Fatalf("ConnectNamedPipe: %v", err)
	}
	r := os.NewFile(uintptr(h), PipePath(leaf))
	defer r.Close()
	const payload = "written and read back by the same fd\n"
	if _, err := w.WriteString(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(buf) != payload {
		t.Fatalf("read back %q, want %q", buf, payload)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
}

// Unlink is DeleteFile on the marker; connected ends keep working, and a
// recreated FIFO at the same path draws a distinct pipe leaf.
func TestMkfifoUnlinkAndRecreate(t *testing.T) {
	path, leaf := createFIFO(t, "fifo-unlink")
	h := listen(t, leaf)
	got := make(chan string, 1)
	go func() { got <- readAll(t, h, leaf) }()
	w := dialWriter(t, leaf)
	if err := os.Remove(path); err != nil {
		t.Fatalf("unlink marker: %v", err)
	}
	const payload = "still flowing after unlink\n"
	if _, err := w.WriteString(payload); err != nil {
		t.Fatalf("write after unlink: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if data := <-got; data != payload {
		t.Fatalf("read %q, want %q", data, payload)
	}

	dir := filepath.Dir(path)
	out, errb, code := runTool(t, dir, "fifo-unlink")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("recreate: code=%d out=%q err=%q", code, out, errb)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	leaf2, ok := ParseMarker(content)
	if !ok {
		t.Fatalf("recreated marker does not parse: %q", content)
	}
	if leaf2 == leaf {
		t.Fatalf("recreated FIFO reused leaf %q", leaf)
	}
}
