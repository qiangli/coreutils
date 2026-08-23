//go:build windows

package newgrpcmd

import (
	"errors"

	"github.com/qiangli/coreutils/tool"
)

// defaultShell exists only so the shared code compiles; nothing reaches a
// spawn on Windows.
const defaultShell = "cmd.exe"

// Windows has no POSIX group identification: a process token carries a SID list
// with no "current group" to change, and there is no setgid to fail. Starting a
// shell with the group "unchanged" — the POSIX fallback everywhere else — would
// therefore be a shell that never had a group in the first place, reported as
// if newgrp had done something. Refusing outright is the only truthful answer.
func defaultSpawnShell(*tool.RunContext, shellSpec) (int, error) {
	return 0, errors.New("changing group identification is not a Windows concept")
}

// readPassword is unreachable for the same reason — nothing on Windows can
// grant the group the password would unlock.
func readPassword(*tool.RunContext, string) (string, error) {
	return "", errors.New("group passwords are not a Windows concept")
}
