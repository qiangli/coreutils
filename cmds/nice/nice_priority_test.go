package nicecmd

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// nice runs in-process inside embedding hosts (see shell/Handler), so
// adjusting scheduling priority must land on the spawned child, never on
// nice's own (the host's) process — otherwise one "nice" invocation would
// permanently alter the niceness of every later command in the same host
// process.
func TestNiceDoesNotAlterOwnPriority(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("priority adjustment is not supported on this platform")
	}
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("true(1) not found")
	}
	before := currentPriority()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Env: []string{"PATH=/usr/bin:/bin"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	if code := run(rc, []string{"-n", "5", "true"}); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}

	if after := currentPriority(); after != before {
		t.Fatalf("nice's own priority changed: before=%d after=%d", before, after)
	}
}

// A child killed by a signal must report the POSIX/GNU shell convention
// (128+signal), not os/exec's raw ExitCode() of -1 for signaled processes.
func TestNiceReportsSignalExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix signals only")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found")
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Env: []string{"PATH=/usr/bin:/bin"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := runCommand(rc, "nice", []string{"sh", "-c", "kill -TERM $$"}, nil, currentPriority())
	const sigterm = 15
	if want := 128 + sigterm; code != want {
		t.Fatalf("code=%d want %d (stderr=%q)", code, want, errb.String())
	}
}
