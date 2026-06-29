package timecmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTime(t *testing.T, env []string, args ...string) (out, errOut string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &o, Err: &e},
	}
	code = cmd.Run(rc, args)
	return o.String(), e.String(), code
}

func TestTimePosixFormat(t *testing.T) {
	_, errOut, code := runTime(t, os.Environ(), "-p", "true")
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	for _, k := range []string{"real ", "user ", "sys "} {
		if !strings.Contains(errOut, k) {
			t.Errorf("-p report missing %q: %q", k, errOut)
		}
	}
}

func TestTimeExitStatusPropagates(t *testing.T) {
	if _, _, code := runTime(t, os.Environ(), "sh", "-c", "exit 7"); code != 7 {
		t.Errorf("exit %d, want 7 (command status propagates)", code)
	}
}

func TestTimeMissingCommand(t *testing.T) {
	if _, _, code := runTime(t, os.Environ(), "-v"); code == 0 {
		t.Error("missing command should be a usage error")
	}
}

func TestTimeFormatSpecifiers(t *testing.T) {
	out, errOut, _ := runTime(t, os.Environ(), "-f", "ELAPSED=%e CMD=%C EXIT=%x", "true")
	_ = out
	if !strings.Contains(errOut, "CMD=true") || !strings.Contains(errOut, "EXIT=0") {
		t.Errorf("-f specifiers not expanded: %q", errOut)
	}
}

func TestTimeAgenticBudgetTodo(t *testing.T) {
	// A zero budget guarantees "over budget" → the TODO fires.
	env := append(os.Environ(), "DHNT_AGENT=1")
	_, errOut, _ := runTime(t, env, "--budget", "0s", "--todo", "split this step", "true")
	if !strings.Contains(errOut, `"kind":"todo"`) || !strings.Contains(errOut, "split this step") {
		t.Errorf("expected an agent-mode TODO line, got %q", errOut)
	}
	// No budget ⇒ no TODO.
	_, errOut2, _ := runTime(t, os.Environ(), "true")
	if strings.Contains(errOut2, "todo") {
		t.Errorf("no budget should produce no TODO: %q", errOut2)
	}
}
