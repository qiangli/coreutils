//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package mvcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestMvEXDEVPreservesSymlinkAttributes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.Symlink("target", src); err != nil {
		t.Fatal(err)
	}
	wantTime := time.Unix(1_600_000_000, 123_000_000)
	tv := []unix.Timeval{unix.NsecToTimeval(wantTime.UnixNano()), unix.NsecToTimeval(wantTime.UnixNano())}
	if err := unix.Lutimes(src, tv); err != nil {
		t.Skipf("setting symlink timestamps is unsupported: %v", err)
	}
	// BSDs which expose symlink modes can make this observably non-default;
	// Linux reports EOPNOTSUPP and correctly retains its fixed 0777 mode.
	if err := unix.Fchmodat(unix.AT_FDCWD, src, 0o751, unix.AT_SYMLINK_NOFOLLOW); err != nil && runtime.GOOS != "linux" {
		t.Fatalf("setting supported symlink mode: %v", err)
	}
	before, err := os.Lstat(src)
	if err != nil {
		t.Fatal(err)
	}
	beforeStat, ok := before.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("symlink ownership metadata unavailable")
	}

	_, errb, code := runToolInputDeps(t, dir, "", exdevDeps(), "src", "dst")
	if code != 0 || errb != "" {
		t.Fatalf("EXDEV symlink move = (_, %q, %d)", errb, code)
	}
	after, err := os.Lstat(filepath.Join(dir, "dst"))
	if err != nil {
		t.Fatal(err)
	}
	afterStat, ok := after.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("destination symlink ownership metadata unavailable")
	}
	if beforeStat.Uid != afterStat.Uid || beforeStat.Gid != afterStat.Gid {
		t.Fatalf("symlink owner = %d:%d, want %d:%d", afterStat.Uid, afterStat.Gid, beforeStat.Uid, beforeStat.Gid)
	}
	if before.Mode().Perm() != after.Mode().Perm() {
		t.Fatalf("symlink mode = %o, want %o", after.Mode().Perm(), before.Mode().Perm())
	}
	if delta := after.ModTime().Sub(before.ModTime()); delta < -time.Millisecond || delta > time.Millisecond {
		t.Fatalf("symlink mtime = %v, want %v", after.ModTime(), before.ModTime())
	}
}
