package renicecmd

import (
	"bytes"
	"errors"
	"fmt"
	"os/user"
	"strconv"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// fakeSched is the hermetic side of the scheduler seam: it records every
// call in order and emulates POSIX setpriority() clamping, so grammar,
// which-dispatch, ordering, and failure handling are tested without
// touching live processes.
type schedKey struct{ which, id int }

const (
	fakeMin = -20
	fakeMax = 19
)

type fakeSched struct {
	nice      map[schedKey]int
	getErr    map[schedKey]error
	setErr    map[schedKey]error
	membersBy map[schedKey][]int
	memberErr map[schedKey]error
	calls     []string
}

func newFake() *fakeSched {
	return &fakeSched{
		nice:      map[schedKey]int{},
		getErr:    map[schedKey]error{},
		setErr:    map[schedKey]error{},
		membersBy: map[schedKey][]int{},
		memberErr: map[schedKey]error{},
	}
}

func whichName(which int) string {
	switch which {
	case whichProcess:
		return "proc"
	case whichPGroup:
		return "pgrp"
	case whichUser:
		return "user"
	}
	return "?"
}

func (f *fakeSched) get(which, id int) (int, error) {
	f.calls = append(f.calls, fmt.Sprintf("get %s %d", whichName(which), id))
	k := schedKey{which, id}
	if err := f.getErr[k]; err != nil {
		return 0, err
	}
	return f.nice[k], nil
}

func (f *fakeSched) set(which, id, prio int) error {
	// POSIX setpriority(): an out-of-range nice value is clamped to the
	// exceeded limit, never rejected.
	if prio > fakeMax {
		prio = fakeMax
	}
	if prio < fakeMin {
		prio = fakeMin
	}
	f.calls = append(f.calls, fmt.Sprintf("set %s %d %d", whichName(which), id, prio))
	k := schedKey{which, id}
	if err := f.setErr[k]; err != nil {
		return err
	}
	f.nice[k] = prio
	return nil
}

func (f *fakeSched) members(which, id int) ([]int, error) {
	f.calls = append(f.calls, fmt.Sprintf("members %s %d", whichName(which), id))
	k := schedKey{which, id}
	if err := f.memberErr[k]; err != nil {
		return nil, err
	}
	if pids, ok := f.membersBy[k]; ok {
		return append([]int(nil), pids...), nil
	}
	return []int{id}, nil
}

func execFake(t *testing.T, f *fakeSched, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := runWith(rc, args, f, nil)
	return out.String(), errb.String(), code
}

func execFakeDedicated(t *testing.T, f *fakeSched, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:              t.TempDir(),
		DedicatedProcess: true,
		Stdio:            tool.Stdio{Out: &out, Err: &errb},
	}
	code := runWith(rc, args, f, nil)
	return out.String(), errb.String(), code
}

func TestMissingIncrementOptionIsUsageError(t *testing.T) {
	for _, args := range [][]string{{}, {"-p", "12"}, {"12"}} {
		_, errs, code := execFake(t, newFake(), args...)
		if code != 2 {
			t.Errorf("renice %v: code=%d, want usage error 2", args, code)
		}
		if !strings.Contains(errs, "-n") {
			t.Errorf("renice %v: diagnostic must name -n; got %q", args, errs)
		}
	}
}

// The obsolescent `renice increment ID...` form is refused rather than
// guessed at: it was removed in Issue 6 and historically took an ABSOLUTE
// nice value, so either silent reading gives wrong answers.
func TestObsolescentFirstOperandFormIsRefusedLoudly(t *testing.T) {
	f := newFake()
	_, errs, code := execFake(t, f, "5", "12")
	if code != 2 {
		t.Fatalf("code=%d, want 2", code)
	}
	if !strings.Contains(errs, "obsolescent") {
		t.Errorf("diagnostic should explain the refused historical form; got %q", errs)
	}
	if len(f.calls) != 0 {
		t.Errorf("no scheduler call may happen on a usage error; got %v", f.calls)
	}
}

