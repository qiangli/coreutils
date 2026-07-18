//go:build linux || darwin

package whocmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/creack/pty/v2"
	"github.com/qiangli/coreutils/tool"
)

func TestWhoSameHost(t *testing.T) {
	_, tty, err := pty.Open()
	if err != nil {
		t.Skipf("cannot open pty: %v", err)
	}
	defer tty.Close()

	dir := t.TempDir()
	ttyName := tty.Name()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob "+ttyName+" 1 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{In: tty, Out: &out, Err: &errb}}
	code := run(rc, []string{"-m", "utmp"})
	if code != 0 {
		t.Fatalf("-m: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "bob") || !strings.Contains(out.String(), filepath.Base(ttyName)) {
		t.Fatalf("expected user and tty in output, got %q", out.String())
	}
}

func TestWhoAmI(t *testing.T) {
	_, tty, err := pty.Open()
	if err != nil {
		t.Skipf("cannot open pty: %v", err)
	}
	defer tty.Close()

	dir := t.TempDir()
	ttyName := tty.Name()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("alice "+ttyName+" 1 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{In: tty, Out: &out, Err: &errb}}
	code := run(rc, []string{"utmp", "am", "i"})
	if code != 0 {
		t.Fatalf("am i: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "alice") || !strings.Contains(out.String(), filepath.Base(ttyName)) {
		t.Fatalf("expected user and tty in output, got %q", out.String())
	}
}
