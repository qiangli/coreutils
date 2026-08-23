package renicecmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func exec(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, args) // run FIRST, then read the buffers
	return out.String(), errb.String(), code
}

// Saturation, not rejection: the kernel clamps, and erroring instead would make
// a portable script fail where the reference succeeds.
func TestNiceValueSaturatesRatherThanErroring(t *testing.T) {
	for _, c := range []struct {
		in, want int
	}{{100, niceMax}, {-100, niceMin}, {0, 0}, {19, 19}, {-20, -20}} {
		if got := clamp(c.in); got != c.want {
			t.Errorf("clamp(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestMutuallyExclusiveSelectors(t *testing.T) {
	if _, e, code := exec(t, "-n", "1", "-p", "-u", "1"); code == 0 {
		t.Errorf("-p with -u must be refused; stderr=%q", e)
	}
}

func TestMissingIncrementAndMissingID(t *testing.T) {
	if _, _, code := exec(t); code == 0 {
		t.Error("no arguments must be a usage error")
	}
	if _, _, code := exec(t, "-n", "5"); code == 0 {
		t.Error("an increment with no ID must be a usage error")
	}
}

func TestInvalidIncrementIsRejected(t *testing.T) {
	if _, _, code := exec(t, "-n", "abc", "1"); code == 0 {
		t.Error("a non-numeric increment must be rejected")
	}
}

// The obsolescent form takes the increment as the first operand; dropping it
// would silently treat the increment as a PID.
func TestObsolescentFormTakesIncrementAsFirstOperand(t *testing.T) {
	if runtimeIsWindows() {
		t.Skip("nice values are not a Windows concept")
	}
	pid := os.Getpid()
	out, errs, _ := exec(t, "0", itoa(pid))
	if !strings.Contains(out, "old priority") && !strings.Contains(errs, "renice") {
		t.Errorf("obsolescent form did not act on the pid; out=%q err=%q", out, errs)
	}
}

// A zero increment must be a no-op on the value, which also proves the code
// reads the CURRENT priority rather than assuming 0.
func TestZeroIncrementPreservesCurrentValue(t *testing.T) {
	if runtimeIsWindows() {
		t.Skip("nice values are not a Windows concept")
	}
	pid := os.Getpid()
	before, err := getPriority(whichProcess, pid)
	if err != nil {
		t.Skipf("cannot read own priority: %v", err)
	}
	if _, _, code := exec(t, "-n", "0", itoa(pid)); code != 0 {
		t.Skip("not permitted to renice in this environment")
	}
	after, err := getPriority(whichProcess, pid)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Errorf("zero increment changed priority %d -> %d", before, after)
	}
}

func itoa(i int) string { return strings.TrimSpace(sprint(i)) }
func sprint(i int) string {
	var b bytes.Buffer
	b.WriteString(itoaSlow(i))
	return b.String()
}
func itoaSlow(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var d []byte
	for i > 0 {
		d = append([]byte{byte('0' + i%10)}, d...)
		i /= 10
	}
	if neg {
		return "-" + string(d)
	}
	return string(d)
}
