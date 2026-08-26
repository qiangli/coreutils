//go:build unix

package catcmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

const catSIGPIPEHelperEnv = "BASHY_CAT_SIGPIPE_HELPER"

func TestCatProcessSIGPIPEBehavior(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    string
		ignored bool
	}{
		{name: "default disposition", mode: "default"},
		{name: "inherited ignored disposition", mode: "ignored", ignored: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdoutR, stdoutW, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			readyR, readyW, err := os.Pipe()
			if err != nil {
				stdoutR.Close()
				stdoutW.Close()
				t.Fatal(err)
			}

			var stderr bytes.Buffer
			child := exec.Command(os.Args[0], "-test.run=^TestCatSIGPIPEHelper$")
			child.Env = append(os.Environ(), catSIGPIPEHelperEnv+"="+tc.mode)
			child.Stdout = stdoutW
			child.Stderr = &stderr
			child.ExtraFiles = []*os.File{readyW}
			if err := child.Start(); err != nil {
				stdoutR.Close()
				stdoutW.Close()
				readyR.Close()
				readyW.Close()
				t.Fatal(err)
			}
			stdoutW.Close()
			readyW.Close()

			ready := make([]byte, len("ready\n"))
			if _, err := io.ReadFull(readyR, ready); err != nil || string(ready) != "ready\n" {
				child.Process.Kill()
				child.Wait()
				stdoutR.Close()
				readyR.Close()
				t.Fatalf("helper readiness = %q, %v; stderr=%q", ready, err, stderr.String())
			}
			readyR.Close()
			// Closing the only read end makes the helper's stdout a real broken
			// pipe. The large input prevents completion before this handshake.
			stdoutR.Close()

			done := make(chan error, 1)
			go func() { done <- child.Wait() }()
			select {
			case err := <-done:
				if tc.ignored {
					exitErr, ok := err.(*exec.ExitError)
					if !ok || exitErr.ExitCode() != 1 || !strings.Contains(stderr.String(), "cat: write error:") {
						t.Fatalf("ignored SIGPIPE: err=%v stderr=%q, want exit 1 and diagnostic", err, stderr.String())
					}
					return
				}
				exitErr, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("default SIGPIPE: err=%v stderr=%q, want signaled child", err, stderr.String())
				}
				status, ok := exitErr.Sys().(syscall.WaitStatus)
				if !ok || !status.Signaled() || status.Signal() != syscall.SIGPIPE {
					t.Fatalf("default SIGPIPE: wait status=%v stderr=%q, want SIGPIPE", exitErr.Sys(), stderr.String())
				}
			case <-time.After(5 * time.Second):
				child.Process.Kill()
				<-done
				t.Fatal("cat SIGPIPE helper did not terminate")
			}
		})
	}
}

func TestCatSIGPIPEHelper(t *testing.T) {
	mode := os.Getenv(catSIGPIPEHelperEnv)
	if mode == "" {
		return
	}
	ignored := mode == "ignored"
	if ignored {
		signal.Ignore(syscall.SIGPIPE)
	} else {
		signal.Reset(syscall.SIGPIPE)
	}
	ready := os.NewFile(3, "cat-sigpipe-ready")
	if ready == nil {
		os.Exit(97)
	}
	if _, err := ready.WriteString("ready\n"); err != nil {
		os.Exit(98)
	}
	ready.Close()

	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(), SIGPIPEIgnored: ignored,
		Stdio: tool.Stdio{
			In:  strings.NewReader(strings.Repeat("x", 1<<20)),
			Out: os.Stdout,
			Err: os.Stderr,
		},
	}
	os.Exit(cmd.Run(rc, nil))
}
