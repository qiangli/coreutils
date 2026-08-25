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

func TestLinuxProgrammingEnvironmentMatchesUbuntuOracle(t *testing.T) {
	if !linuxLP64Build() {
		t.Skip("this Linux target does not claim an LP64/OFF64 C environment")
	}
	system, err := exec.LookPath("getconf")
	if err != nil {
		t.Skip("no independent host getconf oracle")
	}
	for _, version := range []string{"V6", "V7"} {
		specification := "POSIX_" + version + "_LP64_OFF64"
		query := "_" + specification
		wantBytes, err := exec.Command(system, query).Output()
		if err != nil {
			t.Fatalf("host getconf %s: %v", query, err)
		}
		if want := strings.TrimSpace(string(wantBytes)); want != "1" {
			t.Fatalf("host %s=%q, certification image does not advertise expected environment", query, want)
		}
		if got, stderr, code := runCmd(t, query); code != 0 || stderr != "" || got != "1" {
			t.Errorf("%s = (%q, %q, %d), want (1, empty, 0)", query, got, stderr, code)
		}
		plain, plainErr, plainCode := runCmd(t, "ARG_MAX")
		got, stderr, code := runCmd(t, "-v", specification, "ARG_MAX")
		if code != plainCode || stderr != plainErr || got != plain {
			t.Errorf("-v %s ARG_MAX = (%q, %q, %d), default (%q, %q, %d)", specification, got, stderr, code, plain, plainErr, plainCode)
		}
		dir := t.TempDir()
		plain, plainErr, plainCode = runCmd(t, "NAME_MAX", dir)
		got, stderr, code = runCmd(t, "-v", specification, "NAME_MAX", dir)
		if code != plainCode || stderr != plainErr || got != plain {
			t.Errorf("-v %s NAME_MAX = (%q, %q, %d), default (%q, %q, %d)", specification, got, stderr, code, plain, plainErr, plainCode)
		}
	}
}

func TestLinuxDoesNotClaimOtherProgrammingEnvironments(t *testing.T) {
	for _, shape := range []string{"ILP32_OFF32", "ILP32_OFFBIG", "LPBIG_OFFBIG"} {
		for _, version := range []string{"V6", "V7"} {
			specification := "POSIX_" + version + "_" + shape
			if got, stderr, code := runCmd(t, "_"+specification); code != 0 || stderr != "" || got != undefined {
				t.Errorf("_%s = (%q, %q, %d), want undefined", specification, got, stderr, code)
			}
			if out, stderr, code := runCmd(t, "-v", specification, "ARG_MAX"); code == 0 || out != "" || !strings.Contains(stderr, "unsupported specification") {
				t.Errorf("-v %s = (%q, %q, %d), want rejection", specification, out, stderr, code)
			}
		}
	}
}
