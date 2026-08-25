package getconfcmd

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runCmd(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, args)
	return strings.TrimSpace(out.String()), strings.TrimSpace(errb.String()), code
}

// The selector numbers differ between Linux and Darwin and are transcribed by
// hand, so they are checked against the host's OWN getconf. A wrong number
// yields a plausible-looking wrong integer, which no self-consistent test
// would catch.
func TestAgreesWithSystemGetconf(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("host libc differential is covered by the Darwin adapter")
	}
	sys, err := exec.LookPath("getconf")
	if err != nil {
		t.Skip("no system getconf to compare against")
	}
	for _, name := range []string{
		"PAGESIZE", "OPEN_MAX", "NGROUPS_MAX", "ARG_MAX", "CLK_TCK",
		"BC_BASE_MAX", "BC_STRING_MAX", "INT_MAX", "LINE_MAX", "RE_DUP_MAX",
		"SYMLOOP_MAX", "_POSIX_VERSION",
	} {
		want, err := exec.Command(sys, name).Output()
		if err != nil {
			continue
		}
		expect := strings.TrimSpace(string(want))
		got, _, code := runCmd(t, name)
		if code != 0 {
			t.Errorf("%s: exit %d, system getconf says %q", name, code, expect)
			continue
		}
		if got != expect {
			// ARG_MAX and OPEN_MAX track LIVE rlimits, which legitimately differ
			// between two processes (Go raises RLIMIT_NOFILE at startup); the
			// rest must match exactly.
			if name == "ARG_MAX" || name == "OPEN_MAX" {
				t.Logf("%s: ours %q, system %q (rlimit-derived, informational)", name, got, expect)
				continue
			}
			t.Errorf("%s: ours %q, system %q", name, got, expect)
		}
	}
}

func TestPathconfAgreesWithSystem(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("host libc differential is covered by the Darwin adapter")
	}
	sys, err := exec.LookPath("getconf")
	if err != nil {
		t.Skip("no system getconf")
	}
	dir := t.TempDir()
	for _, name := range []string{
		"LINK_MAX", "MAX_CANON", "MAX_INPUT", "NAME_MAX", "PATH_MAX", "PIPE_BUF",
		"_POSIX_CHOWN_RESTRICTED", "_POSIX_NO_TRUNC", "_POSIX_VDISABLE",
		"FILESIZEBITS", "POSIX2_SYMLINKS", "POSIX_ALLOC_SIZE_MIN",
		"POSIX_REC_INCR_XFER_SIZE", "POSIX_REC_MAX_XFER_SIZE", "POSIX_REC_MIN_XFER_SIZE",
		"POSIX_REC_XFER_ALIGN", "SYMLINK_MAX", "_POSIX_ASYNC_IO", "_POSIX_PRIO_IO", "_POSIX_SYNC_IO",
	} {
		want, err := exec.Command(sys, name, dir).Output()
		if err != nil {
			continue
		}
		expect := strings.TrimSpace(string(want))
		got, _, code := runCmd(t, name, dir)
		if code != 0 || got != expect {
			t.Errorf("%s %s: ours %q (exit %d), system %q", name, dir, got, code, expect)
		}
	}
}

func TestDarwinConfstrAdapterMatchesEveryQueryableValue(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin adapter test")
	}
	for _, name := range confstrVars {
		want, err := exec.Command("getconf", name).Output()
		if err != nil { // Darwin deliberately has no V7 confstr environment.
			continue
		}
		got, _, code := runCmd(t, name)
		if code != 0 || got != strings.TrimSpace(string(want)) {
			t.Errorf("%s: ours %q (exit %d), host %q", name, got, code, strings.TrimSpace(string(want)))
		}
	}
}

// POSIX distinguishes "this variable has no limit" from "there is no such
// variable". Collapsing them would let a caller treat a typo as unlimited.
func TestUnknownVariableIsAnErrorNotUndefined(t *testing.T) {
	out, errs, code := runCmd(t, "NO_SUCH_VARIABLE_XYZ")
	if code == 0 {
		t.Fatalf("unknown variable must not exit 0 (got %q)", out)
	}
	if out == undefined {
		t.Fatal("unknown variable must not be reported as undefined")
	}
	if !strings.Contains(errs, "NO_SUCH_VARIABLE_XYZ") {
		t.Errorf("error should name the variable, got %q", errs)
	}
}

func TestCompileTimeMinimumsComeFromTheStandard(t *testing.T) {
	// These are constants in the specification, not host measurements, so they
	// must be identical on every platform including Windows.
	for name, want := range map[string]string{
		"_POSIX_PATH_MAX": "256", "_POSIX_OPEN_MAX": "20",
	} {
		got, _, code := runCmd(t, name)
		if code != 0 || got != want {
			t.Errorf("%s = %q (exit %d), want %q", name, got, code, want)
		}
	}
}

func TestDarwinAdapterMatchesEverySafelyQueryableValue(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin adapter test")
	}
	// The map is the set of Darwin ABI constants we claim.  Every one is
	// checked against the host libc so a SDK transcription cannot become a
	// self-consistent but wrong value.
	for _, name := range platformDifferentialNames() {
		want, err := exec.Command("getconf", name).Output()
		if err != nil {
			t.Errorf("host rejects claimed Darwin name %s: %v", name, err)
			continue
		}
		got, _, code := runCmd(t, name)
		if code != 0 || got != strings.TrimSpace(string(want)) {
			t.Errorf("%s: ours %q (exit %d), host %q", name, got, code, strings.TrimSpace(string(want)))
		}
	}
	for _, name := range []string{"ARG_MAX", "CHILD_MAX", "NGROUPS_MAX", "OPEN_MAX", "PAGESIZE", "PAGE_SIZE", "_NPROCESSORS_CONF", "_NPROCESSORS_ONLN"} {
		want, err := exec.Command("getconf", name).Output()
		if err != nil {
			t.Fatal(err)
		}
		got, _, code := runCmd(t, name)
		if code != 0 || got != strings.TrimSpace(string(want)) {
			t.Errorf("%s: ours %q (exit %d), host %q", name, got, code, strings.TrimSpace(string(want)))
		}
	}
}

