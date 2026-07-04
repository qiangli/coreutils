//go:build unix

package timeoutcmd

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// unixSignals maps signal names (no SIG prefix) portable across Linux + macOS to
// their syscall value. Linux-only signals (PWR/STKFLT) are intentionally absent.
var unixSignals = map[string]syscall.Signal{
	"HUP": syscall.SIGHUP, "INT": syscall.SIGINT, "QUIT": syscall.SIGQUIT,
	"ILL": syscall.SIGILL, "TRAP": syscall.SIGTRAP, "ABRT": syscall.SIGABRT,
	"BUS": syscall.SIGBUS, "FPE": syscall.SIGFPE, "KILL": syscall.SIGKILL,
	"USR1": syscall.SIGUSR1, "SEGV": syscall.SIGSEGV, "USR2": syscall.SIGUSR2,
	"PIPE": syscall.SIGPIPE, "ALRM": syscall.SIGALRM, "TERM": syscall.SIGTERM,
	"CHLD": syscall.SIGCHLD, "CONT": syscall.SIGCONT, "STOP": syscall.SIGSTOP,
	"TSTP": syscall.SIGTSTP, "TTIN": syscall.SIGTTIN, "TTOU": syscall.SIGTTOU,
	"URG": syscall.SIGURG, "XCPU": syscall.SIGXCPU, "XFSZ": syscall.SIGXFSZ,
	"VTALRM": syscall.SIGVTALRM, "PROF": syscall.SIGPROF, "WINCH": syscall.SIGWINCH,
	"IO": syscall.SIGIO, "SYS": syscall.SIGSYS,
}

func signalByName(name string) os.Signal {
	n := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(name)), "SIG")
	if sig, ok := unixSignals[n]; ok {
		return sig
	}
	if v, err := strconv.Atoi(n); err == nil && v > 0 {
		return syscall.Signal(v)
	}
	return nil
}

func killSignal() os.Signal { return syscall.SIGKILL }

// setProcGroup puts the command in its own process group so the whole child tree
// can be signalled together (GNU timeout's default).
func setProcGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalCmd sends sig to the command — to its process group (negative pid) unless
// --foreground, so a wrapping shell's children also receive it.
func signalCmd(c *exec.Cmd, sig os.Signal, foreground bool) {
	if c.Process == nil {
		return
	}
	if ssig, ok := sig.(syscall.Signal); ok && !foreground {
		if err := syscall.Kill(-c.Process.Pid, ssig); err == nil {
			return
		}
	}
	_ = c.Process.Signal(sig)
}
