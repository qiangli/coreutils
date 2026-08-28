package getconfcmd

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/bre"
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
		"BC_BASE_MAX", "BC_STRING_MAX", "INT_MAX", "LINE_MAX",
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
	wantValues := map[string]int64{
		"_POSIX_AIO_LISTIO_MAX": 2, "_POSIX_AIO_MAX": 1,
		"_POSIX_ARG_MAX": 4096, "_POSIX_CHILD_MAX": 25,
		"_POSIX_CLOCKRES_MIN": 20000000, "_POSIX_DELAYTIMER_MAX": 32,
		"_POSIX_HOST_NAME_MAX": 255, "_POSIX_LINK_MAX": 8,
		"_POSIX_LOGIN_NAME_MAX": 9, "_POSIX_MAX_CANON": 255,
		"_POSIX_MAX_INPUT": 255, "_POSIX_MQ_OPEN_MAX": 8,
		"_POSIX_MQ_PRIO_MAX": 32, "_POSIX_NAME_MAX": 14,
		"_POSIX_NGROUPS_MAX": 8, "_POSIX_OPEN_MAX": 20,
		"_POSIX_PATH_MAX": 256, "_POSIX_PIPE_BUF": 512,
		"_POSIX_RE_DUP_MAX": 255, "_POSIX_RTSIG_MAX": 8,
		"_POSIX_SEM_NSEMS_MAX": 256, "_POSIX_SEM_VALUE_MAX": 32767,
		"_POSIX_SIGQUEUE_MAX": 32, "_POSIX_SSIZE_MAX": 32767,
		"_POSIX_SS_REPL_MAX": 4, "_POSIX_STREAM_MAX": 8,
		"_POSIX_SYMLINK_MAX": 255, "_POSIX_SYMLOOP_MAX": 8,
		"_POSIX_THREAD_DESTRUCTOR_ITERATIONS": 4, "_POSIX_THREAD_KEYS_MAX": 128,
		"_POSIX_THREAD_THREADS_MAX": 64, "_POSIX_TIMER_MAX": 32,
		"_POSIX_TRACE_EVENT_NAME_MAX": 30, "_POSIX_TRACE_NAME_MAX": 8,
		"_POSIX_TRACE_SYS_MAX": 8, "_POSIX_TRACE_USER_EVENT_MAX": 32,
		"_POSIX_TTY_NAME_MAX": 9, "_POSIX_TZNAME_MAX": 6,
		"_POSIX2_BC_BASE_MAX": 99, "_POSIX2_BC_DIM_MAX": 2048,
		"_POSIX2_BC_SCALE_MAX": 99, "_POSIX2_BC_STRING_MAX": 1000,
		"_POSIX2_CHARCLASS_NAME_MAX": 14, "_POSIX2_COLL_WEIGHTS_MAX": 2,
		"_POSIX2_EXPR_NEST_MAX": 32, "_POSIX2_LINE_MAX": 2048,
		"_POSIX2_RE_DUP_MAX": 255, "_XOPEN_IOV_MAX": 16,
		"_XOPEN_NAME_MAX": 255, "_XOPEN_PATH_MAX": 1024,
	}
	if len(standardMinimumValues) != len(wantValues) {
		t.Fatalf("POSIX Maximum/Minimum Values inventory has %d names, want %d", len(standardMinimumValues), len(wantValues))
	}
	for name, value := range wantValues {
		if got, ok := standardMinimumValues[name]; !ok || got != value {
			t.Errorf("table[%s] = (%d, %t), want %d", name, got, ok, value)
		}
		want := strconv.FormatInt(value, 10)
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
	// OPEN_MAX is deliberately absent: the Go runtime raises its own descriptor
	// limit during startup, while a separately exec'd getconf observes the
	// shell's original limit. The applet must report its process limit.
	for _, name := range []string{"ARG_MAX", "CHILD_MAX", "NGROUPS_MAX", "PAGESIZE", "PAGE_SIZE", "_NPROCESSORS_CONF", "_NPROCESSORS_ONLN"} {
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
		_, known, _ := pathValue(&tool.RunContext{Dir: t.TempDir()}, name, ".")
		if !known {
			t.Errorf("path inventory %s is not routed", name)
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
		"PTHREAD_STACK_MIN": "16384", "TTY_NAME_MAX": "255",
		"TZNAME_MAX": "255", "MB_LEN_MAX": "6",
	} {
		got, _, code := runCmd(t, name)
		if code != 0 || got != want {
			t.Errorf("%s = %q (exit %d), want %q", name, got, code, want)
		}
	}
	stream, _, streamCode := runCmd(t, "STREAM_MAX")
	open, _, openCode := runCmd(t, "OPEN_MAX")
	if streamCode != 0 || openCode != 0 || stream != open {
		t.Errorf("STREAM_MAX = %q (exit %d), OPEN_MAX = %q (exit %d)", stream, streamCode, open, openCode)
	}
}

func TestWindowsFailsClosed(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	for _, name := range []string{"PATH", "RE_DUP_MAX", "_POSIX_VERSION", "_POSIX2_VERSION", "_XOPEN_VERSION", "_POSIX_V6_LP64_OFF64", "_POSIX_SAVED_IDS"} {
		got, _, code := runCmd(t, name)
		if code != 0 || got != undefined {
			t.Errorf("%s = %q (exit %d), want undefined", name, got, code)
		}
	}
}

func TestPOSIXPlatformREDupMaxMatchesProduct(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("POSIX platform adapter only")
	}
	got, errs, code := runCmd(t, "RE_DUP_MAX")
	if want := strconv.Itoa(bre.REDupMax); code != 0 || errs != "" || got != want {
		t.Fatalf("getconf RE_DUP_MAX = (%q, %q, %d), want (%q, empty, 0)", got, errs, code, want)
	}
}

