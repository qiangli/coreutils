package whocmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestWhoWritable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob pts/1 1 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"--writable", "utmp"})
	if code != 0 || !strings.Contains(out.String(), "+ ") {
		t.Fatalf("--writable: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestWhoMFlag(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob pts/1 1 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"-m", "utmp"})
	if code != 0 {
		t.Fatalf("-m: code=%d err=%q", code, errb.String())
	}
}
