//go:build unix

package newgrpcmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/term"
)

const defaultShell = "/bin/sh"

// defaultSpawnShell starts the replacement shell, optionally under a new group
// id, and returns its exit status.
//
// The credential is applied to the CHILD, never to this process. Two reasons,
// and the second is the load-bearing one: setgid(2) is process-global, and
// these tools run in-process inside an embedding shell, so changing the caller
// would silently re-credential every later command in that host — an effect
// that outlives the invocation and that nothing reports.
//
// NoSetGroups is set because the supplementary group list must NOT be rebuilt:
// setgroups(2) is privileged, and newgrp changes the REAL AND EFFECTIVE GROUP
// ID, leaving the supplementary set alone. Uid is carried through unchanged so
// the credential struct does not imply a user switch.
func defaultSpawnShell(rc *tool.RunContext, spec shellSpec) (int, error) {
	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	c := exec.CommandContext(ctx, spec.Path)
	c.Args = []string{spec.Argv0}
	c.Dir = spec.Dir
	if rc.Env == nil {
		c.Env = []string{}
	} else {
		c.Env = rc.Env
	}
	c.Stdin, c.Stdout, c.Stderr = rc.In, rc.Out, rc.Err

	if spec.GID != "" {
		gid, err := strconv.Atoi(spec.GID)
		if err != nil {
			return 0, &errGroupChange{fmt.Errorf("invalid group id %q", spec.GID)}
		}
		uid, err := strconv.Atoi(spec.UID)
		if err != nil {
			uid = os.Getuid()
		}
		c.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid:         uint32(uid),
				Gid:         uint32(gid),
				NoSetGroups: true,
			},
		}
	}

	if err := c.Start(); err != nil {
		if spec.GID != "" {
			// Start applies the credential in the child between fork and exec,
			// so a refusal (EPERM, the normal case for a non-setuid build)
			// surfaces here rather than as a shell that ran. Tag it so the
			// caller can retry without the credential per the POSIX rule.
			return 0, &errGroupChange{err}
		}
		return 0, fmt.Errorf("cannot run %s: %w", spec.Path, err)
	}

	if err := c.Wait(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			if code := ee.ExitCode(); code >= 0 {
				return code, nil
			}
			// A shell killed by a signal reports 128+N, the status a waiting
			// caller would have seen from the shell itself.
			if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				return 128 + int(ws.Signal()), nil
			}
			return 1, nil
		}
		return 0, fmt.Errorf("cannot run %s: %w", spec.Path, err)
	}
	return 0, nil
}

// readPassword prompts on the CONTROLLING TERMINAL with echo off.
//
// It deliberately does not read rc.In: a group password typed into a pipe that
// some other program is also reading would be echoed into that program's input,
// and an agent harness owns stdin anyway. When there is no terminal there is no
// safe way to ask, and saying so is better than reading a secret from wherever
// stdin happens to point.
func readPassword(rc *tool.RunContext, prompt string) (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("no terminal available to read a password")
	}
	defer tty.Close()

	fmt.Fprint(tty, prompt)
	b, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(tty)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
