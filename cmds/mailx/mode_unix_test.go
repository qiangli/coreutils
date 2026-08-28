//go:build unix

package mailxcmd

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPOSIXRecordCreationModeHonorsUmask(t *testing.T) {
	old := syscall.Umask(0o024)
	t.Cleanup(func() { syscall.Umask(old) })

	_, stderr, code, dir := invoke(t, "body\n", "-F", "bob")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	for path, want := range map[string]os.FileMode{
		filepath.Join(dir, "bob"):          0o642,
		filepath.Join(dir, "spool", "bob"): 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %04o, want %04o", path, got, want)
		}
	}
}
