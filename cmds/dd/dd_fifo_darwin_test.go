//go:build darwin

package ddcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

func TestDdDarwinNamedFIFOInputFailsLoudlyWithoutLeaking(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	beforeG := runtime.NumGoroutine()
	beforeFD := openDescriptorCount(t)
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	started := time.Now()
	if code := cmd.Run(rc, []string{"if=fifo", "status=noxfer"}); code != 1 {
		t.Fatalf("code=%d want 1; stderr=%q", code, errb.String())
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("unsupported FIFO path took %v; want deterministic immediate failure", elapsed)
	}
	const want = "dd: failed to open 'fifo': interruptible named FIFO input is not supported on darwin\n"
	if errb.String() != want {
		t.Fatalf("stderr=%q want %q", errb.String(), want)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout=%q want empty", out.String())
	}
	if rc.ExitSignal != 0 {
		t.Fatalf("ExitSignal=%d want 0", rc.ExitSignal)
	}
	waitForGoroutines(t, beforeG)
	waitForDescriptors(t, beforeFD)
}

func TestDdDarwinRegularFileInputStillWorks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir,
		Stdio: tool.Stdio{In: strings.NewReader("unused"), Out: &out, Err: &errb},
	}
	if code := cmd.Run(rc, []string{"if=in", "status=none"}); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
	if out.String() != "payload" {
		t.Fatalf("stdout=%q want payload", out.String())
	}
}
