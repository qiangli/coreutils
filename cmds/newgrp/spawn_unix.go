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
func defaultSpawnShell(rc *tool.RunContext, spec shellSpec) (int, error) {
	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	c := exec.CommandContext(ctx, spec.Path)
	c.Args = []string{spec.Argv0}
	c.Dir = spec.Dir
	if spec.Env != nil {
		c.Env = spec.Env
	} else if rc.Env == nil {
		c.Env = []string{}
	} else {
		c.Env = rc.Env
	}
	c.Stdin, c.Stdout, c.Stderr = rc.In, rc.Out, rc.Err

	if spec.Credential != nil {
		cred, err := syscallCredential(spec.UID, spec.Credential)
		if err != nil {
			return 0, &errGroupChange{err}
		}
		c.SysProcAttr = &syscall.SysProcAttr{Credential: cred}
	}

	if err := c.Start(); err != nil {
		if spec.Credential != nil && isCredentialStartFailure(err) {
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
				rc.ExitSignal = int(ws.Signal())
				return 128 + int(ws.Signal()), nil
			}
			return 1, nil
		}
		return 0, fmt.Errorf("cannot run %s: %w", spec.Path, err)
	}
	return 0, nil
}

func isCredentialStartFailure(err error) bool {
	// These are the errors produced by setgroups/setgid in the child for a
	// denied assignment or an over-capacity supplementary list. Errors such as
	// ENOENT and ENOEXEC identify the shell, not the credential operation, and
	// must not be mislabeled or retried as an unchanged-credential success.
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EINVAL)
}

// syscallCredential is the Unix privileged-operation adapter. Go's child
// credential path calls setgroups followed by setgid; for a privileged caller,
// setgid sets the real and effective IDs together. Rejecting unequal IDs here
// makes that platform limitation explicit instead of pretending it was met.
func syscallCredential(uid string, plan *credentialPlan) (*syscall.Credential, error) {
	if plan.RealGID != plan.EffectiveGID {
		return nil, fmt.Errorf("cannot assign distinct real and effective group ids (%s and %s)", plan.RealGID, plan.EffectiveGID)
	}
	uidValue, err := parseCredentialID("user", uid)
	if err != nil {
		return nil, err
	}
	gidValue, err := parseCredentialID("group", plan.RealGID)
	if err != nil {
		return nil, err
	}
	groups := make([]uint32, 0, len(plan.Supplementary))
	for _, group := range plan.Supplementary {
		value, err := parseCredentialID("supplementary group", group)
		if err != nil {
			return nil, err
		}
		groups = append(groups, value)
	}
	return &syscall.Credential{
		Uid:         uidValue,
		Gid:         gidValue,
		Groups:      groups,
		NoSetGroups: false,
	}, nil
}

func parseCredentialID(kind, value string) (uint32, error) {
	n, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s id %q", kind, value)
	}
	return uint32(n), nil
}

var (
	openPasswordTTY = func() (*os.File, error) {
		return os.OpenFile("/dev/tty", os.O_RDWR, 0)
	}
	readPasswordSecret = term.ReadPassword
)

// readPassword prompts on the CONTROLLING TERMINAL with echo off.
//
// It deliberately does not read rc.In: a group password typed into a pipe that
// some other program is also reading would be echoed into that program's input,
// and an agent harness owns stdin anyway. When there is no terminal there is no
// safe way to ask, and saying so is better than reading a secret from wherever
// stdin happens to point.
//
// The prompt is routed to stderr (rc.Err) so tests and programmatic callers
// can observe or redirect it.
func readPassword(rc *tool.RunContext, prompt string) (string, error) {
	tty, err := openPasswordTTY()
	if err != nil {
		return "", fmt.Errorf("no terminal available to read a password")
	}
	defer tty.Close()

	fmt.Fprint(rc.Err, prompt)
	b, err := readPasswordSecret(int(tty.Fd()))
	fmt.Fprintln(rc.Err)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
