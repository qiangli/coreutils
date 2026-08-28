//go:build linux

package getconfcmd

import (
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
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
	names := []string{
		"MAX_CANON", "MAX_INPUT", "NAME_MAX", "PATH_MAX", "PIPE_BUF",
		"_POSIX_CHOWN_RESTRICTED", "_POSIX_NO_TRUNC", "_POSIX_VDISABLE",
		"POSIX2_SYMLINKS", "POSIX_ALLOC_SIZE_MIN", "POSIX_REC_MIN_XFER_SIZE",
		"POSIX_REC_XFER_ALIGN",
	}
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		t.Fatalf("statfs temporary directory: %v", err)
	}
	if _, ok := linuxFileSizeBits(int64(st.Type)); ok {
		names = append(names, "FILESIZEBITS")
	}
	for _, name := range names {
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

func TestLinuxFileSizeBitsMapping(t *testing.T) {
	tests := []struct {
		name   string
		fsType int64
		want   string
		ok     bool
	}{
		{name: "ext family", fsType: unix.EXT4_SUPER_MAGIC, want: "64", ok: true},
		{name: "unknown filesystem is not inferred", fsType: 0x12345678},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := linuxFileSizeBits(tt.fsType)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("linuxFileSizeBits(%#x) = (%q, %v), want (%q, %v)", tt.fsType, got, ok, tt.want, tt.ok)
			}
		})
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

// TestLinuxPathReportsStandardUtilityPath pins the `command -p` product
// contract: getconf PATH is the queryable form of the standard-utility search
// path, so it must report the stable path the product guarantees rather than
// undefined. The host-oracle tests deliberately exclude PATH — glibc's
// confstr(_CS_PATH) "/bin:/usr/bin" is a libc policy, not this guarantee.
func TestLinuxPathReportsStandardUtilityPath(t *testing.T) {
	got, stderr, code := runCmd(t, "PATH")
	if code != 0 || stderr != "" || got != standardUtilsPath {
		t.Fatalf("PATH = (%q, stderr %q, exit %d), want %q", got, stderr, code, standardUtilsPath)
	}
}

// TestLinuxAllIncludesStandardUtilityPath keeps the -a extension honest: it
// must list PATH exactly once, with the same value the single-variable query
// reports.
func TestLinuxAllIncludesStandardUtilityPath(t *testing.T) {
	out, stderr, code := runCmd(t, "-a")
	if code != 0 || stderr != "" {
		t.Fatalf("-a = (exit %d, stderr %q)", code, stderr)
	}
	found := 0
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != "PATH" {
			continue
		}
		found++
		if fields[1] != standardUtilsPath {
			t.Errorf("-a PATH = %q, want %q", fields[1], standardUtilsPath)
		}
	}
	if found != 1 {
		t.Errorf("-a listed PATH %d times, want exactly 1", found)
	}
}
