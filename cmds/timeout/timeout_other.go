//go:build !unix

package timeoutcmd

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// On non-unix (Windows) there are no POSIX signals to deliver to another
// process; timeout falls back to terminating it. Any signal request is accepted
// and treated as "terminate".
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

func setProcGroup(c *exec.Cmd) {}

func signalCmd(c *exec.Cmd, sig os.Signal, foreground bool) {
	if c.Process != nil {
		_ = c.Process.Kill()
	}
}
