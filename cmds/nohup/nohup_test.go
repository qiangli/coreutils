package nohupcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty/v2"
	"github.com/qiangli/coreutils/tool"
)

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

func TestNohupOutputHelper(t *testing.T) {
	if os.Getenv("GO_WANT_NOHUP_OUTPUT_HELPER") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, "home")
	os.Exit(0)
}

func TestNohupSignalHelper(t *testing.T) {
	pidPath := os.Getenv("GO_WANT_NOHUP_SIGNAL_HELPER")
	if pidPath == "" {
		return
	}
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0o600); err != nil {
		os.Exit(2)
	}
	time.Sleep(500 * time.Millisecond)
	os.Exit(0)
}

func TestNohupInputHelper(t *testing.T) {
	if os.Getenv("GO_WANT_NOHUP_INPUT_HELPER") != "1" {
		return
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(2)
	}
	fmt.Fprintf(os.Stdout, "read:%d", len(data))
	os.Exit(0)
}

func TestNohupIgnoresHangupForChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGHUP is not available on windows")
	}
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "child.pid")
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   append(os.Environ(), "GO_WANT_NOHUP_SIGNAL_HELPER="+pidPath),
		Stdio: tool.Stdio{Out: io.Discard, Err: io.Discard},
	}
	result := make(chan int, 1)
	go func() {
		result <- run(rc, []string{os.Args[0], "-test.run=^TestNohupSignalHelper$"})
	}()

	var pid int
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidPath)
		if n, scanErr := fmt.Sscanf(string(data), "%d", &pid); err == nil && scanErr == nil && n == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("child did not report its pid")
	}
	if err := syscall.Kill(pid, syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	if code := <-result; code != 0 {
		t.Fatalf("nohup child exited after SIGHUP with code %d", code)
	}
}

func TestNohupRedirectsTerminalInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty not supported in this test on windows")
	}
	ptm, pts, err := pty.Open()
	if err != nil {
		t.Skipf("pty.Open failed: %v", err)
	}
	defer ptm.Close()
	defer pts.Close()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   append(os.Environ(), "GO_WANT_NOHUP_INPUT_HELPER=1"),
		Stdio: tool.Stdio{In: pts, Out: &out, Err: &errb},
	}
	result := make(chan int, 1)
	go func() {
		result <- run(rc, []string{os.Args[0], "-test.run=^TestNohupInputHelper$"})
	}()

	select {
	case code := <-result:
		if code != 0 || out.String() != "read:0" {
			t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
		}
	case <-time.After(250 * time.Millisecond):
		pts.Close()
		ptm.Close()
		<-result
		t.Fatal("nohup left the child reading from terminal input")
	}
}

func TestNohupFallsBackToHomeNohupOut(t *testing.T) {
	dir, home := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "nohup.out"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "GO_WANT_NOHUP_OUTPUT_HELPER=1", "HOME="+home)
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: env, Stdio: tool.Stdio{}}
	if code := run(rc, []string{os.Args[0], "-test.run=^TestNohupOutputHelper$"}); code != 0 {
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

func TestNohupRedirectsTerminalOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty not supported in this test on windows")
	}
	ptm, pts, err := pty.Open()
	if err != nil {
		t.Skipf("pty.Open failed: %v", err)
	}
	defer ptm.Close()
	defer pts.Close()

	dir := t.TempDir()
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"PATH=/bin:/usr/bin"},
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: pts, Err: pts},
	}
	if code := run(rc, []string{"sh", "-c", "printf out; printf err >&2"}); code != 0 {
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