func TestIncrementForms(t *testing.T) {
	for _, c := range []struct {
		args []string
		want int
	}{
		{[]string{"-n", "5", "12"}, 5},
		{[]string{"-n5", "12"}, 5},
		{[]string{"-n", "-4", "12"}, -4},
		{[]string{"-n-4", "12"}, -4},
		{[]string{"-n", "+3", "12"}, 3},
		{[]string{"-n", "0", "12"}, 0},
	} {
		f := newFake()
		if _, errs, code := execFake(t, f, c.args...); code != 0 {
			t.Errorf("renice %v: code=%d stderr=%q", c.args, code, errs)
			continue
		}
		want := []string{"get proc 12", fmt.Sprintf("set proc 12 %d", c.want)}
		if got := strings.Join(f.calls, "; "); got != strings.Join(want, "; ") {
			t.Errorf("renice %v: calls %q, want %q", c.args, got, want)
		}
	}
}

func TestInvalidIncrementIsUsageError(t *testing.T) {
	for _, inc := range []string{"abc", "1.5", "", "+", "-", "5x"} {
		_, _, code := execFake(t, newFake(), "-n", inc, "12")
		if code != 2 {
			t.Errorf("increment %q: code=%d, want 2", inc, code)
		}
	}
}

func TestMissingIncrementArgumentAndMissingID(t *testing.T) {
	if _, _, code := execFake(t, newFake(), "-n"); code != 2 {
		t.Error("-n with no argument must be a usage error")
	}
	if _, _, code := execFake(t, newFake(), "-n", "5"); code != 2 {
		t.Error("an increment with no ID must be a usage error")
	}
}

func TestDuplicateIncrementIsRefusedAndLateIncrementIsAccepted(t *testing.T) {
	if _, _, code := execFake(t, newFake(), "-n", "1", "-n", "2", "12"); code != 2 {
		t.Error("a duplicate -n must be refused")
	}
	f := newFake()
	if _, errs, code := execFake(t, f, "12", "-n", "2", "13"); code != 0 {
		t.Fatalf("interspersed -n: code=%d stderr=%q", code, errs)
	}
	if got := strings.Join(f.calls, "; "); got != "get proc 12; set proc 12 2; get proc 13; set proc 13 2" {
		t.Errorf("late -n calls %q", got)
	}
	f = newFake()
	if _, errs, code := execFake(t, f, "12", "--increment=3"); code != 0 {
		t.Fatalf("interspersed --increment: code=%d stderr=%q", code, errs)
	}
}

// Exact which-dispatch with the Guideline 9 exemption: selectors are
// positional and re-interpret the operands that follow, -p is the default,
// and the synopsis order (selector before -n) also parses.
func TestPositionalSelectorSwitchingDispatchesExactWhich(t *testing.T) {
	f := newFake()
	f.membersBy[schedKey{whichPGroup, 20}] = []int{120}
	f.membersBy[schedKey{whichPGroup, 21}] = []int{121}
	f.membersBy[schedKey{whichUser, 54321}] = []int{221}
	_, errs, code := execFake(t, f, "-n", "1", "10", "-p", "11", "-g", "20", "21", "-u", "54321")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
	want := "get proc 10; set proc 10 1; get proc 11; set proc 11 1; " +
		"members pgrp 20; get proc 120; set proc 120 1; " +
		"members pgrp 21; get proc 121; set proc 121 1; " +
		"members user 54321; get proc 221; set proc 221 1"
	if got := strings.Join(f.calls, "; "); got != want {
		t.Errorf("calls:\n  got  %s\n  want %s", got, want)
	}
}