func TestLinuxReportsOnlyDerivedRuntimeValues(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	// PATH is deliberately absent from this set: on Linux it reports the
	// product's standard-utility search path (TestLinuxPathReportsStandardUtilityPath),
	// not an undefined libc boundary.
	for _, name := range []string{"_POSIX_VERSION", "_POSIX2_VERSION", "_XOPEN_VERSION", "SYMLOOP_MAX"} {
		got, _, code := runCmd(t, name)
		if code != 0 || got != undefined {
			t.Errorf("%s = %q (exit %d), want undefined without a libc adapter", name, got, code)
		}
	}
	got, errs, code := runCmd(t, "RE_DUP_MAX")
	if want := strconv.Itoa(bre.REDupMax); code != 0 || errs != "" || got != want {
		t.Fatalf("getconf RE_DUP_MAX = (%q, %q, %d), want (%q, empty, 0)", got, errs, code, want)
	}

	// LINE_MAX is a POSIX utility input to more than getconf itself: the
	// conformance tests for line-oriented utilities use it to size their test
	// data. The portable Issue 7 value in sysVars must not be shadowed by the
	// Linux platform adapter and reported as "undefined".
	got, errs, code = runCmd(t, "LINE_MAX")
	if code != 0 || errs != "" || got != "2048" {
		t.Fatalf("getconf LINE_MAX = (%q, %q, %d), want (2048, empty, 0)", got, errs, code)
	}

	// BC_BASE_MAX and BC_STRING_MAX describe the calculator shipped in this
	// same multicall binary, not an unknown host-libc implementation. Keep the
	// queryable limits aligned with the bounds enforced by the bc engine.
	for name, want := range map[string]string{
		"BC_BASE_MAX":   "99",
		"BC_STRING_MAX": "1000",
	} {
		got, errs, code = runCmd(t, name)
		if code != 0 || errs != "" || got != want {
			t.Errorf("getconf %s = (%q, %q, %d), want (%s, empty, 0)", name, got, errs, code, want)
		}
	}
}

