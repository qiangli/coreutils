package tool

import (
	"context"
	"io"
	"os/exec"
)

// StartCommand starts an external command with the invocation's context,
// working directory, and environment. On Unix, an executable text file that
// the kernel rejects with ENOEXEC is retried through /bin/sh, matching the
// execvp(3) fallback used by POSIX command wrappers.
//
// stdin/stdout/stderr are explicit because wrappers such as xargs deliberately
// give the child a null stdin and may capture parallel output. A nil rc.Env is
// an empty invocation environment, not permission to inherit the host process
// environment.
func (rc *RunContext) StartCommand(path string, args []string, stdin io.Reader, stdout, stderr io.Writer) (*exec.Cmd, error) {
	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	start := func(command string, commandArgs []string) (*exec.Cmd, error) {
		c := exec.CommandContext(ctx, command, commandArgs...)
		c.Dir = rc.Dir
		if rc.Env == nil {
			c.Env = []string{}
		} else {
			c.Env = rc.Env
		}
		c.Stdin = stdin
		c.Stdout = stdout
		c.Stderr = stderr
		return c, c.Start()
	}

	c, err := start(path, args)
	if shell, shellArgs, ok := commandShellFallback(path, args, err); ok {
		return start(shell, shellArgs)
	}
	return c, err
}
