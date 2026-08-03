//go:build !windows

package envcmd

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// scriptInterpreter is the shell used to retry a COMMAND that the kernel
// rejects as an unrecognized executable image (ENOEXEC) — conventionally a
// script lacking a "#!" interpreter line.
const scriptInterpreter = "/bin/sh"

func isExecFormatError(err error) bool {
	return errors.Is(err, syscall.ENOEXEC)
}

var commandSignalMu sync.Mutex

var namedSignals = map[string]syscall.Signal{
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

func commandSignals(specs []string) ([]os.Signal, error) {
	var out []os.Signal
	seen := make(map[syscall.Signal]bool)
	add := func(sig syscall.Signal, label string) error {
		if sig <= 0 {
			return fmt.Errorf("invalid signal %q", label)
		}
		if sig == syscall.SIGKILL || sig == syscall.SIGSTOP {
			return fmt.Errorf("cannot ignore signal %q", label)
		}
		if !seen[sig] {
			seen[sig] = true
			out = append(out, sig)
		}
		return nil
	}
	for _, spec := range specs {
		for _, part := range strings.Split(spec, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if strings.EqualFold(part, "all") {
				for name, sig := range namedSignals {
					if sig != syscall.SIGKILL && sig != syscall.SIGSTOP {
						if err := add(sig, name); err != nil {
							return nil, err
						}
					}
				}
				continue
			}
			if n, err := strconv.Atoi(part); err == nil {
				if err := add(syscall.Signal(n), part); err != nil {
					return nil, err
				}
				continue
			}
			name := strings.TrimPrefix(strings.ToUpper(part), "SIG")
			sig, ok := namedSignals[name]
			if !ok {
				return nil, fmt.Errorf("unknown signal %q", part)
			}
			if err := add(sig, part); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// Ignored dispositions survive exec. Hold the mutex while temporarily changing
// the process dispositions so concurrent env invocations cannot overlap.
func ignoreForCommandStart(signals []os.Signal) func() {
	if len(signals) == 0 {
		return func() {}
	}
	commandSignalMu.Lock()
	alreadyIgnored := make([]bool, len(signals))
	for i, sig := range signals {
		alreadyIgnored[i] = signal.Ignored(sig)
		signal.Ignore(sig)
	}
	return func() {
		for i, sig := range signals {
			if !alreadyIgnored[i] {
				// Reset only promises to undo Notify. Move an Ignore-managed
				// signal through Notify first so the runtime reinstalls the
				// inherited default disposition instead of leaving it ignored.
				ch := make(chan os.Signal, 1)
				signal.Notify(ch, sig)
				signal.Reset(sig)
			}
		}
		commandSignalMu.Unlock()
	}
}

func commandSignalStatus(state *os.ProcessState) int {
	if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return 125
}

func defaultCommandPath() string { return "/usr/bin:/bin" }

func resolveCommandCandidate(_ interface{ ResolveExecutable(string) string }, candidate string) string {
	return candidate
}
