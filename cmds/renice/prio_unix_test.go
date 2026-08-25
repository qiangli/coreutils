//go:build unix

package renicecmd

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// execReal drives run() against the live kernel scheduler.
func execReal(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, args)
	return out.String(), errb.String(), code
}

// A zero increment on our own PID must leave the nice value untouched and
// print nothing: it proves get-then-set reads the CURRENT value (on Linux
// this fails if the raw getpriority encoding is not undone — nice 0 would
// read as 20 and be re-set as 19) and that success is silent per POSIX.
func TestRealZeroIncrementPreservesOwnNiceAndIsSilent(t *testing.T) {
	pid := os.Getpid()
	before, err := hostScheduler{}.get(whichProcess, pid)
	if err != nil {
		t.Skipf("cannot read own priority: %v", err)
	}
	out, errs, code := execReal(t, "-n", "0", "-p", strconv.Itoa(pid))
	if code != 0 {
		t.Skipf("renice -n 0 on self not permitted here: %q", errs)
	}
	if out != "" {
		t.Errorf("stdout must not be used; got %q", out)
	}
	after, err := hostScheduler{}.get(whichProcess, pid)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Errorf("zero increment changed nice %d -> %d", before, after)
	}
}

// startParkedChild re-executes this test binary as a helper that blocks on
// stdin in its own process group, giving the tests a live PID/PGID they own
// and may renice without touching any other process.
func startParkedChild(t *testing.T) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("cannot find test binary: %v", err)
	}
	child := exec.Command(exe, "-test.run=^TestReniceParkedChildHelper$", "-test.v")
	child.Env = append(os.Environ(), "RENICE_TEST_CHILD=1")
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Skipf("cannot start helper child: %v", err)
	}
	t.Cleanup(func() {
		stdin.Close()
		child.Wait()
	})
	// Wait for the child to announce it is parked before touching it.
	buf := make([]byte, 1)
	if _, err := io.ReadFull(stdout, buf); err != nil {
		t.Skipf("helper child did not start: %v", err)
	}
	return child
}

// TestReniceParkedChildHelper is not a test of renice: it is the body of
// the helper child spawned by startParkedChild.
func TestReniceParkedChildHelper(t *testing.T) {
	if os.Getenv("RENICE_TEST_CHILD") != "1" {
		t.Skip("helper body, only meaningful when re-executed")
	}
	os.Stdout.WriteString("R")
	io.Copy(io.Discard, os.Stdin)
}

// A positive increment never needs privilege, so it is exercised for real
// against a child we own, for both PRIO_PROCESS and PRIO_PGRP dispatch.
func TestRealPositiveIncrementOnOwnedChildAndGroup(t *testing.T) {
	child := startParkedChild(t)
	pid := child.Process.Pid
	old, err := hostScheduler{}.get(whichProcess, pid)
	if err != nil {
		t.Fatalf("read child priority: %v", err)
	}
	if old > 15 {
		t.Skipf("child already at nice %d; +1 could clamp ambiguously", old)
	}
	if out, errs, code := execReal(t, "-n", "1", "-p", strconv.Itoa(pid)); code != 0 || out != "" {
		t.Fatalf("renice -n 1 -p child: code=%d out=%q stderr=%q", code, out, errs)
	}
	got, err := hostScheduler{}.get(whichProcess, pid)
	if err != nil {
		t.Fatal(err)
	}
	if got != old+1 {
		t.Errorf("child nice = %d after +1, want %d", got, old+1)
	}
	// Setpgid gave the child its own group with PGID == its PID.
	out, errs, code := execReal(t, "-n", "1", "-g", strconv.Itoa(pid))
	if runtime.GOOS != "linux" {
		if code != 1 || out != "" || !strings.Contains(errs, "exact per-process membership enumeration is unavailable") {
			t.Fatalf("non-Linux -g must fail closed: code=%d out=%q stderr=%q", code, out, errs)
		}
		return
	}
	if code != 0 || out != "" {
		t.Fatalf("renice -n 1 -g childpgid: code=%d out=%q stderr=%q", code, out, errs)
	}
	// Linux implements the group operation as exact per-process expansion;
	// read the child itself rather than reintroducing collective getpriority
	// semantics in the assertion.
	got, err = hostScheduler{}.get(whichProcess, pid)
	if err != nil {
		t.Fatal(err)
	}
	if got != old+2 {
		t.Errorf("child-group nice = %d after two +1 passes, want %d", got, old+2)
	}
}

// Lowering a nice value requires appropriate privileges; without them the
// kernel refuses, renice diagnoses on stderr naming the ID, and exits >0.
// If this environment IS privileged, the change is undone and the test
// skips rather than asserting a refusal the kernel never made.
func TestRealNegativeIncrementWithoutPrivilegeFailsPerOperand(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running privileged; the refusal path cannot be observed")
	}
	child := startParkedChild(t)
	pid := child.Process.Pid
	spid := strconv.Itoa(pid)
	out, errs, code := execReal(t, "-n", "-1", "-p", spid)
	if code == 0 {
		// RLIMIT_NICE (or similar) allowed it: restore and skip honestly.
		execReal(t, "-n", "1", "-p", spid)
		t.Skip("this environment permits lowering nice without euid 0")
	}
	if code != 1 {
		t.Errorf("code=%d, want per-operand failure status 1", code)
	}
	if out != "" {
		t.Errorf("stdout must stay unused on failure; got %q", out)
	}
	if !strings.Contains(errs, "renice: "+spid+":") {
		t.Errorf("stderr %q must name the failing ID %s", errs, spid)
	}
}

// A PID beyond any real process draws the kernel's ESRCH through the
// per-operand diagnostic path.
func TestRealNonexistentProcessDiagnostic(t *testing.T) {
	const ghost = "2147483646"
	out, errs, code := execReal(t, "-n", "0", "-p", ghost)
	if code != 1 || out != "" {
		t.Fatalf("code=%d out=%q stderr=%q, want silent stdout and status 1", code, out, errs)
	}
	if !strings.Contains(errs, "renice: "+ghost+":") {
		t.Errorf("stderr %q must name the failing ID", errs)
	}
}
