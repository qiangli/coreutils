//go:build linux

package pathchkcmd

import (
	"os"
	"testing"
)

// Linux filesystems such as ext4 accept arbitrary non-NUL, non-slash bytes in
// a component. A UTF-8 locale must not impose an extra storage restriction:
// validity is decided by the containing filesystem, not LC_CTYPE.
func TestIssue741LinuxAcceptsFilesystemValidNonUTF8Name(t *testing.T) {
	dir := t.TempDir()
	name := "bad-" + string([]byte{0xff})
	if err := os.WriteFile(dir+"/"+name, []byte("x"), 0o600); err != nil {
		t.Skipf("containing filesystem rejects the byte sequence: %v", err)
	}
	for _, env := range [][]string{{"LC_ALL=C.UTF-8"}, {"LC_ALL=de_DE.UTF-8"}} {
		code, errText := runPathchkEnv(t, dir, env, name)
		if code != 0 || errText != "" {
			t.Fatalf("env=%q code=%d stderr=%q", env, code, errText)
		}
	}
}
