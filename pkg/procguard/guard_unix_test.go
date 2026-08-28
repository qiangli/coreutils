//go:build !windows

package procguard

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestGuardNormalRun(t *testing.T) {
	cmd := exec.Command("sh", "-c", "printf guarded")
	var out bytes.Buffer
	cmd.Stdout = &out
	guard, err := Arm(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("Arm did not make the guard a process-group leader")
	}
	startErr := cmd.Start()
	guard.Started(startErr)
	if startErr != nil {
		t.Fatal(startErr)
	}
	waitErr := cmd.Wait()
	guard.Disarm()
	if waitErr != nil {
		t.Fatal(waitErr)
	}
	if out.String() != "guarded" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestPortableGuardDoesNotClaimSessionEscapeContainment(t *testing.T) {
	if ContainsSessionEscapes() {
		t.Fatal("portable process-group guard must not claim cgroup/job-object containment")
	}
}

func TestGuardPreservesArgvZero(t *testing.T) {
	if os.Getenv("BASHY_PROCGUARD_ARGV0_HELPER") == "1" {
		fmt.Fprint(os.Stdout, os.Args[0])
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestGuardPreservesArgvZero$", "-test.v=false")
	cmd.Args[0] = "deliberate-custom-argv0"
	cmd.Env = append(os.Environ(), "BASHY_PROCGUARD_ARGV0_HELPER=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var out bytes.Buffer
	cmd.Stdout = &out
	guard, err := Arm(cmd)
	if err != nil {
		t.Fatal(err)
	}
	startErr := cmd.Start()
	guard.Started(startErr)
	if startErr != nil {
		t.Fatal(startErr)
	}
	waitErr := cmd.Wait()
	guard.Disarm()
	if waitErr != nil {
		t.Fatal(waitErr)
	}
	if out.String() != "deliberate-custom-argv0" {
		t.Fatalf("argv[0] = %q", out.String())
	}
}

func TestGuardPreservesRelativePathArgsAndEnvironment(t *testing.T) {
	const helper = "BASHY_PROCGUARD_DIRECT_PATH_HELPER"
	if os.Getenv(helper) == "value with spaces" {
		fmt.Fprintf(os.Stdout, "%s\n%s\n%s", os.Args[0], os.Args[len(os.Args)-1], os.Getenv(helper))
		os.Exit(0)
	}
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(dir, "custom-binary")); err != nil {
		t.Fatal(err)
	}
	cmd := &exec.Cmd{
		Path: "./custom-binary",
		Args: []string{"odd argv zero", "-test.run=^TestGuardPreservesRelativePathArgsAndEnvironment$", "-test.v=false", "--", "argument with spaces"},
		Dir:  dir,
		Env:  []string{"PATH=/usr/bin:/bin", helper + "=value with spaces"},
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	guard, err := Arm(cmd)
	if err != nil {
		t.Fatal(err)
	}
	startErr := cmd.Start()
	guard.Started(startErr)
	if startErr != nil {
		t.Fatal(startErr)
	}
	waitErr := cmd.Wait()
	guard.Disarm()
	if waitErr != nil {
		t.Fatal(waitErr)
	}
	want := "odd argv zero\nargument with spaces\nvalue with spaces"
	if out.String() != want {
		t.Fatalf("guarded command identity = %q, want %q", out.String(), want)
	}
}

func TestGuardRelaysSignalStatus(t *testing.T) {
	cmd := exec.Command("sh", "-c", "kill -TERM $$")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	guard, err := Arm(cmd)
	if err != nil {
		t.Fatal(err)
	}
	startErr := cmd.Start()
	guard.Started(startErr)
	if startErr != nil {
		t.Fatal(startErr)
	}
	waitErr := cmd.Wait()
	guard.Disarm()
	exitErr, ok := waitErr.(*exec.ExitError)
	if !ok {
		t.Fatalf("wait error = %v", waitErr)
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGTERM {
		t.Fatalf("wait status = %#v, want signal SIGTERM", exitErr.Sys())
	}
}

// TestGuardExistsBeforeCommandStart closes the former cmd.Start-to-watcher
// window. The helper publishes readiness immediately after Start returns while
// the requested command delays before publishing its own PID. Killing the Go
// helper at that boundary must either prevent the command from starting or
// remove it through the already-established in-group guard.
func TestGuardExistsBeforeCommandStart(t *testing.T) {
	if os.Getenv("BASHY_PROCGUARD_EARLY_HELPER") == "1" {
		dir := os.Getenv("BASHY_PROCGUARD_EARLY_DIR")
		cmd := exec.Command("sh", "-c", "sleep 0.05; echo $$ > child.pid; sleep 120")
		cmd.Dir = dir
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		guard, err := Arm(cmd)
		if err != nil {
			return
		}
		startErr := cmd.Start()
		guard.Started(startErr)
		if startErr != nil {
			return
		}
		_ = os.WriteFile(filepath.Join(dir, "ready"), []byte("ready"), 0o600)
		_ = cmd.Wait()
		guard.Disarm()
		return
	}

	dir := t.TempDir()
	helper := exec.Command(os.Args[0], "-test.run=^TestGuardExistsBeforeCommandStart$", "-test.v=false")
	helper.Env = append(os.Environ(), "BASHY_PROCGUARD_EARLY_HELPER=1", "BASHY_PROCGUARD_EARLY_DIR="+dir)
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	waitFile(t, filepath.Join(dir, "ready"), 5*time.Second)
	if err := helper.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = helper.Wait()
	time.Sleep(150 * time.Millisecond)
	raw, err := os.ReadFile(filepath.Join(dir, "child.pid"))
	if os.IsNotExist(err) {
		return // The requested command never started: also fail-closed.
	}
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
	if !waitGone(pid, 5*time.Second) {
		t.Fatalf("command %d survived supervisor death at the Start boundary", pid)
	}
}

func waitFile(t *testing.T, path string, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("file did not appear: %s", path)
}

func waitGone(pid int, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return syscall.Kill(pid, 0) == syscall.ESRCH
}