func TestMandatoryTableInventoryAndCompatibilityAliases(t *testing.T) {
	if len(pathVars) != 21 {
		t.Fatalf("fpathconf Variable inventory has %d names, want 21", len(pathVars))
	}
	if len(confstrVars) != 31 {
		t.Fatalf("confstr name inventory has %d names, want 31", len(confstrVars))
	}
	seen := map[string]bool{}
	for _, name := range systemInventoryNames {
		seen[name] = true
		if _, ok := sysVars[name]; !ok {
			t.Errorf("audited system name %s is not registered", name)
		}
	}
	if len(seen) != 165 {
		t.Fatalf("audited system superset has %d unique names, want 165", len(seen))
	}
	// Exact Variable column of sysconf(), excluding the three entries that the
	// getconf utility specification explicitly excludes. The two explanatory
	// getgr/getpw table rows are not variables.
	mandatorySysconf := strings.Fields(`
AIO_LISTIO_MAX AIO_MAX AIO_PRIO_DELTA_MAX ARG_MAX ATEXIT_MAX BC_BASE_MAX BC_DIM_MAX BC_SCALE_MAX
BC_STRING_MAX CHILD_MAX COLL_WEIGHTS_MAX DELAYTIMER_MAX EXPR_NEST_MAX HOST_NAME_MAX IOV_MAX LINE_MAX
LOGIN_NAME_MAX NGROUPS_MAX MQ_OPEN_MAX MQ_PRIO_MAX OPEN_MAX PAGE_SIZE PAGESIZE PTHREAD_DESTRUCTOR_ITERATIONS
PTHREAD_KEYS_MAX PTHREAD_STACK_MIN PTHREAD_THREADS_MAX RE_DUP_MAX RTSIG_MAX SEM_NSEMS_MAX SEM_VALUE_MAX SIGQUEUE_MAX
STREAM_MAX SYMLOOP_MAX TIMER_MAX TTY_NAME_MAX TZNAME_MAX _POSIX_ADVISORY_INFO _POSIX_BARRIERS _POSIX_ASYNCHRONOUS_IO
_POSIX_CLOCK_SELECTION _POSIX_CPUTIME _POSIX_FSYNC _POSIX_IPV6 _POSIX_JOB_CONTROL _POSIX_MAPPED_FILES _POSIX_MEMLOCK _POSIX_MEMLOCK_RANGE
_POSIX_MEMORY_PROTECTION _POSIX_MESSAGE_PASSING _POSIX_MONOTONIC_CLOCK _POSIX_PRIORITIZED_IO _POSIX_PRIORITY_SCHEDULING _POSIX_RAW_SOCKETS _POSIX_READER_WRITER_LOCKS _POSIX_REALTIME_SIGNALS
_POSIX_REGEXP _POSIX_SAVED_IDS _POSIX_SEMAPHORES _POSIX_SHARED_MEMORY_OBJECTS _POSIX_SHELL _POSIX_SPAWN _POSIX_SPIN_LOCKS _POSIX_SPORADIC_SERVER
_POSIX_SS_REPL_MAX _POSIX_SYNCHRONIZED_IO _POSIX_THREAD_ATTR_STACKADDR _POSIX_THREAD_ATTR_STACKSIZE _POSIX_THREAD_CPUTIME _POSIX_THREAD_PRIO_INHERIT _POSIX_THREAD_PRIO_PROTECT _POSIX_THREAD_PRIORITY_SCHEDULING
_POSIX_THREAD_PROCESS_SHARED _POSIX_THREAD_ROBUST_PRIO_INHERIT _POSIX_THREAD_ROBUST_PRIO_PROTECT _POSIX_THREAD_SAFE_FUNCTIONS _POSIX_THREAD_SPORADIC_SERVER _POSIX_THREADS _POSIX_TIMEOUTS _POSIX_TIMERS
_POSIX_TRACE _POSIX_TRACE_EVENT_FILTER _POSIX_TRACE_EVENT_NAME_MAX _POSIX_TRACE_INHERIT _POSIX_TRACE_LOG _POSIX_TRACE_NAME_MAX _POSIX_TRACE_SYS_MAX _POSIX_TRACE_USER_EVENT_MAX
_POSIX_TYPED_MEMORY_OBJECTS _POSIX_VERSION _POSIX_V7_ILP32_OFF32 _POSIX_V7_ILP32_OFFBIG _POSIX_V7_LP64_OFF64 _POSIX_V7_LPBIG_OFFBIG _POSIX_V6_ILP32_OFF32 _POSIX_V6_ILP32_OFFBIG
_POSIX_V6_LP64_OFF64 _POSIX_V6_LPBIG_OFFBIG _POSIX2_C_BIND _POSIX2_C_DEV _POSIX2_CHAR_TERM _POSIX2_FORT_DEV _POSIX2_FORT_RUN _POSIX2_LOCALEDEF
_POSIX2_PBS _POSIX2_PBS_ACCOUNTING _POSIX2_PBS_CHECKPOINT _POSIX2_PBS_LOCATE _POSIX2_PBS_MESSAGE _POSIX2_PBS_TRACK _POSIX2_SW_DEV _POSIX2_UPE
_POSIX2_VERSION _XOPEN_CRYPT _XOPEN_ENH_I18N _XOPEN_REALTIME _XOPEN_REALTIME_THREADS _XOPEN_SHM _XOPEN_STREAMS _XOPEN_UNIX
_XOPEN_UUCP _XOPEN_VERSION`)
	if len(mandatorySysconf) != 122 {
		t.Fatalf("mandatory sysconf Variable inventory has %d names, want 122", len(mandatorySysconf))
	}
	for _, name := range mandatorySysconf {
		if _, ok := sysVars[name]; !ok {
			t.Errorf("mandatory sysconf name %s is not registered", name)
		}
	}
	for alias, target := range compatibilityAliases {
		got, _, code := runCmd(t, alias)
		want, _, wantCode := runCmd(t, target)
		if code != wantCode || got != want {
			t.Errorf("alias %s=%q (exit %d), %s=%q (exit %d)", alias, got, code, target, want, wantCode)
		}
	}
	for _, name := range []string{"_POSIX_THREAD_ROBUST_PRIO_INHERIT", "_POSIX_THREAD_ROBUST_PRIO_PROTECT"} {
		if _, _, code := runCmd(t, name); code != 0 {
			t.Errorf("missing Issue 7 sysconf spelling %s", name)
		}
	}
}

func TestPathErrorsWriteNoStdoutAndFail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no pathconf ABI and deliberately reports undefined")
	}
	out, errs, code := runCmd(t, "NAME_MAX", filepath.Join(t.TempDir(), "missing"))
	if code == 0 || out != "" || errs == "" {
		t.Fatalf("missing path = (%q, %q, %d), want empty stdout, diagnostic, non-zero", out, errs, code)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("sink failed") }

func TestStandardOutputFailureIsAnError(t *testing.T) {
	var errs bytes.Buffer
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{Out: failingWriter{}, Err: &errs}}
	if code := run(rc, []string{"_POSIX_PATH_MAX"}); code != 1 || !strings.Contains(errs.String(), "write error") {
		t.Fatalf("code=%d stderr=%q", code, errs.String())
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
	if out, errb, code := runCmd(t, "-v", "", "_POSIX_PATH_MAX"); code == 0 || out != "" || !strings.Contains(errb, "unsupported specification") {
		t.Fatalf("empty -v specification = (%q, %q, %d), want diagnostic and non-zero status", out, errb, code)
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
