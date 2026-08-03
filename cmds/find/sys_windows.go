//go:build windows

package findcmd

import (
	"io/fs"
	"os"
)

// Windows has no unix uid/gid/nlink/device identity on FileInfo;
// the parser rejects the predicates that need them with a clear
// unsupported error before evaluation can reach these stubs.
const (
	haveSysStat = false
	haveDev     = false
)

func fileUID(fs.FileInfo) (uint32, bool)   { return 0, false }
func fileGID(fs.FileInfo) (uint32, bool)   { return 0, false }
func fileNlink(fs.FileInfo) (uint64, bool) { return 0, false }
func fileDev(fs.FileInfo) (uint64, bool)   { return 0, false }

// defaultCommandPath: Windows has no confstr(_CS_PATH). With PATH unset
// the -exec search falls back to the working directory (an empty list is
// one zero-length element per commandSearchPath).
func defaultCommandPath() string { return "" }

// shellPath / isExecFormatError: the ENOEXEC-through-the-shell retry is a
// POSIX execvp behavior. Windows has no /bin/sh and no such fallback, so
// there is nothing to retry.
func shellPath() string            { return "" }
func isExecFormatError(error) bool { return false }

// signaledExitCode: Windows has no POSIX signal wait status, so a child's
// termination is always reported through its exit code, never here.
func signaledExitCode(*os.ProcessState) (int, bool) { return 0, false }
