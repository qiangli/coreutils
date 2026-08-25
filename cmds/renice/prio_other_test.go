//go:build !unix

package renicecmd

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func execReal(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, args)
	return out.String(), errb.String(), code
}

// Off unix, renice fails closed once — after parsing, before any ID is
// touched — with a diagnostic naming the platform. Usage errors and the
// universal options keep their cross-platform behavior.
func TestNonUnixFailsClosedAfterParsing(t *testing.T) {
	out, errs, code := execReal(t, "-n", "1", "-p", "12")
	if code != 1 || out != "" {
		t.Fatalf("code=%d out=%q, want silent stdout and status 1", code, out)
	}
	if !strings.Contains(errs, "not supported on "+runtime.GOOS) {
		t.Errorf("stderr %q must name the unsupported platform", errs)
	}
	if _, _, code := execReal(t, "-p", "12"); code != 2 {
		t.Error("usage errors must still be diagnosed as usage errors (exit 2)")
	}
	if out, _, code := execReal(t, "--help"); code != 0 || !strings.Contains(out, "renice") {
		t.Error("--help must still succeed")
	}
}
