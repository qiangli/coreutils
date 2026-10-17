//go:build !unix && !windows

package timeoutcmd

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// On platforms with neither POSIX signals nor Windows job objects, timeout falls
// back to terminating the process.
func signalByName(name string) os.Signal {
	n := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(name)), "SIG")
	if n == "" {
		return nil
	}
	if _, err := strconv.Atoi(n); err == nil {
		return os.Kill
	}
	// accept known names (validation only); delivery is always Kill
	switch n {
	case "HUP", "INT", "QUIT", "KILL", "TERM", "ABRT", "USR1", "USR2", "STOP", "CONT":
		return os.Kill
	}
	return nil
}

func killSignal() os.Signal { return os.Kill }

type procTree struct{}

func createProcTree(c *exec.Cmd, foreground bool) *procTree {
	return &procTree{}
}

func (t *procTree) started(c *exec.Cmd) {}

func (t *procTree) signal(c *exec.Cmd, sig os.Signal) {
	if c != nil && c.Process != nil {
		_ = c.Process.Kill()
	}
}

func (t *procTree) close() {}

func setProcGroup(c *exec.Cmd) {}

func signalCmd(c *exec.Cmd, sig os.Signal, foreground bool) {
	if c != nil && c.Process != nil {
		_ = c.Process.Kill()
	}
}
