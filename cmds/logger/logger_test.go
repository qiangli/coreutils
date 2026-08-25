package loggercmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// fakeSink captures exactly what logger decided to submit, so every assertion
// below is about the RECORD rather than about anything a daemon did with it.
type fakeSink struct {
	got      []record
	sendErr  error
	closeErr error
	closed   bool
	openPrio priority
	openTag  string
}

func (f *fakeSink) Send(r record) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	f.got = append(f.got, r)
	return nil
}

func (f *fakeSink) Close() error { f.closed = true; return f.closeErr }

// install replaces the transport seam and the pid source for one test.
func install(t *testing.T, openErr error) *fakeSink {
	t.Helper()
	f := &fakeSink{}
	oldOpen, oldPid, oldUser := openSink, pid, currentUserName
	openSink = func(_ *tool.RunContext, p priority, tag string) (sink, error) {
		if openErr != nil {
			return nil, openErr
		}
		f.openPrio, f.openTag = p, tag
		return f, nil
	}
	pid = func() int { return 4242 }
	currentUserName = func() string { return "fallbackuser" }
	t.Cleanup(func() { openSink, pid, currentUserName = oldOpen, oldPid, oldUser })
	return f
}

// exec runs the command and only THEN reads the buffers. Written the other way
// round — `return out.String(), errb.String(), run(...)` — Go evaluates the two
// String calls before run, and every output assertion silently passes against
// an empty string.
func exec(t *testing.T, env []string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:   t.TempDir(),
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code := run(rc, args)
	return out.String(), errb.String(), code
}

var testEnv = []string{"LOGNAME=alice"}

