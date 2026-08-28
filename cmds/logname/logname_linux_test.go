//go:build linux

package lognamecmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoginNameFromRecordedTerminalSession(t *testing.T) {
	dir := t.TempDir()
	utmp := filepath.Join(dir, "utmp")
	data := []byte("other pts/1 1 host\nvsctester pts/7 2 host\n")
	if err := os.WriteFile(utmp, data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tty := range []string{"pts/7", "/dev/pts/7"} {
		if got := loginNameFromSession(utmp, tty, []string{"POSIXLY_CORRECT=1"}); got != "vsctester" {
			t.Fatalf("loginNameFromSession(%q) = %q, want vsctester", tty, got)
		}
	}
	if got := loginNameFromSession(utmp, "/dev/pts/9", []string{"POSIXLY_CORRECT=1"}); got != "" {
		t.Fatalf("unrecorded terminal resolved to %q", got)
	}
}
