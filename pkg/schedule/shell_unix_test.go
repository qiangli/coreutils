//go:build unix

package schedule

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSavedUmaskUsesBoundShellReference(t *testing.T) {
	original := shellPath
	t.Cleanup(func() { shellPath = original })
	shellPath = filepath.Join(t.TempDir(), "declared-go-sh")
	cmd := commandWithUmask([]string{"job", "argument with spaces"}, 0027, true)
	if cmd.Path != shellPath {
		t.Fatalf("wrapper path=%q, want %q", cmd.Path, shellPath)
	}
	want := []string{shellPath, "-c", `umask "$1"; shift; exec "$@"`, "bashy-schedule", "0027", "job", "argument with spaces"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("wrapper args=%q, want %q", cmd.Args, want)
	}
	// An unavailable declared shell must fail instead of trying the host shell.
	if err := cmd.Run(); !os.IsNotExist(err) {
		t.Fatalf("missing declared shell: %v", err)
	}
	direct := commandWithUmask([]string{"job", "argument with spaces"}, 0027, false)
	if !reflect.DeepEqual(direct.Args, []string{"job", "argument with spaces"}) {
		t.Fatalf("unwrapped args=%q", direct.Args)
	}
}

func TestBoundShellRunsSavedUmask(t *testing.T) {
	cmd := commandWithUmask([]string{DefaultShellPath(), "-c", `printf '%s\n' "$1"; umask`, "job", "argument with spaces"}, 0027, true)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("declared shell %q: %v: %s", DefaultShellPath(), err, out)
	}
	if string(out) != "argument with spaces\n0027\n" {
		t.Fatalf("saved umask or argv changed: %q", out)
	}
}