func TestSelectorBeforeIncrementMatchesSynopsis(t *testing.T) {
	f := newFake()
	if _, errs, code := execFake(t, f, "-g", "-n", "3", "7"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
	if got := strings.Join(f.calls, "; "); got != "members pgrp 7; get proc 7; set proc 7 3" {
		t.Errorf("calls %q, want pgrp dispatch", got)
	}
}

func TestClusteredOptionsParse(t *testing.T) {
	f := newFake()
	if _, errs, code := execFake(t, f, "-gn1", "7"); code != 0 {
		t.Fatalf("-gn1: code=%d stderr=%q", code, errs)
	}
	if got := strings.Join(f.calls, "; "); got != "members pgrp 7; get proc 7; set proc 7 1" {
		t.Errorf("-gn1 calls %q", got)
	}
	f = newFake()
	if _, _, code := execFake(t, f, "-pg", "-n", "1", "7"); code != 0 {
		t.Fatal("-pg must parse as two selectors")
	}
	if got := strings.Join(f.calls, "; "); got != "members pgrp 7; get proc 7; set proc 7 1" {
		t.Errorf("-pg calls %q, want the last selector to win for following operands", got)
	}
}

func TestDoubleDashEndsOptionsAndLoneDashIsOperand(t *testing.T) {
	// After --, "-p" is an operand and fails ID validation at runtime
	// (exit 1), not option parsing (exit 2).
	f := newFake()
	_, errs, code := execFake(t, f, "-n", "1", "--", "-p")
	if code != 1 {
		t.Errorf("post -- operand: code=%d stderr=%q, want 1", code, errs)
	}
	if !strings.Contains(errs, "invalid ID") {
		t.Errorf("stderr %q must report an invalid ID", errs)
	}
	if _, errs, code := execFake(t, newFake(), "-n", "1", "-"); code != 1 || !strings.Contains(errs, "invalid ID") {
		t.Errorf("lone dash: code=%d stderr=%q, want invalid-ID failure", code, errs)
	}
}

func TestUnknownOptionsAreUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"-x", "-n", "1", "12"},
		{"-5", "-p", "12"},
		{"--bogus", "-n", "1", "12"},
		{"--help=now"},     // universal long options take no value
		{"--n", "1", "12"}, // -n has no long form
	} {
		if _, _, code := execFake(t, newFake(), args...); code != 2 {
			t.Errorf("renice %v: code=%d, want usage error 2", args, code)
		}
	}
}

func TestRetainedLongOptionsAndHelpVersionAliases(t *testing.T) {
	f := newFake()
	f.membersBy[schedKey{whichPGroup, 7}] = []int{70}
	if _, errs, code := execFake(t, f, "--pgrp", "7", "--increment", "2"); code != 0 {
		t.Fatalf("retained long options: code=%d stderr=%q", code, errs)
	}
	if got := strings.Join(f.calls, "; "); got != "members pgrp 7; get proc 70; set proc 70 2" {
		t.Errorf("long-option calls %q", got)
	}
	for _, alias := range []string{"-h", "-V"} {
		if _, _, code := execFake(t, newFake(), alias); code != 0 {
			t.Errorf("%s must retain its universal success behavior", alias)
		}
	}
}

func TestInvalidIDsFailPerOperand(t *testing.T) {
	for _, id := range []string{"-1", "+1", "abc", "1.5", "4294967296"} {
		_, errs, code := execFake(t, newFake(), "-n", "0", "--", id)
		if code != 1 {
			t.Errorf("ID %q: code=%d, want 1", id, code)
		}
		if !strings.Contains(errs, "invalid ID") {
			t.Errorf("ID %q: stderr %q must report invalid ID", id, errs)
		}
	}
}

