//go:build unix

package edcmd

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

const edSignalHelper = "COREUTILS_ED_SIGNAL_HELPER"

func TestEdSignalSetPreservesInheritedIgnores(t *testing.T) {
	cases := []struct {
		name    string
		ignored map[os.Signal]bool
		want    []os.Signal
	}{
		{
			name: "default dispositions receive ed actions",
			want: []os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT},
		},
		{
			name: "inherited ignores stay ignored",
			ignored: map[os.Signal]bool{
				syscall.SIGHUP:  true,
				syscall.SIGINT:  true,
				syscall.SIGQUIT: true,
			},
		},
		{
			name:    "mixed dispositions",
			ignored: map[os.Signal]bool{syscall.SIGINT: true},
			want:    []os.Signal{syscall.SIGHUP, syscall.SIGQUIT},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := edSignalSet(func(sig os.Signal) bool { return tc.ignored[sig] })
			if len(got) != len(tc.want) {
				t.Fatalf("signal set=%v want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("signal set=%v want %v", got, tc.want)
				}
			}
		})
	}
}

func TestEdSignalProcessContract(t *testing.T) {
	if mode := os.Getenv(edSignalHelper); mode != "" {
		if mode == "ignored-int" {
			signal.Ignore(syscall.SIGINT)
		}
		dir, _ := os.Getwd()
		rc := &tool.RunContext{
			Ctx: context.Background(), Dir: dir, FS: tool.NewLocalFS(), Env: os.Environ(),
			Stdio: tool.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr},
		}
		os.Exit(runCore(rc, []string{"-s", "-p", "R"}))
	}

	for _, tc := range []struct {
		name, mode string
		sig        os.Signal
		want       string
	}{
		{name: "SIGQUIT is ignored", mode: "quit", sig: syscall.SIGQUIT, want: "R"},
		{name: "inherited ignored SIGINT stays ignored", mode: "ignored-int", sig: syscall.SIGINT, want: "R"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestEdSignalProcessContract$")
			cmd.Env = append(os.Environ(), edSignalHelper+"="+tc.mode)
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			ready := make([]byte, 1)
			if _, err := io.ReadFull(stdout, ready); err != nil || string(ready) != "R" {
				t.Fatalf("readiness=%q err=%v", ready, err)
			}
			if err := cmd.Process.Signal(tc.sig); err != nil {
				t.Fatal(err)
			}
			// Let the signal reach either the inherited disposition or ed's
			// registered action before supplying the terminating command.
			time.Sleep(20 * time.Millisecond)
			if _, err := io.WriteString(stdin, "Q\n"); err != nil {
				t.Fatal(err)
			}
			_ = stdin.Close()
			rest, err := io.ReadAll(stdout)
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Wait(); err != nil {
				t.Fatalf("wait: %v; output=%q", err, append(ready, rest...))
			}
			if got := string(append(ready, rest...)); got != tc.want {
				t.Fatalf("output=%q want %q", got, tc.want)
			}
		})
	}
}
