//go:build unix

package filecmd

import (
	"fmt"
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSpecialDeviceNumbers(t *testing.T) {
	info, err := os.Stat("/dev/null")
	if err != nil {
		t.Skipf("/dev/null unavailable: %v", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("/dev/null stat lacks Unix metadata")
	}
	device := uint64(stat.Rdev)
	wantType := fmt.Sprintf("character special (%d/%d)", unix.Major(device), unix.Minor(device))

	out, errOut, code := invoke(t, t.TempDir(), "", "/dev/null")
	want := "/dev/null: " + wantType + "\n"
	if code != 0 || errOut != "" || out != want {
		t.Fatalf("out=%q err=%q code=%d want=%q", out, errOut, code, want)
	}

	out, errOut, code = invoke(t, t.TempDir(), "", "-b", "/dev/null")
	if code != 0 || errOut != "" || out != wantType+"\n" {
		t.Fatalf("brief out=%q err=%q code=%d want=%q", out, errOut, code, wantType+"\n")
	}
}
