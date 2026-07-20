package nohupcmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// TestMain lets the test binary re-exec itself as a trivial cross-platform
// "helper" command: when GO_NOHUP_HELPER is set it prints that value to stdout
// and exits 0. This gives OS-neutral tests a real external command to run under
// nohup without depending on a Unix shell (`sh` is absent on Windows).
func TestMain(m *testing.M) {
	if v, ok := os.LookupEnv("GO_NOHUP_HELPER"); ok {
		io.WriteString(os.Stdout, v)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestNohupMissing(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")}}, nil)
	if code != 2 {
		t.Fatalf("code=%d", code)
	}
}

func TestNohupRunsCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command differs on windows")
	}
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Env: []string{"PATH=/bin:/usr/bin"}, Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")}}, []string{"sh", "-c", "printf ok"})
	if code != 0 || out.String() != "ok" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestNohupSearchesPATHRelativeToRunContextDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command differs on windows")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	command := filepath.Join(bin, "helper")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf relative\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"PATH=bin"},
		Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")},
	}
	if code := run(rc, []string{"helper"}); code != 0 || out.String() != "relative" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestNohupFoundButNotExecutableReturns126(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on windows")
	}
	dir := t.TempDir()
	command := filepath.Join(dir, "blocked")
	if err := os.WriteFile(command, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"PATH=."}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	if code := run(rc, []string{"blocked"}); code != 126 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestNohupNotFoundReturns127(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Env: []string{"PATH=" + t.TempDir()}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	if code := run(rc, []string{"missing-command"}); code != 127 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestNohupRedirectsTerminalEquivalentOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command differs on windows")
	}
	dir := t.TempDir()
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"PATH=/bin:/usr/bin"},
		Stdio: tool.Stdio{In: strings.NewReader("")},
	}
	if code := run(rc, []string{"sh", "-c", "printf out; printf err >&2; exit 17"}); code != 17 {
		t.Fatalf("code=%d", code)
	}
	data, err := os.ReadFile(filepath.Join(dir, "nohup.out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "outerr" {
		t.Fatalf("nohup.out=%q", data)
	}
}

func TestNohupRedirectsStderrToStdoutWhenErrIsNil(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command differs on windows")
	}
	var out bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   []string{"PATH=/bin:/usr/bin"},
		Stdio: tool.Stdio{Out: &out, In: strings.NewReader("")},
	}
	if code := run(rc, []string{"sh", "-c", "printf out; printf err >&2"}); code != 0 {
		t.Fatalf("code=%d out=%q", code, out.String())
	}
	if out.String() != "outerr" {
		t.Fatalf("stdout=%q", out.String())
	}
}

func TestNohupFallsBackToHomeNohupOut(t *testing.T) {
	dir, home := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "nohup.out"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Use the test binary itself as the command (see TestMain) so the
	// fallback-to-$HOME path is exercised on every OS. The prior `sh -c`
	// form resolved to 127 on Windows, where /bin:/usr/bin and `sh` don't
	// exist; this stays byte-exact ("home") and shell-free.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + filepath.Dir(exe), "HOME=" + home, "GO_NOHUP_HELPER=home"}
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: env, Stdio: tool.Stdio{}}
	if code := run(rc, []string{filepath.Base(exe)}); code != 0 {
		t.Fatalf("code=%d", code)
	}
	data, err := os.ReadFile(filepath.Join(home, "nohup.out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "home" {
		t.Fatalf("home/nohup.out=%q", data)
	}
}
