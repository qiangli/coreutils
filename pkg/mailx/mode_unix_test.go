//go:build unix

package mailx

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestAppendMboxWithModeHonorsUmask(t *testing.T) {
	old := syscall.Umask(0o024)
	t.Cleanup(func() { syscall.Umask(old) })

	path := filepath.Join(t.TempDir(), "record")
	msg := &Message{Headers: []Header{{Name: "Subject", Value: "record"}}, Body: []byte("body\n")}
	if err := AppendMboxWithMode(path, "alice", time.Unix(0, 0), msg, 0o666); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o642 {
		t.Fatalf("record mode = %04o, want 0642", got)
	}
}
