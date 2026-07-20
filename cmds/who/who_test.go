package whocmd

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
)

func TestWhoFileAndCount(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob pts/1 1 host\nalice tty1 2 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"-q", "utmp"})
	if code != 0 || !strings.Contains(out.String(), "# users=2") {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestWhoHelp(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"--help"})
	if code != 0 || !strings.Contains(out.String(), "Usage: who") {
		t.Fatalf("--help: code=%d out=%q", code, out.String())
	}
}

func TestWhoAliasHelpVersion(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"-h"})
	if code != 0 || !strings.Contains(out.String(), "Usage: who") {
		t.Fatalf("-h: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	out.Reset()
	code = run(rc, []string{"-V"})
	if code != 0 || !strings.Contains(out.String(), "qiangli/coreutils") {
		t.Fatalf("-V: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestWhoTimeFormat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob pts/1 1720000000 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"utmp"})
	if code != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	want := time.Unix(1720000000, 0).Local().Format("Jan _2 15:04")
	if !strings.Contains(out.String(), want) {
		t.Fatalf("expected time %q in output, got %q", want, out.String())
	}
}

func TestWhoWritable(t *testing.T) {
	dir := t.TempDir()
	tty := filepath.Join(dir, "faketty")
	if err := os.WriteFile(tty, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tty, 0o660); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob "+tty+" 1 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"--writable", "utmp"})
	if code != 0 {
		t.Fatalf("--writable: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if runtime.GOOS == "windows" {
		// Windows has no Unix tty group-write permission model, so the
		// writable status is unknowable ('?'), and os.Chmod cannot flip
		// it (chmod only toggles the read-only attribute).
		if !strings.Contains(out.String(), "bob      ?   "+tty) {
			t.Fatalf("expected writable status '?' for bob on windows, got %q", out.String())
		}
		return
	}
	if !strings.Contains(out.String(), "bob      +   "+tty) {
		t.Fatalf("expected writable status '+' for bob, got %q", out.String())
	}

	// Remove group write: status should flip to '-'.
	if err := os.Chmod(tty, 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	code = run(rc, []string{"--writable", "utmp"})
	if code != 0 {
		t.Fatalf("--writable after chmod: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "bob      -   "+tty) {
		t.Fatalf("expected writable status '-' for bob, got %q", out.String())
	}
}

func TestWhoIdle(t *testing.T) {
	dir := t.TempDir()
	tty := filepath.Join(dir, "faketty")
	if err := os.WriteFile(tty, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob "+tty+" 1 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Recent activity: idle should be '.'.
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"-u", "utmp"})
	if code != 0 {
		t.Fatalf("-u: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), " .") {
		t.Fatalf("expected idle '.' for active terminal, got %q", out.String())
	}

	// Stale activity: idle should be 'old'.
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(tty, old, old); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	code = run(rc, []string{"-u", "utmp"})
	if code != 0 {
		t.Fatalf("-u stale: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), " old") {
		t.Fatalf("expected idle 'old' for stale terminal, got %q", out.String())
	}
}

func TestWhoOperands(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}

	out.Reset()
	errbuf := &bytes.Buffer{}
	rc.Err = errbuf
	code := run(rc, []string{"am", "x", "y", "z"})
	if code == 0 {
		t.Fatalf("expected error for 'am x y z', got code=0")
	}
	if !strings.Contains(errbuf.String(), "extra operand") {
		t.Fatalf("expected extra operand error, got %q", errbuf.String())
	}
}

func TestWhoBootTime(t *testing.T) {
	dir := t.TempDir()
	content := "reboot ~ 1720000000 ~ BOOT_TIME\nbob pts/1 1720000000 host\n"
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"-b", "utmp"})
	if code != 0 {
		t.Fatalf("-b: code=%d err=%q", code, errb.String())
	}
	if !strings.Contains(out.String(), "reboot") {
		t.Fatalf("expected reboot record in output, got %q", out.String())
	}
	if strings.Contains(out.String(), "bob") {
		t.Fatalf("did not expect bob in output for -b, got %q", out.String())
	}
}

func TestWhoMessageOption(t *testing.T) {
	dir := t.TempDir()
	tty := filepath.Join(dir, "faketty")
	if err := os.WriteFile(tty, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tty, 0o660); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob "+tty+" 1 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"-w", "utmp"})
	if code != 0 {
		t.Fatalf("-w: code=%d err=%q", code, errb.String())
	}
	if !strings.Contains(out.String(), "bob") {
		t.Fatalf("expected bob in output, got %q", out.String())
	}
	if runtime.GOOS == "windows" {
		if !strings.Contains(out.String(), "bob      ?   "+tty) {
			t.Fatalf("expected message status '?' on windows, got %q", out.String())
		}
		return
	}
	if !strings.Contains(out.String(), "bob      +   "+tty) {
		t.Fatalf("expected message status '+' for bob, got %q", out.String())
	}
}
