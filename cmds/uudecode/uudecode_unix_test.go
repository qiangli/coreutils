//go:build darwin || linux || freebsd || netbsd || openbsd

package uudecodecmd

import (
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

func TestOverwriteFIFODoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipe")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _, _, _ = runTool(t, dir, "begin 600 pipe\n \nend\n"); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("uudecode blocked while checking FIFO overwrite")
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeNamedPipe != 0 {
		t.Fatalf("FIFO was not atomically replaced: mode=%v", info.Mode())
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
