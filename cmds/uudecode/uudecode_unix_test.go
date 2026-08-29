//go:build darwin || linux || freebsd || netbsd || openbsd

package uudecodecmd

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestHeaderModeIgnoresUmask(t *testing.T) {
	old := unix.Umask(0o077)
	t.Cleanup(func() { unix.Umask(old) })
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "begin 666 out\n \nend\n")
	if code != 0 || errb != "" {
		t.Fatalf("got (%q,%d)", errb, code)
	}
	info, err := os.Stat(filepath.Join(dir, "out"))
	if err != nil || info.Mode().Perm() != 0o666 {
		t.Fatalf("mode=%v err=%v", info.Mode().Perm(), err)
	}
	got := unix.Umask(0)
	unix.Umask(got)
	if got != 0o077 {
		t.Fatalf("uudecode changed process umask to %03o", got)
	}
}

func TestDecodeExistingFIFOPreservesFileObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.fifo")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}

	type readResult struct {
		data []byte
		err  error
	}
	readerStarted := make(chan struct{})
	readDone := make(chan readResult, 1)
	go func() {
		close(readerStarted)
		file, err := os.Open(path)
		if err != nil {
			readDone <- readResult{err: err}
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		readDone <- readResult{data: data, err: err}
	}()
	<-readerStarted

	decodeDone := make(chan struct {
		errb string
		code int
	}, 1)
	go func() {
		_, errb, code := runTool(t, dir, "begin-base64 640 output.fifo\nRG9uZQ==\n====\n")
		decodeDone <- struct {
			errb string
			code int
		}{errb, code}
	}()

	select {
	case got := <-decodeDone:
		if got.code != 0 || got.errb != "" {
			t.Fatalf("decode through FIFO: err=%q code=%d", got.errb, got.code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("decode through FIFO blocked")
	}
	select {
	case got := <-readDone:
		if got.err != nil || string(got.data) != "Done" {
			t.Fatalf("FIFO reader got %q, %v", got.data, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("FIFO reader blocked")
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("decoded output stopped being a FIFO: mode=%v", info.Mode())
	}
}

func TestOverwriteWritableFileDoesNotRequireParentWrite(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "parent")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "target")
	if err := os.WriteFile(target, []byte("old"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
	if err := unix.Faccessat(unix.AT_FDCWD, target, unix.W_OK, unix.AT_EACCESS); err != nil {
		t.Skipf("effective credentials cannot write fixture: %v", err)
	}
	_, errb, code := runTool(t, dir, "begin-base64 640 parent/target\nRG9uZQ==\n====\n")
	if code != 0 || errb != "" {
		t.Fatalf("err=%q code=%d", errb, code)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "Done" {
		t.Fatalf("content=%q err=%v", got, err)
	}
}

func TestDecodeFollowsFinalSymlinkWithoutReplacingIt(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", link); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "begin 640 link\n#0V%T\n \nend\n")
	if code != 0 || errb != "" {
		t.Fatalf("err=%q code=%d", errb, code)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link mode=%v", info.Mode())
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "Cat" {
		t.Fatalf("target = %q, %v", got, err)
	}
}

func TestDecodeCreatesDanglingFinalSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "missing")
	link := filepath.Join(dir, "link")
	if err := os.Symlink("missing", link); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "begin 640 link\n#0V%T\n \nend\n")
	if code != 0 || errb != "" {
		t.Fatalf("err=%q code=%d", errb, code)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link mode=%v", info.Mode())
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "Cat" {
		t.Fatalf("target = %q, %v", got, err)
	}
}

func TestDecodeRefusesResolvedTargetWithoutEffectiveWriteAccess(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("keep"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := unix.Faccessat(unix.AT_FDCWD, target, unix.W_OK, unix.AT_EACCESS); err == nil {
		t.Skip("effective credentials can write a mode-0400 file")
	}
	if err := os.Symlink("target", link); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "begin 600 link\n#0V%T\n \nend\n")
	if code == 0 || errb == "" {
		t.Fatalf("err=%q code=%d", errb, code)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "keep" {
		t.Fatalf("target = %q, %v", got, err)
	}
}
