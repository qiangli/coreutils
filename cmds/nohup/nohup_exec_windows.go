//go:build windows

package nohupcmd

import (
	"context"
	"os/exec"
)

func newNohupCommand(ctx context.Context, path string, args, env []string) *exec.Cmd {
	if ctx == nil {
		ctx = context.Background()
	}
	c := exec.CommandContext(ctx, path, args...)
	if env == nil {
		c.Env = []string{}
	} else {
		c.Env = env
	}
	return c
}