// Ordered mixed success/failure: every operand is attempted in command-line
// order, each failure gets its own stderr line, and the exit status is >0.
func TestOrderedMixedSuccessAndFailureContinues(t *testing.T) {
	f := newFake()
	f.getErr[schedKey{whichProcess, 7}] = errors.New("operation not permitted")
	f.setErr[schedKey{whichProcess, 9}] = errors.New("permission denied")
	_, errs, code := execFake(t, f, "-n", "2", "7", "8", "9")
	if code != 1 {
		t.Fatalf("code=%d, want 1", code)
	}
	want := "get proc 7; get proc 8; set proc 8 2; get proc 9; set proc 9 2"
	if got := strings.Join(f.calls, "; "); got != want {
		t.Errorf("calls:\n  got  %s\n  want %s", got, want)
	}
	sevenIdx := strings.Index(errs, "renice: 7: process 7: operation not permitted")
	nineIdx := strings.Index(errs, "renice: 9: process 9: permission denied")
	if sevenIdx < 0 || nineIdx < 0 || nineIdx < sevenIdx {
		t.Errorf("stderr must carry one ordered diagnostic per failure; got %q", errs)
	}
	if f.nice[schedKey{whichProcess, 8}] != 2 {
		t.Error("the operand between two failures must still be adjusted")
	}
}

// POSIX: STDOUT is not used — success is silent.
func TestSuccessIsSilentOnStdout(t *testing.T) {
	f := newFake()
	out, errs, code := execFake(t, f, "-n", "5", "12")
	if code != 0 || out != "" || errs != "" {
		t.Errorf("out=%q errs=%q code=%d, want silent success", out, errs, code)
	}
}

// The increment is applied to the CURRENT nice value, and out-of-range
// results are clamped by the scheduler (the seam emulates the kernel),
// not rejected.
func TestIncrementIsRelativeAndBoundsAreSchedulerClamped(t *testing.T) {
	f := newFake()
	f.nice[schedKey{whichProcess, 12}] = 10
	if _, _, code := execFake(t, f, "-n", "5", "12"); code != 0 {
		t.Fatal("relative adjustment failed")
	}
	if got := f.nice[schedKey{whichProcess, 12}]; got != 15 {
		t.Errorf("nice=%d, want 10+5=15", got)
	}
	if _, _, code := execFake(t, f, "-n", "100", "12"); code != 0 {
		t.Fatal("over-limit increment must clamp, not fail")
	}
	if got := f.nice[schedKey{whichProcess, 12}]; got != fakeMax {
		t.Errorf("nice=%d, want clamp to %d", got, fakeMax)
	}
	if _, _, code := execFake(t, f, "-n", "-99999999999999999999999", "12"); code != 2 {
		t.Fatal("an increment outside the implementation's integer range must fail clearly")
	}
}

func TestRequestedNiceSaturates(t *testing.T) {
	for _, c := range []struct {
		old   int
		delta int64
		want  int
	}{
		{0, 5, 5},
		{10, -4, 6},
		{0, 1 << 30, 1 << 30},
		{0, -(1 << 30), -(1 << 30)},
		{19, 9223372036854775807, priorityArgMax},
		{-20, -9223372036854775808, priorityArgMin},
	} {
		if got := requestedNice(c.old, c.delta); got != c.want {
			t.Errorf("requestedNice(%d, %d) = %d, want %d", c.old, c.delta, got, c.want)
		}
	}
}

// POSIX -u: an existing user NAME wins; only otherwise is an unsigned
// decimal operand a numeric user ID; anything else is a per-operand error.
func TestUserOperandResolution(t *testing.T) {
	f := newFake()
	if _, errs, code := execFake(t, f, "-n", "1", "-u", "54321"); code != 0 {
		t.Fatalf("numeric user: code=%d stderr=%q", code, errs)
	}
	if got := strings.Join(f.calls, "; "); got != "members user 54321; get proc 54321; set proc 54321 1" {
		t.Errorf("numeric user calls %q", got)
	}
	_, errs, code := execFake(t, newFake(), "-n", "1", "-u", "no-such-user-renice-test")
	if code != 1 || !strings.Contains(errs, "no such user") {
		t.Errorf("unknown user: code=%d stderr=%q, want 'no such user' failure", code, errs)
	}
}

