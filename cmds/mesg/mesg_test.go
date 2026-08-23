package mesgcmd

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func withFakeTTY(t *testing.T, mode fs.FileMode) (path string, restore func()) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "faketty")
	if err := os.WriteFile(p, nil, mode); err != nil {
		t.Fatal(err)
	}
	// WriteFile's mode is masked by the process umask, which strips group write
	// on a default 022 - the exact bit under test. Set it explicitly.
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	oldTTY, oldStat, oldChmod := ttyName, statFn, chmodFn
	ttyName = func(*tool.RunContext) (string, error) { return p, nil }
	statFn, chmodFn = os.Stat, os.Chmod
	return p, func() { ttyName, statFn, chmodFn = oldTTY, oldStat, oldChmod }
}

func exec(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, args) // run FIRST, then read
	return out.String(), errb.String(), code
}

// The exit status IS the answer, so `mesg` is usable in a conditional. A caller
// must not read 1 as failure, and a test that only checked stdout would let a
// wrong status through.
func TestQueryReportsStateThroughExitStatus(t *testing.T) {
	_, restore := withFakeTTY(t, 0o620)
	defer restore()
	out, _, code := exec(t)
	if code != 0 || !strings.Contains(out, "is y") {
		t.Errorf("group-writable terminal: got %q exit %d, want \"is y\" exit 0", out, code)
	}
	restore()

	_, restore2 := withFakeTTY(t, 0o600)
	defer restore2()
	out, _, code = exec(t)
	if code != 1 || !strings.Contains(out, "is n") {
		t.Errorf("non-writable terminal: got %q exit %d, want \"is n\" exit 1", out, code)
	}
}

func TestSetTogglesOnlyTheGroupWriteBit(t *testing.T) {
	p, restore := withFakeTTY(t, 0o640)
	defer restore()
	if _, e, code := exec(t, "y"); code != 0 {
		t.Fatalf("mesg y: exit %d %s", code, e)
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o660 {
		t.Errorf("mesg y should set only g+w: got %o want %o", fi.Mode().Perm(), 0o660)
	}
	// Denying reports the resulting state, which is exit 1 by the same rule as
	// the query form - not a failure.
	if _, _, code := exec(t, "n"); code != 1 {
		t.Errorf("mesg n should exit 1 (messages now denied), got %d", code)
	}
	fi, _ = os.Stat(p)
	if fi.Mode().Perm() != 0o640 {
		t.Errorf("mesg n should clear only g+w: got %o want %o", fi.Mode().Perm(), 0o640)
	}
}

func TestBadOperandAndExtraOperand(t *testing.T) {
	_, restore := withFakeTTY(t, 0o620)
	defer restore()
	if _, _, code := exec(t, "maybe"); code == 0 || code == 1 {
		t.Errorf("an invalid operand must be a usage error, got %d", code)
	}
	if _, _, code := exec(t, "y", "n"); code == 0 || code == 1 {
		t.Errorf("an extra operand must be a usage error, got %d", code)
	}
}
