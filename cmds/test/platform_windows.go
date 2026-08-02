//go:build windows

package testcmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/qiangli/coreutils/tool"
)

// defaultPathExt is what Windows itself assumes when PATHEXT is unset.
const defaultPathExt = ".COM;.EXE;.BAT;.CMD"

// errNoOwnership is the loud answer for -O/-G on a platform with no
// POSIX uid/gid: reporting false would be a silent wrong answer, since
// the file may very well be owned by the caller.
var errNoOwnership = errors.New("file ownership tests need POSIX uid/gid, which windows does not have")

func ownedByEffective(string, bool) (bool, error) { return false, errNoOwnership }

// accessOK answers -r/-w/-x with the Windows notions of the three
// permissions: everything that can be stat'ed can be read; writability
// is the read-only attribute (which Go surfaces as the 0200 mode bit);
// and executability is directory search, or an extension listed in
// PATHEXT — the same rule RunContext.ResolveExecutable applies, and the
// rule Windows uses to decide what it will run.
func accessOK(rc *tool.RunContext, path string, op byte) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, nil
	}
	switch op {
	case 'r':
		return true, nil
	case 'w':
		return fi.Mode()&0200 != 0, nil
	default: // 'x'
		if fi.IsDir() {
			return true, nil
		}
		ext := filepath.Ext(path)
		if ext == "" {
			return false, nil
		}
		pathExt := rc.Getenv("PATHEXT")
		if pathExt == "" {
			pathExt = defaultPathExt
		}
		for e := range strings.SplitSeq(pathExt, ";") {
			if e != "" && strings.EqualFold(strings.TrimSpace(e), ext) {
				return true, nil
			}
		}
		return false, nil
	}
}

// fileTimes returns the access and modification times for -N.
func fileTimes(path string) (atime, mtime time.Time, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, d.LastAccessTime.Nanoseconds()), fi.ModTime(), nil
	}
	return fi.ModTime(), fi.ModTime(), nil
}

// isTerminal reports whether f is a console handle — the Windows
// terminal, and what -t has to mean here.
func isTerminal(f *os.File) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil
}
