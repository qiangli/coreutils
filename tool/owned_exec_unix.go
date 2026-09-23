//go:build unix

package tool

import (
	"golang.org/x/sys/unix"
	"mvdan.cc/sh/v3/interp/ownedexec"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func installOwnedFrame(c *exec.Cmd, f *os.File) (uint64, func(), error) {
	_ = os.Remove(f.Name())
	fd := uint64(3 + len(c.ExtraFiles))
	c.ExtraFiles = append(c.ExtraFiles, f)
	return fd, func() {}, nil
}

// ExecOwnedCommand replaces the current process while preserving PID. For a
// verified Bashy target, large argv/env travel through an inheritable fd.
func ExecOwnedCommand(path string, args, env []string) error {
	if err := CheckExecBudget(args, env); err != nil {
		return err
	}
	if !needsOwnedFrame(args, env) || !verifiedOwnedImage(path) {
		return syscall.Exec(path, args, env)
	}
	f, err := os.CreateTemp("", ".bashy-owned-exec-*")
	if err != nil {
		return err
	}
	defer f.Close()
	defer os.Remove(f.Name())
	if err := ownedexec.Write(f, ownedexec.Frame{Args: args, Env: env}); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	fd, err := unix.FcntlInt(f.Fd(), unix.F_DUPFD, 3)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	_ = os.Remove(f.Name())
	nativeEnv := earlyOwnedEnv(env)
	nativeEnv = append(nativeEnv, ownedexec.Marker+"="+strconv.Itoa(fd))
	return syscall.Exec(path, []string{path, ownedexec.Sentinel}, nativeEnv)
}
