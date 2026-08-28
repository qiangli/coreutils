// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !windows

package posixproviderscmd

import (
	"fmt"
	"os"
	"syscall"

	"github.com/qiangli/coreutils/tool"
)

// execProviderDedicated replaces a standalone multicall process with the
// provider.  Keeping the advertised utility PID is observable POSIX behavior:
// a signal sent to that PID must reach the utility, not kill a Go waiter and
// leave the real provider running against inherited stdio.
//
// Embedded callers cannot self-replace, and test/custom contexts can carry
// non-file streams, so those continue through the fork-and-wait path.
func execProviderDedicated(rc *tool.RunContext, name, path string, args []string) (bool, int) {
	if !rc.DedicatedProcess || !rc.DirIsProcessCwd || !processStdio(rc) {
		return false, 0
	}
	argv := append([]string{name}, args...)
	if err := syscall.Exec(path, argv, rc.Env); err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", name, err)
		return true, 126
	}
	return true, 0 // unreachable after a successful execve
}

func processStdio(rc *tool.RunContext) bool {
	in, inOK := rc.In.(*os.File)
	out, outOK := rc.Out.(*os.File)
	errOut, errOK := rc.Err.(*os.File)
	return inOK && outOK && errOK && in.Fd() == os.Stdin.Fd() &&
		out.Fd() == os.Stdout.Fd() && errOut.Fd() == os.Stderr.Fd()
}