func TestOperandsJoinWithSingleSpaces(t *testing.T) {
	f := install(t, nil)
	_, errOut, code := exec(t, testEnv, "", "hello", "there", "world")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if len(f.got) != 1 {
		t.Fatalf("want exactly one record, got %d: %+v", len(f.got), f.got)
	}
	if f.got[0].Message != "hello there world" {
		t.Errorf("message = %q, want %q", f.got[0].Message, "hello there world")
	}
	// An operand that is itself blank still contributes its own separator: the
	// join is over operands, not over a re-split of the whole line.
	f2 := install(t, nil)
	if _, _, code := exec(t, testEnv, "", "a", "", "b"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if f2.got[0].Message != "a  b" {
		t.Errorf("message = %q, want %q", f2.got[0].Message, "a  b")
	}
}

// A single empty-string operand is still ONE operand: len(operands) > 0 must
// hold and the (empty) message must be logged, not silently rerouted to the
// zero-operand stdin extension. An invocation with one empty argument and an
// invocation with no arguments are different under POSIX, which requires at
// least one string operand.
func TestSingleEmptyStringOperandLogsAnEmptyMessage(t *testing.T) {
	f := install(t, nil)
	_, errOut, code := exec(t, testEnv, "unread stdin\n", "")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if len(f.got) != 1 {
		t.Fatalf("want exactly one record, got %d: %+v", len(f.got), f.got)
	}
	if f.got[0].Message != "" {
		t.Errorf("message = %q, want empty", f.got[0].Message)
	}
}

// Every empty operand still contributes its own separator: N empty operands
// join into a message of exactly N-1 spaces, matching the non-empty-operand
// join rule in TestOperandsJoinWithSingleSpaces.
func TestAllEmptyStringOperandsStillJoinWithSpaces(t *testing.T) {
	f := install(t, nil)
	_, errOut, code := exec(t, testEnv, "", "", "", "")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if len(f.got) != 1 || f.got[0].Message != "  " {
		t.Errorf("got %+v, want one record with message %q", f.got, "  ")
	}
}

// A leading or trailing empty operand still costs a separator, so the join is
// visible at both ends of the message, not only in the middle.
func TestLeadingAndTrailingEmptyOperandsJoinWithSpaces(t *testing.T) {
	f := install(t, nil)
	_, errOut, code := exec(t, testEnv, "", "", "middle", "")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if len(f.got) != 1 || f.got[0].Message != " middle " {
		t.Errorf("got %+v, want one record with message %q", f.got, " middle ")
	}
}

func TestNoOperandsReadsStdinOneRecordPerLine(t *testing.T) {
	f := install(t, nil)
	_, errOut, code := exec(t, testEnv, "first line\nsecond line\nthird\n")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	want := []string{"first line", "second line", "third"}
	if len(f.got) != len(want) {
		t.Fatalf("want %d records, got %d: %+v", len(want), len(f.got), f.got)
	}
	for i, w := range want {
		if f.got[i].Message != w {
			t.Errorf("record %d = %q, want %q", i, f.got[i].Message, w)
		}
	}
	// The trailing newline terminates the last line; it does not begin an
	// empty fourth record.
	if !f.closed {
		t.Error("the sink must be closed when the command returns")
	}
}

func TestStdinWithoutTrailingNewlineStillLogsTheLastLine(t *testing.T) {
	f := install(t, nil)
	if _, _, code := exec(t, testEnv, "no newline at eof"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if len(f.got) != 1 || f.got[0].Message != "no newline at eof" {
		t.Errorf("got %+v, want one record %q", f.got, "no newline at eof")
	}
}

func TestEmptyStdinLogsNothingAndSucceeds(t *testing.T) {
	f := install(t, nil)
	_, errOut, code := exec(t, testEnv, "")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if len(f.got) != 0 {
		t.Errorf("empty input must produce no records, got %+v", f.got)
	}
}

func TestDefaultPriorityAndTag(t *testing.T) {
	f := install(t, nil)
	if _, _, code := exec(t, testEnv, "", "msg"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if f.got[0].Priority != defaultPriority {
		t.Errorf("default priority = %d, want user.notice (%d)", f.got[0].Priority, defaultPriority)
	}
	if f.got[0].Tag != "alice" {
		t.Errorf("default tag = %q, want the login name %q", f.got[0].Tag, "alice")
	}
	// The transport must be opened with the SAME priority and tag the record
	// carries: on unix the facility is fixed at dial time, so a mismatch here
	// would route the record to the wrong facility on a real daemon.
	if f.openPrio != f.got[0].Priority || f.openTag != f.got[0].Tag {
		t.Errorf("sink opened with (%d,%q) but record is (%d,%q)",
			f.openPrio, f.openTag, f.got[0].Priority, f.got[0].Tag)
	}
}

func TestDefaultTagFallbackOrder(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{"LOGNAME wins", []string{"USER=bob", "LOGNAME=alice"}, "alice"},
		{"USER when LOGNAME is unset", []string{"USER=bob"}, "bob"},
		{"empty LOGNAME falls through", []string{"LOGNAME=", "USER=bob"}, "bob"},
		{"process user last", nil, "fallbackuser"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := install(t, nil)
			if _, _, code := exec(t, tc.env, "", "m"); code != 0 {
				t.Fatalf("exit %d", code)
			}
			if f.got[0].Tag != tc.want {
				t.Errorf("tag = %q, want %q", f.got[0].Tag, tc.want)
			}
		})
	}
}

func TestExplicitTagOverridesEnvironment(t *testing.T) {
	f := install(t, nil)
	if _, _, code := exec(t, testEnv, "", "-t", "mytag", "m"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if f.got[0].Tag != "mytag" {
		t.Errorf("tag = %q, want %q", f.got[0].Tag, "mytag")
	}
}

func TestPriorityFlag(t *testing.T) {
	for _, tc := range []struct {
		arg  string
		want priority
	}{
		{"daemon.err", 3<<3 | 3},
		{"local0.debug", 16<<3 | 7},
		{"kern.emerg", 0},
		{"auth.crit", 4<<3 | 2},
		{"security.crit", 4<<3 | 2},
		{"AUTHPRIV.Warning", 10<<3 | 4},
		{"warn", 1<<3 | 4},   // bare level, facility defaults to user
		{"error", 1<<3 | 3},  // historical synonym for err
		{"panic", 1 << 3},    // historical synonym for emerg, default user facility
		{"191", maxPriority}, // already-encoded wire value
		{"0", 0},
	} {
		t.Run(tc.arg, func(t *testing.T) {
			f := install(t, nil)
			_, errOut, code := exec(t, testEnv, "", "-p", tc.arg, "m")
			if code != 0 {
				t.Fatalf("exit %d, stderr %q", code, errOut)
			}
			if f.got[0].Priority != tc.want {
				t.Errorf("-p %s = %d, want %d", tc.arg, f.got[0].Priority, tc.want)
			}
		})
	}
}

// An unknown facility or level must FAIL. A logger that fell back to
// user.notice would route an audit record somewhere nobody is watching and
// report success for it.
func TestUnknownPriorityIsAnError(t *testing.T) {
	for _, arg := range []string{
		"nosuch.err",     // unknown facility
		"daemon.nosuch",  // unknown level
		"daemon",         // facility with no level
		"daemon.err.foo", // too many separators
		"192",            // above local7.debug
		"-1",             // below kern.emerg
		"",               // empty
	} {
		t.Run(fmt.Sprintf("%q", arg), func(t *testing.T) {
			f := install(t, nil)
			_, errOut, code := exec(t, testEnv, "", "-p="+arg, "m")
			if code == 0 {
				t.Fatalf("-p %q must fail, got exit 0", arg)
			}
			if len(f.got) != 0 {
				t.Errorf("nothing may be logged after a bad -p, got %+v", f.got)
			}
			if !strings.Contains(errOut, "logger:") {
				t.Errorf("stderr must carry a logger diagnostic, got %q", errOut)
			}
		})
	}
}

// The facility-named-without-a-level case earns its own message, because
// `logger -p daemon` is the common typo and "unknown level" would misdescribe
// it.
func TestFacilityWithoutLevelSaysSo(t *testing.T) {
	install(t, nil)
	_, errOut, _ := exec(t, testEnv, "", "-p", "daemon", "m")
	if !strings.Contains(errOut, "no level") {
		t.Errorf("stderr = %q, want it to say a level is missing", errOut)
	}
}

func TestStderrCopy(t *testing.T) {
	f := install(t, nil)
	_, errOut, code := exec(t, testEnv, "", "-s", "-t", "tg", "hello")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if errOut != "tg: hello\n" {
		t.Errorf("stderr = %q, want %q", errOut, "tg: hello\n")
	}
	// -s is a COPY: the record still goes to the log.
	if len(f.got) != 1 || f.got[0].Message != "hello" {
		t.Errorf("-s must not suppress the syslog record, got %+v", f.got)
	}
}

func TestProcessIDFlagShowsOnTheStderrCopy(t *testing.T) {
	install(t, nil)
	_, errOut, code := exec(t, testEnv, "", "-i", "-s", "-t", "tg", "hello")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if errOut != "tg[4242]: hello\n" {
		t.Errorf("stderr = %q, want %q", errOut, "tg[4242]: hello\n")
	}
}

func TestStderrCopyPerStdinLine(t *testing.T) {
	install(t, nil)
	_, errOut, code := exec(t, testEnv, "one\ntwo\n", "-s", "-t", "tg")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if errOut != "tg: one\ntg: two\n" {
		t.Errorf("stderr = %q, want one copy per line", errOut)
	}
}

func TestSinkOpenFailureIsReported(t *testing.T) {
	install(t, errors.New("no daemon here"))
	_, errOut, code := exec(t, testEnv, "", "msg")
	if code == 0 {
		t.Fatalf("a transport that will not open must fail, got exit 0")
	}
	if !strings.Contains(errOut, "no daemon here") {
		t.Errorf("stderr = %q, want the transport error", errOut)
	}
}

func TestSendFailureIsReported(t *testing.T) {
	f := install(t, nil)
	f.sendErr = errors.New("write failed")
	_, errOut, code := exec(t, testEnv, "", "msg")
	if code == 0 {
		t.Fatal("a failed submission must not exit 0")
	}
	if !strings.Contains(errOut, "write failed") {
		t.Errorf("stderr = %q, want the submission error", errOut)
	}
}

// A transport whose Close() fails must not report success: the finalization
// failure means the log may not hold what logger claimed to send, so it has to
// surface in the exit status rather than be dropped by a bare defer.
func TestCloseFailureIsReported(t *testing.T) {
	f := install(t, nil)
	f.closeErr = errors.New("flush failed")
	_, errOut, code := exec(t, testEnv, "", "msg")
	if code == 0 {
		t.Fatal("a failed Close must not exit 0")
	}
	if len(f.got) != 1 {
		t.Errorf("the record was still submitted before Close, got %+v", f.got)
	}
	if !strings.Contains(errOut, "flush failed") {
		t.Errorf("stderr = %q, want the close error", errOut)
	}
}

// A Close() failure must be diagnosed even after an earlier failure. The
// earlier non-zero status is retained, and neither failure is hidden.
func TestSendFailureIsNotMaskedByClose(t *testing.T) {
	f := install(t, nil)
	f.sendErr = errors.New("write failed")
	f.closeErr = errors.New("flush failed")
	_, errOut, code := exec(t, testEnv, "", "msg")
	if code != 1 {
		t.Fatalf("code=%d, want 1", code)
	}
	if !strings.Contains(errOut, "write failed") {
		t.Errorf("stderr = %q, want the send error preserved", errOut)
	}
	if !strings.Contains(errOut, "flush failed") {
		t.Errorf("stderr = %q, want the close error diagnosed too", errOut)
	}
}

// Unsupported flags fail loudly rather than being ignored: a script that says
// -u /path/to/socket must not be told its record went to that socket.
func TestUnsupportedFlagFailsLoudly(t *testing.T) {
	f := install(t, nil)
	_, errOut, code := exec(t, testEnv, "", "-u", "/run/other.sock", "msg")
	if code == 0 {
		t.Fatal("an unimplemented flag must not be accepted")
	}
	if len(f.got) != 0 {
		t.Errorf("nothing may be logged after a rejected flag, got %+v", f.got)
	}
	if !strings.Contains(errOut, "logger") {
		t.Errorf("stderr = %q, want a diagnostic naming the command", errOut)
	}
}

func TestHelpAndVersionSucceedWithoutOpeningTheTransport(t *testing.T) {
	for _, arg := range []string{"--help", "--version"} {
		t.Run(arg, func(t *testing.T) {
			// The transport is deliberately made unopenable: --help must not
			// need a syslog daemon, which is also what makes it usable on
			// Windows, where the transport never opens at all.
			install(t, errors.New("must not be dialed"))
			out, errOut, code := exec(t, testEnv, "", arg)
			if code != 0 {
				t.Fatalf("%s exit %d, stderr %q", arg, code, errOut)
			}
			if !strings.Contains(out, "logger") {
				t.Errorf("%s output = %q, want it to name the command", arg, out)
			}
		})
	}
}

// A leading dash inside an operand must not be reparsed as a flag once the
// operands have begun; `--` is the explicit escape.
func TestDashDashEndsOptions(t *testing.T) {
	f := install(t, nil)
	if _, _, code := exec(t, testEnv, "", "--", "-t", "not-a-flag"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if f.got[0].Message != "-t not-a-flag" {
		t.Errorf("message = %q, want the operands verbatim", f.got[0].Message)
	}
}

// A clustered shorthand whose VALUE contains an 'h' must still be a tag, not a
// request for help. The framework's uutils -h/-V pre-pass rewrites any short
// cluster containing h into --help, which would turn `logger -tmyhost msg`
// into a help screen that exits 0 — the message silently never logged.
func TestShorthandValueContainingHIsNotHelp(t *testing.T) {
	f := install(t, nil)
	out, errOut, code := exec(t, testEnv, "", "-tmyhost", "the message")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if strings.Contains(out, "Usage") {
		t.Fatalf("-tmyhost printed help instead of logging: %q", out)
	}
	if len(f.got) != 1 || f.got[0].Tag != "myhost" || f.got[0].Message != "the message" {
		t.Errorf("got %+v, want tag %q message %q", f.got, "myhost", "the message")
	}
}

// -h and -V still work: tool.Parse registers them as aliases because logger
// does not use those shorthands for anything of its own.
func TestShortHelpAndVersionAliases(t *testing.T) {
	for _, arg := range []string{"-h", "-V"} {
		t.Run(arg, func(t *testing.T) {
			install(t, errors.New("must not be dialed"))
			out, errOut, code := exec(t, testEnv, "", arg)
			if code != 0 {
				t.Fatalf("%s exit %d, stderr %q", arg, code, errOut)
			}
			if !strings.Contains(out, "logger") {
				t.Errorf("%s output = %q", arg, out)
			}
		})
	}
}
