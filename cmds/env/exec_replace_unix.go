//go:build !windows

package envcmd

import (
	"os"
	"syscall"

	"github.com/qiangli/coreutils/tool"
)

// replaceCommand overlays a dedicated standalone env process with COMMAND.
// Upstream env is an exec utility; retaining the PID is observable to signal
// senders and prevents a killed wrapper from orphaning COMMAND.  Tool.Run in a
// long-lived embedding host deliberately leaves DedicatedProcess false and
// therefore uses the fork/wait path in runCommand instead.
func replaceCommand(rc *tool.RunContext, path string, argv, env []string, argv0 string, signals []os.Signal) (bool, error) {
	if !rc.DedicatedProcess || !rc.DirIsProcessCwd || !processStdio(rc) {
		return false, nil
	}

	if rc.Dir != "" {
		if err := os.Chdir(rc.Dir); err != nil {
			return true, err
		}
	}

	// execvp resolves argv[0] through PATH but preserves the spelling supplied
	// by the caller. Only env's explicit --argv0 option replaces that value.
	execArgv := append([]string{argv[0]}, argv[1:]...)
	if argv0 != "" {
		execArgv[0] = argv0
	}
	restoreSignals := ignoreForCommandStart(signals)
	defer restoreSignals()
	if err := syscall.Exec(path, execArgv, env); !isExecFormatError(err) {
		return true, err
	}

	// Match execvp's historical ENOEXEC fallback for executable text without
	// a shebang.  As in the fork/wait path, an argv0 override does not survive
	// the shell retry.
	shellArgv := append([]string{scriptInterpreter, path}, argv[1:]...)
	return true, syscall.Exec(scriptInterpreter, shellArgv, env)
}

// processStdio proves that replacing this process preserves the invocation's
// streams exactly. A crafted RunContext with buffers, pipes represented by
// wrappers, or arbitrary *os.File values must stay on the safe fork/wait path
// even if it incorrectly claims to be dedicated.
func processStdio(rc *tool.RunContext) bool {
	in, inOK := rc.In.(*os.File)
	out, outOK := rc.Out.(*os.File)
	errOut, errOK := rc.Err.(*os.File)
	return inOK && outOK && errOK && in.Fd() == os.Stdin.Fd() &&
		out.Fd() == os.Stdout.Fd() && errOut.Fd() == os.Stderr.Fd()
}
