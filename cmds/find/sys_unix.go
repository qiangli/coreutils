//go:build unix

package findcmd

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// Predicates backed by the unix stat structure (-user/-group/-nouser/
// -nogroup/-links/-xdev) are supported wherever Sys() is a *Stat_t.
const (
	haveSysStat = true
	haveDev     = true
)

// defaultCommandPath is the -exec search path when PATH is unset (not
// merely empty): POSIX's confstr(_CS_PATH) default, matching env's.
func defaultCommandPath() string { return "/usr/bin:/bin" }

// shellPath is the interpreter used for the ENOEXEC retry (see runArgv):
// execvp runs a non-binary executable through _PATH_BSHELL, "/bin/sh".
func shellPath() string { return "/bin/sh" }

// isExecFormatError reports whether starting the utility failed with
// ENOEXEC — the file is present and executable but not a recognized
// binary (typically a shebang-less script), which POSIX execvp retries
// through the shell.
func isExecFormatError(err error) bool { return errors.Is(err, syscall.ENOEXEC) }

// signaledExitCode maps a child killed by a signal onto the POSIX 128+N
// wait-status convention. ok is false when the child was not signaled.
func signaledExitCode(ps *os.ProcessState) (code int, ok bool) {
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal()), true
	}
	return 0, false
}

func statOf(info fs.FileInfo) (*syscall.Stat_t, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	return st, ok
}

func fileUID(info fs.FileInfo) (uint32, bool) {
	st, ok := statOf(info)
	if !ok {
		return 0, false
	}
	return uint32(st.Uid), true
}

func fileGID(info fs.FileInfo) (uint32, bool) {
	st, ok := statOf(info)
	if !ok {
		return 0, false
	}
	return uint32(st.Gid), true
}

func fileNlink(info fs.FileInfo) (uint64, bool) {
	st, ok := statOf(info)
	if !ok {
		return 0, false
	}
	return uint64(st.Nlink), true
}

func fileDev(info fs.FileInfo) (uint64, bool) {
	st, ok := statOf(info)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true
}
