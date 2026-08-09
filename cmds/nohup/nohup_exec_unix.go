//go:build unix

package nohupcmd

import (
	"context"
	"os/exec"
)

// newNohupCommand changes SIGHUP's disposition in a short-lived child shell,
// never in the embedding process. POSIX preserves ignored signals across
// exec, so the requested command inherits SIGHUP ignored. Having the shell
// perform exec also provides the POSIX ENOEXEC fallback for executable text
// files without a shebang.
func newNohupCommand(ctx context.Context, path string, args, env []string) *exec.Cmd {
	if ctx == nil {
		ctx = context.Background()
	}
	shellArgs := make([]string, 0, len(args)+4)
	shellArgs = append(shellArgs, "-c", "trap '' HUP; exec \"$@\"", "nohup", path)
	shellArgs = append(shellArgs, args...)
	c := exec.CommandContext(ctx, "/bin/sh", shellArgs...)
	if env == nil {
		c.Env = []string{}
	} else {
		c.Env = env
	}
	return c
}
