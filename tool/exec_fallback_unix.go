//go:build !windows

package tool

import (
	"errors"
	"syscall"
)

func commandShellFallback(path string, args []string, err error) (string, []string, bool) {
	if !errors.Is(err, syscall.ENOEXEC) {
		return "", nil, false
	}
	shellArgs := make([]string, 1, len(args)+1)
	shellArgs[0] = path
	shellArgs = append(shellArgs, args...)
	return "/bin/sh", shellArgs, true
}
