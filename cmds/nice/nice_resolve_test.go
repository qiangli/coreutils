package nicecmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runNiceEnv(t *testing.T, env []string, args ...string) (out, errOut string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   env,
		Stdio: tool.Stdio{Out: &o, Err: &e},
	}
	code = cmd.Run(rc, args)
	return o.String(), e.String(), code
}

// PATH-unset must fall back to the default search path and find `echo`. This
// was nice's specific failure in the cluster (rc.Getenv conflated unset with
// empty, so it searched only the working directory).
func TestNicePathUnsetFindsCommand(t *testing.T) {
	out, _, code := runNiceEnv(t, []string{"HOME=/tmp"}, "echo", "hi")
	if code != 0 {
		t.Fatalf("PATH-unset nice echo: code=%d, want 0", code)
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("PATH-unset nice echo output=%q, want it to contain hi", out)
	}
}

// nice propagates the command's own exit status (not remapped).
func TestNiceChildExitPropagates(t *testing.T) {
	_, _, code := runNiceEnv(t, []string{"PATH=/bin:/usr/bin"}, "sh", "-c", "exit 7")
	if code != 7 {
		t.Errorf("child exit 7: nice code=%d, want 7", code)
	}
}

// `--` ends nice's options so the command and its flags are operands.
func TestNiceDoubleDashStopsOptionParsing(t *testing.T) {
	out, _, code := runNiceEnv(t, []string{"PATH=/bin:/usr/bin"}, "--", "echo", "kept")
	if code != 0 {
		t.Fatalf("nice -- echo: code=%d, want 0", code)
	}
	if !strings.Contains(out, "kept") {
		t.Errorf("nice -- output=%q", out)
	}
}

// `--` must stop option parsing even after an adjustment, so a command that
// looks like an option (-5) is run as the command rather than parsed as -n.
func TestNiceDoubleDashGuardsCommandStartingWithDash(t *testing.T) {
	_, _, code := runNiceEnv(t, []string{"PATH=/bin:/usr/bin"}, "-n", "5", "--", "true")
	if code != 0 {
		t.Errorf("nice -n5 -- true: code=%d, want 0", code)
	}
}
