//go:build windows

package envcmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// Windows has no POSIX signal wait-status model, so the "raise"/"boundary"
// helper roles the unix tests use do not apply. These stubs exist only so the
// shared helperMain switch compiles for GOOS=windows.
func helperSelfRaise(string) {
	fmt.Fprintln(os.Stderr, "helper: raise unsupported on windows")
	os.Exit(2)
}
func helperStandaloneBoundary(string) {
	fmt.Fprintln(os.Stderr, "helper: boundary unsupported on windows")
	os.Exit(2)
}

// Explicit Windows behavior: a COMMAND that fails without an exit code yields
// GNU's 125 and leaves RunContext.ExitSignal at zero — there is no signal for a
// standalone boundary to re-raise, so the process always exits normally.
func TestEnvExecWindowsLeavesExitSignalUnset(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   []string{"PATH="},
		FS:    tool.NewLocalFS(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	// A directory named as COMMAND is found-but-not-invokable (126); the point
	// is only that no signal outcome is recorded on Windows.
	dir := t.TempDir()
	if err := os.Mkdir(dir+string(os.PathSeparator)+"cmddir", 0o755); err != nil {
		t.Fatal(err)
	}
	rc.Dir = dir
	_ = cmd.Run(rc, []string{"cmddir"})
	if rc.ExitSignal != 0 {
		t.Errorf("rc.ExitSignal = %d on Windows, want 0", rc.ExitSignal)
	}
}