func TestUserNameOperandResolvesToUID(t *testing.T) {
	cur, err := user.Current()
	if err != nil || cur.Username == "" {
		t.Skip("current user unavailable")
	}
	uid, err := parseUIDForTest(cur.Uid)
	if err != nil {
		t.Skipf("non-numeric uid %q on this platform", cur.Uid)
	}
	f := newFake()
	if _, errs, code := execFake(t, f, "-n", "1", "-u", cur.Username); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
	want := fmt.Sprintf("members user %d; get proc %d; set proc %d 1", uid, uid, uid)
	if got := strings.Join(f.calls, "; "); got != want {
		t.Errorf("calls %q, want %q (name resolved before numeric reading)", got, want)
	}
}

func parseUIDForTest(s string) (int, error) {
	var uid int
	_, err := fmt.Sscanf(s, "%d", &uid)
	return uid, err
}

// setpriority() defines ID 0 as the calling process/group; the operand is
// passed through, not rejected, exactly like the reference implementations.
func TestZeroProcessIDFailsClosedInEmbeddingAndPassesInDedicatedProcess(t *testing.T) {
	f := newFake()
	if _, errs, code := execFake(t, f, "-n", "0", "-p", "0"); code != 1 || !strings.Contains(errs, "in-process embedding") {
		t.Fatalf("embedded ID 0: code=%d stderr=%q", code, errs)
	}
	if len(f.calls) != 0 {
		t.Fatalf("embedded ID 0 touched scheduler: %v", f.calls)
	}
	if _, _, code := execFakeDedicated(t, f, "-n", "0", "-p", "0"); code != 0 {
		t.Fatal("a dedicated command process may use the kernel's ID-0 semantics")
	}
	if got := strings.Join(f.calls, "; "); got != "get proc 0; set proc 0 0" {
		t.Errorf("calls %q", got)
	}
}

func TestCollectiveIncrementIsPerProcessAndContinuesWithinSelector(t *testing.T) {
	f := newFake()
	f.membersBy[schedKey{whichPGroup, 7}] = []int{70, 71, 72}
	f.nice[schedKey{whichProcess, 70}] = 0
	f.nice[schedKey{whichProcess, 71}] = 10
	f.nice[schedKey{whichProcess, 72}] = 5
	f.setErr[schedKey{whichProcess, 71}] = errors.New("permission denied")
	_, errs, code := execFake(t, f, "-g", "7", "-n", "2")
	if code != 1 || !strings.Contains(errs, "process 71: permission denied") {
		t.Fatalf("collective partial failure: code=%d stderr=%q", code, errs)
	}
	if got := f.nice[schedKey{whichProcess, 70}]; got != 2 {
		t.Errorf("member 70 nice=%d, want 2", got)
	}
	if got := f.nice[schedKey{whichProcess, 72}]; got != 7 {
		t.Errorf("member 72 nice=%d, want 7 (continued after member failure)", got)
	}
}

func TestFullWidthNumericUIDWhereHostIntPermits(t *testing.T) {
	const operand = "4294967294"
	uid, err := uidToHostInt(operand, 4294967294)
	if strconv.IntSize < 64 {
		if err == nil {
			t.Fatal("32-bit host must fail clearly rather than wrap a UID")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	f := newFake()
	f.membersBy[schedKey{whichUser, uid}] = []int{88}
	if _, errs, code := execFake(t, f, "-u", operand, "-n", "0"); code != 0 {
		t.Fatalf("full-width UID: code=%d stderr=%q", code, errs)
	}
}

func TestHelpAndVersion(t *testing.T) {
	out, _, code := execFake(t, newFake(), "--help")
	if code != 0 || !strings.Contains(out, "renice [-g|-p|-u] -n increment ID...") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = execFake(t, newFake(), "--version")
	if code != 0 || !strings.Contains(out, "renice (qiangli/coreutils)") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
	// Unambiguous long-option prefixes expand, GNU-style.
	if out, _, code := execFake(t, newFake(), "--ver"); code != 0 || !strings.Contains(out, "renice (qiangli/coreutils)") {
		t.Errorf("--ver: code=%d out=%q", code, out)
	}
}