func TestEveryInventoryNameHasAValueClass(t *testing.T) {
	for name := range sysVars {
		got, _, code := runCmd(t, name)
		if code != 0 || got == "" {
			t.Errorf("system inventory %s: output %q, exit %d", name, got, code)
		}
	}
	for name := range pathVars {
		got, _, code := runCmd(t, name, ".")
		if code != 0 || got == "" {
			t.Errorf("path inventory %s: output %q, exit %d", name, got, code)
		}
	}
	for _, name := range confstrVars {
		got, _, code := runCmd(t, name)
		if code != 0 {
			t.Errorf("confstr inventory %s: exit %d", name, code)
		}
		if got == undefined && runtime.GOOS == "darwin" && name == "PATH" {
			t.Errorf("PATH unexpectedly undefined")
		}
	}
}

func TestDarwinRegressionValues(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin only")
	}
	for name, want := range map[string]string{
		"ATEXIT_MAX": "2147483647", "IOV_MAX": "1024", "LOGIN_NAME_MAX": "255",
		"PTHREAD_STACK_MIN": "16384", "STREAM_MAX": "2560", "TTY_NAME_MAX": "255",
		"TZNAME_MAX": "255", "MB_LEN_MAX": "6",
	} {
		got, _, code := runCmd(t, name)
		if code != 0 || got != want {
			t.Errorf("%s = %q (exit %d), want %q", name, got, code, want)
		}
	}
}

func TestWindowsFailsClosed(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	for _, name := range []string{"PATH", "_POSIX_VERSION", "_POSIX2_VERSION", "_XOPEN_VERSION", "_POSIX_V6_LP64_OFF64", "_POSIX_SAVED_IDS"} {
		got, _, code := runCmd(t, name)
		if code != 0 || got != undefined {
			t.Errorf("%s = %q (exit %d), want undefined", name, got, code)
		}
	}
}

func TestLinuxDoesNotInventLibcValues(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	for _, name := range []string{"ARG_MAX", "CLK_TCK", "PATH", "_POSIX_VERSION", "_POSIX2_VERSION", "_XOPEN_VERSION", "RE_DUP_MAX", "SYMLOOP_MAX"} {
		got, _, code := runCmd(t, name)
		if code != 0 || got != undefined {
			t.Errorf("%s = %q (exit %d), want undefined without a libc adapter", name, got, code)
		}
	}
}

func TestAllListsAndDoesNotTakeOperands(t *testing.T) {
	out, _, code := runCmd(t, "-a")
	if code != 0 {
		t.Fatalf("-a exit %d", code)
	}
	if !strings.Contains(out, "PAGESIZE") {
		t.Error("-a should list PAGESIZE")
	}
	if _, _, code := runCmd(t, "-a", "extra"); code == 0 {
		t.Error("-a with an operand must be a usage error")
	}
}

func TestUnsupportedSpecificationIsRefused(t *testing.T) {
	if _, _, code := runCmd(t, "-v", "POSIX_V7_ILP32_OFF32", "PAGESIZE"); code == 0 {
		if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
			t.Error("a 64-bit build must not claim an ILP32 environment")
		}
	}
	if runtime.GOOS == "darwin" {
		if _, _, code := runCmd(t, "-v", "POSIX_V6_LP64_OFF64", "PAGESIZE"); code != 0 {
			t.Error("Darwin V6 LP64 environment must be accepted")
		}
		if _, _, code := runCmd(t, "-v", "POSIX_V7_LP64_OFF64", "PAGESIZE"); code == 0 {
			t.Error("Darwin must not claim V7")
		}
	}
}

func TestPOSIXArityAndOptionForms(t *testing.T) {
	dir := t.TempDir()
	if out, errb, code := runCmd(t, "-v", "", "_POSIX_PATH_MAX"); code != 0 || out != "256" || errb != "" {
		t.Fatalf("empty -v specification should fall through: (%q, %q, %d)", out, errb, code)
	}
	if out, errb, code := runCmd(t, "NAME_MAX", dir); code != 0 || out == "" || errb != "" {
		t.Fatalf("path_var pathname = (%q, %q, %d), want value", out, errb, code)
	}
	if _, errb, code := runCmd(t, "NAME_MAX"); code != 2 || !strings.Contains(errb, "NAME_MAX") {
		t.Fatalf("path_var without pathname = (%q, %d), want unknown system variable usage error", errb, code)
	}
	if _, errb, code := runCmd(t, "_POSIX_PATH_MAX", dir); code != 2 || !strings.Contains(errb, "_POSIX_PATH_MAX") {
		t.Fatalf("system_var with pathname = (%q, %d), want unknown path variable usage error", errb, code)
	}
	if _, errb, code := runCmd(t, "-v"); code != 2 || !strings.Contains(errb, "flag needs an argument") {
		t.Fatalf("missing -v argument = (%q, %d), want option-argument error", errb, code)
	}
}

func TestMissingOperand(t *testing.T) {
	if _, _, code := runCmd(t); code == 0 {
		t.Error("no operand must be a usage error")
	}
	_ = os.Getenv
}
