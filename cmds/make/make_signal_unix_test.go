//go:build unix

package makecmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func TestStandaloneSignalCleanupHelper(t *testing.T) {
	if os.Getenv("MAKE_SIGNAL_HELPER") != "1" {
		return
	}
	d := os.Getenv("MAKE_SIGNAL_DIR")
	rc := &tool.RunContext{Ctx: context.Background(), Dir: d, Env: []string{"PATH=/bin:/usr/bin"}, FS: tool.NewLocalFS(), DedicatedProcess: true, Stdio: tool.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}}
	go func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(d, "out")); err == nil {
				_ = syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	if code := run(rc, []string{"out"}); code == 0 {
		t.Fatal("signal-interrupted make succeeded")
	}
	if rc.ExitSignal != int(syscall.SIGTERM) {
		t.Fatalf("ExitSignal=%d", rc.ExitSignal)
	}
	if _, err := os.Stat(filepath.Join(d, "out")); !os.IsNotExist(err) {
		t.Fatalf("target remains: %v", err)
	}
}

func TestStandaloneSignalSetsBoundarySignalAndRemovesTarget(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "Makefile"), "out:\n\tprintf partial > out; while :; do :; done\n")
	command := exec.Command(os.Args[0], "-test.run=^TestStandaloneSignalCleanupHelper$")
	command.Env = append(os.Environ(), "MAKE_SIGNAL_HELPER=1", "MAKE_SIGNAL_DIR="+d)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("helper: %v\n%s", err, output)
	}
}
