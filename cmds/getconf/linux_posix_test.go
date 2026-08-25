//go:build linux

package getconfcmd

import (
	"os/exec"
	"strings"
	"testing"
)

// The Linux adapter deliberately avoids libc and derives only values exposed
// by kernel interfaces. Compare that bounded set with the host's independent
// libc implementation so a plausible but wrong selector or formula fails.
func TestLinuxDerivedValuesMatchHostGetconf(t *testing.T) {
	system, err := exec.LookPath("getconf")
	if err != nil {
		t.Skip("no independent host getconf oracle")
	}
	for _, name := range []string{
		"ARG_MAX", "CHILD_MAX", "CLK_TCK", "NGROUPS_MAX", "OPEN_MAX",
		"PAGESIZE", "PAGE_SIZE", "SIGQUEUE_MAX", "_NPROCESSORS_CONF",
		"_NPROCESSORS_ONLN",
	} {
		wantBytes, err := exec.Command(system, name).Output()
		if err != nil {
			t.Fatalf("host getconf %s: %v", name, err)
		}
		want := strings.TrimSpace(string(wantBytes))
		got, stderr, code := runCmd(t, name)
		if code != 0 || stderr != "" || got != want {
			t.Errorf("%s = (%q, %q, %d), host getconf %q", name, got, stderr, code, want)
		}
	}
}

func TestLinuxDerivedPathValuesMatchHostGetconf(t *testing.T) {
	system, err := exec.LookPath("getconf")
	if err != nil {
		t.Skip("no independent host getconf oracle")
	}
	dir := t.TempDir()
	for _, name := range []string{
		"MAX_CANON", "MAX_INPUT", "NAME_MAX", "PATH_MAX", "PIPE_BUF",
		"_POSIX_CHOWN_RESTRICTED", "_POSIX_NO_TRUNC", "_POSIX_VDISABLE",
		"POSIX2_SYMLINKS", "POSIX_ALLOC_SIZE_MIN", "POSIX_REC_MIN_XFER_SIZE",
		"POSIX_REC_XFER_ALIGN",
	} {
		wantBytes, err := exec.Command(system, name, dir).Output()
		if err != nil {
			t.Fatalf("host getconf %s: %v", name, err)
		}
		want := strings.TrimSpace(string(wantBytes))
		got, stderr, code := runCmd(t, name, dir)
		if code != 0 || stderr != "" || got != want {
			t.Errorf("%s = (%q, %q, %d), host getconf %q", name, got, stderr, code, want)
		}
	}
}

func TestLinuxTimestampResolutionIsExactOrUndefined(t *testing.T) {
	dir := t.TempDir()
	want := undefined
	if linuxNanosecondFilesystem(dir) {
		want = "1"
	}
	got, stderr, code := runCmd(t, "_POSIX_TIMESTAMP_RESOLUTION", dir)
	if code != 0 || stderr != "" || got != want {
		t.Fatalf("timestamp resolution = (%q, %q, %d), want %q", got, stderr, code, want)
	}
}
