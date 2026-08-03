//go:build unix

package killcmd

import "syscall"

type nativeSignal = syscall.Signal

type signalEntry struct {
	name string
	sig  syscall.Signal
}

var portableSignals = []signalEntry{
	{"HUP", syscall.SIGHUP}, {"INT", syscall.SIGINT}, {"QUIT", syscall.SIGQUIT},
	{"ILL", syscall.SIGILL}, {"TRAP", syscall.SIGTRAP}, {"ABRT", syscall.SIGABRT},
	{"BUS", syscall.SIGBUS}, {"FPE", syscall.SIGFPE}, {"KILL", syscall.SIGKILL},
	{"USR1", syscall.SIGUSR1}, {"SEGV", syscall.SIGSEGV}, {"USR2", syscall.SIGUSR2},
	{"PIPE", syscall.SIGPIPE}, {"ALRM", syscall.SIGALRM}, {"TERM", syscall.SIGTERM},
	{"CHLD", syscall.SIGCHLD}, {"CONT", syscall.SIGCONT}, {"STOP", syscall.SIGSTOP},
	{"TSTP", syscall.SIGTSTP}, {"TTIN", syscall.SIGTTIN}, {"TTOU", syscall.SIGTTOU},
}

func invalidSignal() nativeSignal { return syscall.Signal(-1) }

func signalByName(name string) nativeSignal {
	if name == "0" || name == "EXIT" {
		return 0
	}
	for _, entry := range portableSignals {
		if entry.name == name {
			return entry.sig
		}
	}
	return invalidSignal()
}

func signalName(number int) (string, bool) {
	if number == 0 {
		return "EXIT", true
	}
	for _, entry := range portableSignals {
		if int(entry.sig) == number {
			return entry.name, true
		}
	}
	return "", false
}

func signalNames() []string {
	names := make([]string, 0, len(portableSignals))
	for _, entry := range portableSignals {
		names = append(names, entry.name)
	}
	return names
}

func signalFromNumber(number int) nativeSignal   { return syscall.Signal(number) }
func signalNumber(sig nativeSignal) int          { return int(sig) }
func maxSignalNumber() int                       { return 255 }
func sendSignal(pid int, sig nativeSignal) error { return syscall.Kill(pid, sig) }
