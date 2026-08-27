//go:build unix

package talkcmd

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"github.com/qiangli/coreutils/cmds/internal/session"
)

func currentOSAccount() (account, error) {
	uid := strconv.Itoa(os.Geteuid())
	u, err := user.LookupId(uid)
	if err != nil {
		return account{}, err
	}
	return account{Name: u.Username, UID: uid}, nil
}

func defaultTalkRoot() string {
	// macOS exposes /tmp as a symlink to /private/tmp. Resolve the system-wide
	// sticky directory before the no-symlink ownership validation; TMPDIR is
	// intentionally not used because it is normally private to one login.
	root, err := filepath.EvalSymlinks("/tmp")
	if err == nil {
		return root
	}
	return "/tmp"
}

func lookupOSAccount(name string) (account, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return account{}, err
	}
	if _, err := strconv.ParseUint(u.Uid, 10, 32); err != nil {
		return account{}, fmt.Errorf("account %s has non-numeric uid", name)
	}
	return account{Name: u.Username, UID: u.Uid}, nil
}

func notifyTerminal(record session.Record, peer account, text string) error {
	path := session.TTYPath(record.TTY)
	fd, err := unix.Open(path, unix.O_WRONLY|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		_ = unix.Close(fd)
		return errors.New("cannot own recipient terminal descriptor")
	}
	defer f.Close()
	if !term.IsTerminal(fd) {
		return errors.New("recipient device is not a terminal")
	}
	info, err := f.Stat()
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || strconv.FormatUint(uint64(st.Uid), 10) != peer.UID {
		return errors.New("recipient terminal ownership changed before notification")
	}
	return writeBytes(f, []byte(text))
}

func fileOwnerUID(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", errors.New("file owner uid is unavailable")
	}
	return strconv.FormatUint(uint64(st.Uid), 10), nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := unix.Kill(pid, 0)
	return err == nil || errors.Is(err, unix.EPERM)
}
