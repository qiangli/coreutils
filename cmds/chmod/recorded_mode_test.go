package chmodcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// TestRecordModeWritesComputedMode pins the second half of the transition:
// every file chmod touches gets the mode it computed handed to the
// recorder, in full — including the setuid/setgid/sticky bits and the r/x
// bits that the Windows read-only attribute cannot express, which is the
// entire reason the recorder exists.
//
// The recorder is a seam here rather than a real ACL, so the contract is
// checked on every host and not only on the one platform that implements
// it.
func TestRecordModeWritesComputedMode(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	recorded := map[string]os.FileMode{}
	restore := recordMode
	recordMode = func(path string, mode os.FileMode) error {
		recorded[filepath.Base(path)] = mode
		return nil
	}
	t.Cleanup(func() { recordMode = restore })

	if _, errb, code := runTool(t, dir, "4751", "a", "b"); code != 0 {
		t.Fatalf("chmod exited %d: %s", code, errb)
	}
	want := os.FileMode(0o751) | os.ModeSetuid
	for _, name := range []string{"a", "b"} {
		if got, ok := recorded[name]; !ok {
			t.Errorf("no mode recorded for %s", name)
		} else if got != want {
			t.Errorf("recorded %v for %s, want %v", got, name, want)
		}
	}
}

// TestRecordModeFailureIsReported pins that a mode the platform cannot
// record is a failure of chmod, not a silent success: the command that
// reports "mode changed to 0751" must be the command that actually stored
// 0751 somewhere a later test -x can find it.
func TestRecordModeFailureIsReported(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	restore := recordMode
	recordMode = func(string, os.FileMode) error { return os.ErrPermission }
	t.Cleanup(func() { recordMode = restore })

	out, errb, code := runTool(t, dir, "-v", "755", "a")
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if want := "chmod: changing permissions of 'a'"; !strings.Contains(errb, want) {
		t.Errorf("stderr %q, want it to name the failure with %q", errb, want)
	}
	if out != "" {
		t.Errorf("stdout %q, want nothing — a mode that was not recorded was not changed", out)
	}
}

// TestChmodReadsRecordedMode pins that a symbolic MODE computes from the
// mode chmod last set, not from whatever the platform reports. On Unix the
// two are the same file mode; the assertion is that the command reads it
// through tool.Stat, so a host that keeps the mode elsewhere gets the same
// answer.
func TestChmodReadsRecordedMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, errb, code := runTool(t, dir, "u+x", "a"); code != 0 {
		t.Fatalf("chmod exited %d: %s", code, errb)
	}
	fi, err := tool.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got&0o100 == 0 {
		t.Errorf("mode after u+x is %04o, want the owner execute bit set", got)
	}
}
