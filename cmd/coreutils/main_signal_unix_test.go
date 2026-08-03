//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/qiangli/coreutils/multicall"
)

const coreutilsSignalHelper = "COREUTILS_MAIN_SIGNAL_HELPER"

// TestCoreutilsEnvStandaloneSignalBoundary is both the parent assertion and
// the subprocess helper. The boundary role calls the real cmd/coreutils main,
// not a facsimile, so this pins the actual standalone multicall behavior.
func TestCoreutilsEnvStandaloneSignalBoundary(t *testing.T) {
	if os.Getenv(coreutilsSignalHelper) == "1" {
		args := argsAfterDoubleDash(os.Args)
		if len(args) != 2 {
			os.Exit(2)
		}
		sig := signalNumber(args[1])
		switch args[0] {
		case "raise":
			multicall.TerminateBySignal(int(sig))
			os.Exit(2)
		case "boundary":
			exe, err := os.Executable()
			if err != nil {
				os.Exit(2)
			}
			os.Args = []string{
				"coreutils", "env", exe,
				"-test.run=^TestCoreutilsEnvStandaloneSignalBoundary$",
				"--", "raise", args[1],
			}
			main()
			os.Exit(2)
		default:
			os.Exit(2)
		}
	}

	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable unavailable: %v", err)
	}
	for _, name := range []string{"TERM", "INT", "USR1", "QUIT", "ABRT"} {
		t.Run(name, func(t *testing.T) {
			sig := signalNumber(name)
			cmd := exec.Command(exe,
				"-test.run=^TestCoreutilsEnvStandaloneSignalBoundary$",
				"--", "boundary", name)
			cmd.Env = append(os.Environ(), coreutilsSignalHelper+"=1")
			cmd.Dir = t.TempDir()

			err := cmd.Run()
			ee, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("coreutils env boundary err = %v (%T), want signal exit", err, err)
			}
			ws, ok := ee.ProcessState.Sys().(syscall.WaitStatus)
			if !ok || !ws.Signaled() {
				t.Fatalf("coreutils env exited normally (%d), want killed by SIG%s", ee.ExitCode(), name)
			}
			if ws.Signal() != sig {
				t.Errorf("coreutils env killed by %v, want %v", ws.Signal(), sig)
			}
		})
	}
}

func argsAfterDoubleDash(args []string) []string {
	for i, arg := range args {
		if arg == "--" {
			return args[i+1:]
		}
	}
	return nil
}

func signalNumber(name string) syscall.Signal {
	switch strings.TrimPrefix(strings.ToUpper(name), "SIG") {
	case "TERM":
		return syscall.SIGTERM
	case "INT":
		return syscall.SIGINT
	case "USR1":
		return syscall.SIGUSR1
	case "QUIT":
		return syscall.SIGQUIT
	case "ABRT":
		return syscall.SIGABRT
	default:
		return 0
	}
}
