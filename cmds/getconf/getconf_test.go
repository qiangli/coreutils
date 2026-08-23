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
	sys, err := exec.LookPath("getconf")
	if err != nil {
		t.Skip("no system getconf to compare against")
	}
	for _, name := range []string{"PAGESIZE", "OPEN_MAX", "NGROUPS_MAX", "ARG_MAX", "CLK_TCK", "_POSIX_VERSION"} {
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
	sys, err := exec.LookPath("getconf")
	if err != nil {
		t.Skip("no system getconf")
	}
	dir := t.TempDir()
	for _, name := range []string{"NAME_MAX", "PATH_MAX", "PIPE_BUF"} {
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
		"_POSIX_PATH_MAX": "256", "_POSIX_OPEN_MAX": "20", "_XOPEN_VERSION": "700",
	} {
		got, _, code := runCmd(t, name)
		if code != 0 || got != want {
			t.Errorf("%s = %q (exit %d), want %q", name, got, code, want)
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
	if _, _, code := runCmd(t, "-v", "POSIX_V7_LP64_OFF64", "PAGESIZE"); code != 0 {
		t.Error("the environment this build targets must be accepted")
	}
}

func TestMissingOperand(t *testing.T) {
	if _, _, code := runCmd(t); code == 0 {
		t.Error("no operand must be a usage error")
	}
	_ = os.Getenv
}
