package writecmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	whodb "github.com/qiangli/coreutils/pkg/who"
	"github.com/qiangli/coreutils/tool"
)

// ---------------------------------------------------------------------------
// Harness
//
// Nothing here touches /var/run, os/user, or a real terminal: a hermetic run
// has no logged-in user and no tty, so every OS fact is injected through the
// package seams and every "terminal" is a plain file in t.TempDir().
// ---------------------------------------------------------------------------

// fixture describes the synthetic world one test case runs in.
type fixture struct {
	layout   utmpLayout
	logins   []login  // USER_PROCESS records
	dead     []login  // DEAD_PROCESS records, written into the same database
	stale    []string // ut_line values with no device file behind them
	sender   string
	uid      int
	myTTY    string // the sender's own terminal, bare name
	unknown  bool   // user.Lookup should fail
	noPlat   bool   // pretend the platform has no messaging
	controlW io.Writer
	veol     byte
}

type login struct {
	user string
	line string
	mode os.FileMode // permission bits of the device file
	when time.Time
}

type world struct {
	dir  string // stands in for /dev
	devs map[string]string
}

var epoch = time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)

// install rewires the package seams for one test and restores them after.
func install(t *testing.T, f fixture) *world {
	t.Helper()
	if f.layout.Size == 0 {
		f.layout = layoutLinuxUtmpCompat32
	}
	if f.sender == "" {
		f.sender = "alice"
	}

	root := t.TempDir()
	dev := filepath.Join(root, "dev")
	if err := os.MkdirAll(filepath.Join(dev, "pts"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := &world{dir: dev, devs: map[string]string{}}

	var recs []utmpRecord
	for _, l := range f.logins {
		p := filepath.Join(dev, filepath.FromSlash(l.line))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, l.mode); err != nil {
			t.Fatal(err)
		}
		// The process umask masks WriteFile's mode and would strip exactly the
		// group-write bit under test on a default 022. Set it explicitly.
		if err := os.Chmod(p, l.mode); err != nil {
			t.Fatal(err)
		}
		w.devs[l.line] = p
		recs = append(recs, utmpRecord{User: l.user, Line: l.line, Host: "somehost", PID: 100, Time: l.when})
	}
	for _, line := range f.stale {
		recs = append(recs, utmpRecord{User: f.logins[0].user, Line: line, Time: epoch})
	}

	db := filepath.Join(root, "utmp")
	blob := encodeUtmp(recs, f.layout, f.layout.UserProcess)
	for _, d := range f.dead {
		blob = append(blob, encodeUtmp(
			[]utmpRecord{{User: d.user, Line: d.line, Time: d.when}}, f.layout, 8 /* DEAD_PROCESS */)...)
	}
	if err := os.WriteFile(db, blob, 0o644); err != nil {
		t.Fatal(err)
	}

	oldPath, oldLayout, oldDev := dbPath, dbLayout, devDir
	oldSup, oldLookup, oldSender := supported, lookupUser, senderInfo
	oldTTY, oldNow := senderTTY, nowFn
	oldStat, oldOpen := statFn, openTTYFn
	oldControlTTY, oldGetVEOL, oldCType := openSenderControlTTYFn, getVEOL, openCTypeFn
	oldSessionActive, oldSessionOwns, oldTerminalDevice := sessionActiveFn, sessionOwnsTerminalFn, terminalDeviceFn
	oldWatchInterrupt := watchInterruptFn

	dbPath, dbLayout, devDir = db, f.layout, dev
	supported = !f.noPlat
	lookupUser = func(string) error {
		if f.unknown {
			return errors.New("user: unknown user")
		}
		return nil
	}
	sender, uid := f.sender, f.uid
	senderInfo = func() (string, int, error) { return sender, uid, nil }
	myTTY := f.myTTY
	senderTTY = func(*tool.RunContext) string { return myTTY }
	nowFn = func() time.Time { return epoch.Add(90 * time.Minute) }
	statFn = os.Stat
	openTTYFn = func(path string) (io.WriteCloser, error) { return os.OpenFile(path, os.O_WRONLY, 0) }
	sessionActiveFn = func(int) bool { return true }
	sessionOwnsTerminalFn = func(int, string) bool { return true }
	terminalDeviceFn = func(string) bool { return true }

	openSenderControlTTYFn = func(rc *tool.RunContext, _ string) (io.WriteCloser, error) {
		if f.controlW != nil {
			return nopWriteCloser{f.controlW}, nil
		}
		return nopWriteCloser{io.Discard}, nil
	}

	if f.veol != 0 {
		getVEOL = func(io.Reader) byte { return f.veol }
	}

	t.Cleanup(func() {
		dbPath, dbLayout, devDir = oldPath, oldLayout, oldDev
		supported, lookupUser, senderInfo = oldSup, oldLookup, oldSender
		senderTTY, nowFn = oldTTY, oldNow
		statFn, openTTYFn = oldStat, oldOpen
		sessionActiveFn, sessionOwnsTerminalFn, terminalDeviceFn = oldSessionActive, oldSessionOwns, oldTerminalDevice
		openSenderControlTTYFn, getVEOL, openCTypeFn = oldControlTTY, oldGetVEOL, oldCType
		watchInterruptFn = oldWatchInterrupt
	})
	return w
}

// exec runs the command and THEN reads the captured buffers. Written the other
// way round — `return out.String(), errb.String(), run(...)` — Go evaluates the
// buffers before run() has written anything and every assertion silently sees
// empty output.
func exec(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = run(rc, args)
	return out.String(), errb.String(), code
}

func (w *world) read(t *testing.T, line string) string {
	t.Helper()
	b, err := os.ReadFile(w.devs[line])
	if err != nil {
		t.Fatalf("reading fake terminal %s: %v", line, err)
	}
	return string(b)
}

const (
	writable = 0o620 // owner rw, group w — messages allowed (mesg y)
	denied   = 0o600 // group-write clear — messages denied (mesg n)
)

// ---------------------------------------------------------------------------
// Delivery
// ---------------------------------------------------------------------------

func TestDeliversBannerBodyAndEOF(t *testing.T) {
	w := install(t, fixture{
		sender: "alice", uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	out, errOut, code := exec(t, "hello\nthere\n", "bob")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if out != "" || errOut != "" {
		t.Errorf("write must say nothing on its own streams; got out=%q err=%q", out, errOut)
	}
	got := w.read(t, "pts/9")
	want := "Message from alice (pts/1) [Sat Aug 22 10:30:00 2026]...\nhello\nthere\nEOT\n"
	if got != want {
		t.Errorf("terminal received\n %q\nwant\n %q", got, want)
	}
}

func TestBannerUsesLoginIDAssociatedWithSenderTerminal(t *testing.T) {
	w := install(t, fixture{
		sender: "effective-account", uid: 1000, myTTY: "pts/1",
		logins: []login{
			{user: "alice-login", line: "pts/1", mode: writable, when: epoch},
			{user: "bob", line: "pts/9", mode: writable, when: epoch},
		},
	})
	if _, errOut, code := exec(t, "", "bob"); code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
	if got := w.read(t, "pts/9"); !strings.HasPrefix(got, "Message from alice-login (pts/1) [") {
		t.Fatalf("banner did not use tty-associated login ID: %q", got)
	}
}

// A final line with no trailing newline must still arrive: dropping it would
// silently truncate the message, which the sender cannot see.
func TestUnterminatedFinalLineIsDelivered(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	if _, e, code := exec(t, "no newline", "bob"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	if got := w.read(t, "pts/9"); !strings.Contains(got, "no newline\nEOT\n") {
		t.Errorf("unterminated final line lost: %q", got)
	}
}

func TestEmptyStdinStillSendsBannerAndEOF(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	if _, e, code := exec(t, "", "bob"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	got := w.read(t, "pts/9")
	if !strings.HasPrefix(got, "Message from") || !strings.HasSuffix(got, "EOT\n") {
		t.Errorf("empty message should still be framed: %q", got)
	}
}

func TestBannerWithNoControllingTerminal(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "", // a pipeline or an agent harness
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	if _, e, code := exec(t, "hi\n", "bob"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	if got := w.read(t, "pts/9"); !strings.Contains(got, "Message from alice (?) [") {
		t.Errorf("a sender with no tty must not be attributed to one: %q", got)
	}
}

func TestTerminalOperandAcceptsBareAndDevForm(t *testing.T) {
	for _, operand := range []string{"pts/9", "/dev/pts/9"} {
		t.Run(operand, func(t *testing.T) {
			w := install(t, fixture{
				uid: 1000, myTTY: "pts/1",
				logins: []login{
					{user: "bob", line: "pts/8", mode: writable, when: epoch.Add(time.Hour)},
					{user: "bob", line: "pts/9", mode: writable, when: epoch},
				},
			})
			// The operand names /dev/pts/9 under the real root; the fixture's
			// dev dir is elsewhere, so rewrite the absolute form to match.
			op := operand
			if strings.HasPrefix(op, "/dev/") {
				op = filepath.Join(w.dir, strings.TrimPrefix(op, "/dev/"))
			}
			if _, e, code := exec(t, "x\n", "bob", op); code != 0 {
				t.Fatalf("exit %d: %s", code, e)
			}
			if got := w.read(t, "pts/9"); !strings.Contains(got, "x\n") {
				t.Errorf("operand %q did not select pts/9: %q", operand, got)
			}
			if got := w.read(t, "pts/8"); got != "" {
				t.Errorf("operand %q leaked onto pts/8: %q", operand, got)
			}
		})
	}
}

func TestAgentShellUsesWhoRegistryAndPtyDir(t *testing.T) {
	root := t.TempDir()
	whoFile := filepath.Join(root, "who", "sessions")
	env := []string{
		"HOME=" + root,
		"SHELL=/bin/bashy",
		"BASHY_AGENT_ID=sender",
		whodb.FileEnv + "=" + whoFile,
	}
	ptyDir := whodb.PTYDirForEnv(env)
	if err := os.MkdirAll(ptyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	line := "agent-one"
	tty := filepath.Join(ptyDir, line)
	if err := os.WriteFile(tty, nil, writable); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tty, writable); err != nil {
		t.Fatal(err)
	}
	record := fmt.Sprintf("agent-one %s %d write user id=agent-one pid=%d\n", line, epoch.Unix(), os.Getpid())
	if err := os.MkdirAll(filepath.Dir(whoFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(whoFile, []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}

	oldSup, oldSender := supported, senderInfo
	oldTTY, oldNow := senderTTY, nowFn
	oldStat, oldOpen := statFn, openTTYFn
	oldSessionActive, oldTerminalDevice := sessionActiveFn, terminalDeviceFn
	t.Cleanup(func() {
		supported, senderInfo = oldSup, oldSender
		senderTTY, nowFn = oldTTY, oldNow
		statFn, openTTYFn = oldStat, oldOpen
		sessionActiveFn, terminalDeviceFn = oldSessionActive, oldTerminalDevice
	})
	supported = false // native platform support must not matter for agent records.
	senderInfo = func() (string, int, error) { return "sender", 1000, nil }
	senderTTY = func(*tool.RunContext) string { return "" }
	nowFn = func() time.Time { return epoch.Add(time.Hour) }
	statFn = os.Stat
	openTTYFn = func(path string) (io.WriteCloser, error) { return os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0) }
	sessionActiveFn = func(pid int) bool { return pid == os.Getpid() }
	terminalDeviceFn = func(string) bool { return true }

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:   t.TempDir(),
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader("hello\n"), Out: &out, Err: &errb},
	}
	if code := run(rc, []string{"agent-one"}); code != 0 {
		t.Fatalf("write code=%d stderr=%q", code, errb.String())
	}
	got, err := os.ReadFile(tty)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Message from sender (?)") || !strings.Contains(string(got), "hello\nEOT\n") {
		t.Fatalf("agent pty did not receive write payload: %q", got)
	}
}

func TestExplicitOwnTerminalIsAllowed(t *testing.T) {
	w := install(t, fixture{sender: "alice", uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "alice", line: "pts/1", mode: writable, when: epoch}}})
	if _, e, code := exec(t, "explicit\n", "alice", "pts/1"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	if got := w.read(t, "pts/1"); !strings.Contains(got, "explicit\n") {
		t.Fatalf("terminal = %q", got)
	}
}

// ---------------------------------------------------------------------------
// Deterministic terminal selection
// ---------------------------------------------------------------------------

func TestSelectionPrefersTheMostRecentLogin(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{
			{user: "bob", line: "pts/2", mode: writable, when: epoch},
			{user: "bob", line: "pts/7", mode: writable, when: epoch.Add(2 * time.Hour)},
			{user: "bob", line: "pts/4", mode: writable, when: epoch.Add(time.Hour)},
		},
	})
	if _, e, code := exec(t, "hi\n", "bob"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	if got := w.read(t, "pts/7"); !strings.Contains(got, "hi") {
		t.Errorf("most recent login (pts/7) should receive: %q", got)
	}
	for _, other := range []string{"pts/2", "pts/4"} {
		if got := w.read(t, other); got != "" {
			t.Errorf("%s should not receive: %q", other, got)
		}
	}
}

// Equal login times are common (a terminal multiplexer opens several sessions
// in the same second), so the tie-break has to be total on its own.
func TestSelectionTieBreaksOnTerminalName(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{
			{user: "bob", line: "pts/5", mode: writable, when: epoch},
			{user: "bob", line: "pts/3", mode: writable, when: epoch},
			{user: "bob", line: "pts/4", mode: writable, when: epoch},
		},
	})
	if _, e, code := exec(t, "hi\n", "bob"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	if got := w.read(t, "pts/3"); !strings.Contains(got, "hi") {
		t.Errorf("lowest name (pts/3) should win the tie: %q", got)
	}
}

// A user with mesg n on the newest terminal is still reachable on an older
// one. Failing on the first candidate examined would make reachability depend
// on database order, which is not something the recipient can reason about.
func TestSelectionSkipsDeniedTerminalsAndFindsAPermittedOne(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{
			{user: "bob", line: "pts/2", mode: writable, when: epoch},
			{user: "bob", line: "pts/7", mode: denied, when: epoch.Add(2 * time.Hour)},
		},
	})
	if _, e, code := exec(t, "hi\n", "bob"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	if got := w.read(t, "pts/2"); !strings.Contains(got, "hi") {
		t.Errorf("permitted older terminal should receive: %q", got)
	}
	if got := w.read(t, "pts/7"); got != "" {
		t.Errorf("denied terminal must receive nothing: %q", got)
	}
}

// A database entry whose device is gone is not a terminal; it must not be
// chosen, and it must not mask a live one.
func TestSelectionIgnoresEntriesWithNoDevice(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/2", mode: writable, when: epoch}},
		stale:  []string{"pts/99"},
	})
	if _, e, code := exec(t, "hi\n", "bob"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	if got := w.read(t, "pts/2"); !strings.Contains(got, "hi") {
		t.Errorf("live terminal should receive despite a stale sibling: %q", got)
	}
}

// DEAD_PROCESS records keep the user name of the session that ended. Treating
// one as a login would address a terminal nobody is sitting at.
func TestDeadRecordsAreNotLogins(t *testing.T) {
	install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "carol", line: "pts/2", mode: writable, when: epoch}},
		dead:   []login{{user: "bob", line: "pts/3", when: epoch}},
	})
	_, errOut, code := exec(t, "hi\n", "bob")
	if code == 0 {
		t.Fatalf("a DEAD_PROCESS record must not count as logged in")
	}
	if !strings.Contains(errOut, "bob is not logged in") {
		t.Errorf("stderr = %q, want \"bob is not logged in\"", errOut)
	}
}

// ---------------------------------------------------------------------------
// Diagnostics — every one of these must be a non-zero exit AND a message
// ---------------------------------------------------------------------------

func TestFailureModes(t *testing.T) {
	cases := []struct {
		name  string
		fix   fixture
		args  []string
		want  string
		exit  int
		stdin string
	}{
		{
			name: "permission denied on the only terminal",
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{{user: "bob", line: "pts/9", mode: denied, when: epoch}}},
			args: []string{"bob"},
			want: "bob has messages disabled on pts/9",
			exit: 1,
		},
		{
			name: "permission denied on the named terminal",
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{
					{user: "bob", line: "pts/8", mode: writable, when: epoch},
					{user: "bob", line: "pts/9", mode: denied, when: epoch},
				}},
			args: []string{"bob", "pts/9"},
			want: "bob has messages disabled on pts/9",
			exit: 1,
		},
		{
			name: "user is not logged in",
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{{user: "carol", line: "pts/2", mode: writable, when: epoch}}},
			args: []string{"bob"},
			want: "bob is not logged in",
			exit: 1,
		},
		{
			name: "user is not logged in on the named terminal",
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{{user: "bob", line: "pts/2", mode: writable, when: epoch}}},
			args: []string{"bob", "pts/9"},
			want: "bob is not logged in on pts/9",
			exit: 1,
		},
		{
			name: "unknown user",
			fix: fixture{uid: 1000, myTTY: "pts/1", unknown: true,
				logins: []login{{user: "bob", line: "pts/2", mode: writable, when: epoch}}},
			args: []string{"nosuchuser"},
			want: "nosuchuser: no such user",
			exit: 1,
		},
		{
			name: "writing to your own terminal",
			fix: fixture{sender: "alice", uid: 1000, myTTY: "pts/4",
				logins: []login{{user: "alice", line: "pts/4", mode: writable, when: epoch}}},
			args: []string{"alice"},
			want: "you cannot write to your own terminal",
			exit: 1,
		},
		{
			name: "every candidate device is gone",
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{{user: "bob", line: "pts/2", mode: writable, when: epoch}}},
			args: []string{"bob", "pts/2"},
			want: "bob: no accessible terminal",
			exit: 1,
		},
		{
			name: "unsupported platform",
			fix: fixture{uid: 1000, myTTY: "pts/1", noPlat: true,
				logins: []login{{user: "bob", line: "pts/2", mode: writable, when: epoch}}},
			args: []string{"bob"},
			want: "not a Windows concept",
			exit: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := install(t, tc.fix)
			if tc.name == "every candidate device is gone" {
				if err := os.Remove(w.devs["pts/2"]); err != nil {
					t.Fatal(err)
				}
			}
			if tc.name == "unsupported platform" {
				// The refusal text is platform-specific; assert only that a
				// refusal happened and that it names the platform reason.
				tc.want = ""
			}
			out, errOut, code := exec(t, tc.stdin, tc.args...)
			if code != tc.exit {
				t.Errorf("exit = %d, want %d (stderr %q)", code, tc.exit, errOut)
			}
			if out != "" {
				t.Errorf("a failure must not write to stdout, got %q", out)
			}
			if errOut == "" {
				t.Fatalf("a failure must be diagnosed on stderr, got nothing")
			}
			if !strings.HasPrefix(errOut, "write: ") {
				t.Errorf("diagnostic must be prefixed with the tool name: %q", errOut)
			}
			if tc.want != "" && !strings.Contains(errOut, tc.want) {
				t.Errorf("stderr = %q, want it to contain %q", errOut, tc.want)
			}
		})
	}
}

// The recipient must not receive a partial message when delivery is refused.
func TestDeniedTerminalReceivesNothing(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: denied, when: epoch}},
	})
	if _, _, code := exec(t, "secret\n", "bob"); code == 0 {
		t.Fatal("expected refusal")
	}
	if got := w.read(t, "pts/9"); got != "" {
		t.Errorf("a refused write must deliver nothing, got %q", got)
	}
}

// root is exempt from the recipient's message bit, exactly as it is exempt
// from the file mode the bit lives in.
func TestSuperuserBypassesTheMessageBit(t *testing.T) {
	w := install(t, fixture{
		sender: "root", uid: 0, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: denied, when: epoch}},
	})
	if _, e, code := exec(t, "hi\n", "bob"); code != 0 {
		t.Fatalf("root should reach a mesg-n terminal: exit %d %s", code, e)
	}
	if got := w.read(t, "pts/9"); !strings.Contains(got, "hi") {
		t.Errorf("root's message did not arrive: %q", got)
	}
}

// Writing to yourself on a DIFFERENT terminal is legitimate and useful; only
// the sender's own terminal is refused.
func TestSelfWriteToAnotherTerminalIsAllowed(t *testing.T) {
	w := install(t, fixture{
		sender: "alice", uid: 1000, myTTY: "pts/1",
		logins: []login{
			{user: "alice", line: "pts/1", mode: writable, when: epoch.Add(time.Hour)},
			{user: "alice", line: "pts/5", mode: writable, when: epoch},
		},
	})
	if _, e, code := exec(t, "note to self\n", "alice"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	if got := w.read(t, "pts/5"); !strings.Contains(got, "note to self") {
		t.Errorf("the other terminal should receive: %q", got)
	}
	if got := w.read(t, "pts/1"); got != "" {
		t.Errorf("the sender's own terminal must receive nothing: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Usage
// ---------------------------------------------------------------------------

func TestUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no operands", nil},
		{"three operands", []string{"bob", "pts/1", "extra"}},
		{"unsupported flag", []string{"-x", "bob"}},
		{"unsupported long flag", []string{"--broadcast", "bob"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			install(t, fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}})
			_, errOut, code := exec(t, "", tc.args...)
			if code != 2 {
				t.Errorf("exit = %d, want 2 (usage); stderr %q", code, errOut)
			}
			if errOut == "" {
				t.Error("a usage error must be diagnosed, not silently ignored")
			}
		})
	}
}

func TestHelpAndVersion(t *testing.T) {
	install(t, fixture{uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}})
	for _, flag := range []string{"--help", "--version"} {
		out, errOut, code := exec(t, "", flag)
		if code != 0 || out == "" {
			t.Errorf("%s: exit %d out %q err %q", flag, code, out, errOut)
		}
	}
	out, _, _ := exec(t, "", "--help")
	if !strings.Contains(out, "write user_name [terminal]") {
		t.Errorf("--help must show the POSIX synopsis: %q", out)
	}
}

func TestRegisteredUnderItsPosixName(t *testing.T) {
	if got := tool.Lookup("write"); got == nil {
		t.Fatal("write is not in the tool registry")
	} else if got.Name != "write" {
		t.Errorf("registered as %q", got.Name)
	}
}

// ---------------------------------------------------------------------------
// Control-character rendering
// ---------------------------------------------------------------------------

func TestSanitizeRendersControlCharactersSafely(t *testing.T) {
	classes := loadCharClasses([]string{"LC_ALL=C"})
	cases := []struct{ in, want string }{
		{"plain\n", "plain\n"},
		{"tab\there\n", "tab\there\n"},
		{"\x1b[2J", "^[[2J"},                            // ESC — a screen-clearing sequence
		{"bel\x07", "bel\x07"},                          // BEL is preserved per POSIX
		{"\x00nul", "^@nul"},                            //
		{"del\x7f", "del^?"},                            //
		{"héllo — ok\n", "hM-CM-)llo M-bM-^@M-^T ok\n"}, // C locale is byte-classified
		{"\xffbad", "M-^?bad"},
		{"\xe9x", "M-ix"}, // lone latin-1 byte: meta notation
	}
	for _, tc := range cases {
		if got := sanitize(tc.in, classes); got != tc.want {
			t.Errorf("sanitize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

type fakeCType struct{ print, space map[byte]bool }

func (f *fakeCType) IsCntrl(b byte) (bool, error) { return !f.print[b] && !f.space[b], nil }
func (f *fakeCType) IsPrint(b byte) (bool, error) { return f.print[b], nil }
func (f *fakeCType) IsSpace(b byte) (bool, error) { return f.space[b], nil }
func (*fakeCType) Close() error                   { return nil }

func TestSanitizeUsesResolvedLCCTYPEByteClasses(t *testing.T) {
	old := openCTypeFn
	defer func() { openCTypeFn = old }()
	openCTypeFn = func(name string) (ctypeProvider, error) {
		if name != "test_8bit" {
			t.Fatalf("locale = %q", name)
		}
		return &fakeCType{print: map[byte]bool{0xe9: true}, space: map[byte]bool{0xa0: true}}, nil
	}
	classes := loadCharClasses([]string{"LC_ALL=test_8bit"})
	if got := sanitize("\xe9\xa0\x1b", classes); got != "\xe9\xa0^[" {
		t.Fatalf("sanitize = %q", got)
	}
}

// The escape hatch must actually reach the wire, not just the helper.
func TestEscapeSequencesDoNotReachTheRecipientRaw(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	if _, e, code := exec(t, "\x1b]0;pwned\x07\n", "bob"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	got := w.read(t, "pts/9")
	if strings.Contains(got, "\x1b") {
		t.Errorf("raw ESC reached the recipient's terminal: %q", got)
	}
	if !strings.Contains(got, "^[]0;pwned\a") {
		t.Errorf("escape not rendered correctly: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Failure while writing to the device
// ---------------------------------------------------------------------------

type failingWriter struct{ after int }

func (f *failingWriter) Write(p []byte) (int, error) {
	if f.after <= 0 {
		return 0, errors.New("device write failed")
	}
	f.after--
	return len(p), nil
}
func (f *failingWriter) Close() error { return nil }

type shortWriter struct {
	writes int
	short  int
	closed bool
}

func (w *shortWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.short && len(p) > 0 {
		return len(p) - 1, nil
	}
	return len(p), nil
}
func (w *shortWriter) Close() error { w.closed = true; return nil }

type closeErrorWriter struct{ bytes.Buffer }

func (*closeErrorWriter) Close() error { return errors.New("terminal close failed") }

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

type interruptingWriteCloser struct {
	bytes.Buffer
	once func()
}

func (w *interruptingWriteCloser) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	if w.once != nil {
		fn := w.once
		w.once = nil
		fn()
	}
	return n, err
}

func (w *interruptingWriteCloser) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (*interruptingWriteCloser) Close() error { return nil }

func TestWriteErrorToTerminalIsReported(t *testing.T) {
	install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	openTTYFn = func(string) (io.WriteCloser, error) { return &failingWriter{after: 1}, nil }
	_, errOut, code := exec(t, "body\n", "bob")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "device write failed") {
		t.Errorf("stderr = %q, want the underlying write error", errOut)
	}
}

func TestOpenErrorIsReported(t *testing.T) {
	install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	openTTYFn = func(string) (io.WriteCloser, error) { return nil, os.ErrPermission }
	_, errOut, code := exec(t, "", "bob")
	if code != 1 || !strings.Contains(errOut, "permission denied") {
		t.Errorf("exit %d stderr %q", code, errOut)
	}
}

func TestShortBannerWriteIsReportedAndDoesNotAlert(t *testing.T) {
	var control bytes.Buffer
	install(t, fixture{uid: 1000, myTTY: "pts/1", controlW: &control,
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}})
	recipient := &shortWriter{short: 1}
	openTTYFn = func(string) (io.WriteCloser, error) { return recipient, nil }
	_, errOut, code := exec(t, "body\n", "bob")
	if code != 1 || !strings.Contains(errOut, io.ErrShortWrite.Error()) {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
	if control.Len() != 0 {
		t.Fatalf("sender alerted before complete banner: %q", control.String())
	}
}

func TestShortAlertWriteIsReported(t *testing.T) {
	install(t, fixture{uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}})
	alert := &shortWriter{short: 1}
	openSenderControlTTYFn = func(*tool.RunContext, string) (io.WriteCloser, error) { return alert, nil }
	_, errOut, code := exec(t, "body\n", "bob")
	if code != 1 || !strings.Contains(errOut, io.ErrShortWrite.Error()) {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
}

func TestShortBodyAndEOTWritesAreReported(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		short       int
	}{{"body", "body\n", 2}, {"EOT", "", 2}} {
		t.Run(tc.name, func(t *testing.T) {
			install(t, fixture{uid: 1000, myTTY: "",
				logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}})
			recipient := &shortWriter{short: tc.short}
			openTTYFn = func(string) (io.WriteCloser, error) { return recipient, nil }
			_, errOut, code := exec(t, tc.input, "bob")
			if code != 1 || !strings.Contains(errOut, io.ErrShortWrite.Error()) {
				t.Fatalf("exit=%d stderr=%q", code, errOut)
			}
		})
	}
}

func TestBannerCompletesBeforeSenderAlerts(t *testing.T) {
	install(t, fixture{uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}})
	var events []string
	recipient := nopWriteCloser{writerFunc(func(p []byte) (int, error) {
		if len(events) == 0 {
			events = append(events, "banner")
		}
		return len(p), nil
	})}
	openTTYFn = func(string) (io.WriteCloser, error) { return recipient, nil }
	openSenderControlTTYFn = func(*tool.RunContext, string) (io.WriteCloser, error) {
		return nopWriteCloser{writerFunc(func(p []byte) (int, error) {
			events = append(events, "alerts")
			return len(p), nil
		})}, nil
	}
	if _, errOut, code := exec(t, "", "bob"); code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
	if len(events) < 2 || events[0] != "banner" || events[1] != "alerts" {
		t.Fatalf("cross-sink order = %v", events)
	}
}

func TestInterruptAfterBannerWritesEOTWithoutAlertOrBody(t *testing.T) {
	var alerts bytes.Buffer
	install(t, fixture{uid: 1000, myTTY: "pts/1", controlW: &alerts,
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}})
	sigCh := make(chan os.Signal, 1)
	watchInterruptFn = func() (chan os.Signal, func()) { return sigCh, func() {} }
	recipient := &interruptingWriteCloser{once: func() { sigCh <- os.Interrupt }}
	openTTYFn = func(string) (io.WriteCloser, error) { return recipient, nil }
	_, errOut, code := exec(t, "body\n", "bob")
	if code != 0 || errOut != "" {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
	if !strings.Contains(recipient.String(), "Message from") || !strings.HasSuffix(recipient.String(), "EOT\n") ||
		strings.Contains(recipient.String(), "body\n") || alerts.Len() != 0 {
		t.Fatalf("recipient=%q alerts=%q pending-signal=%d", recipient.String(), alerts.String(), len(sigCh))
	}
}

func TestInterruptAfterMultiLoginNoticeWritesEOTWithoutAlertOrBody(t *testing.T) {
	var alerts bytes.Buffer
	install(t, fixture{uid: 1000, myTTY: "pts/1", controlW: &alerts,
		logins: []login{
			{user: "bob", line: "pts/2", mode: writable, when: epoch},
			{user: "bob", line: "pts/4", mode: writable, when: epoch.Add(time.Hour)},
		}})
	sigCh := make(chan os.Signal, 1)
	watchInterruptFn = func() (chan os.Signal, func()) { return sigCh, func() {} }
	var recipient bytes.Buffer
	openTTYFn = func(string) (io.WriteCloser, error) { return nopWriteCloser{&recipient}, nil }
	out := &interruptingWriteCloser{once: func() { sigCh <- os.Interrupt }}
	var errOut bytes.Buffer
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{
		In: strings.NewReader("body\n"), Out: out, Err: &errOut,
	}}
	if code := run(rc, []string{"bob"}); code != 0 || errOut.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "using pts/4") || !strings.HasSuffix(recipient.String(), "EOT\n") ||
		strings.Contains(recipient.String(), "body\n") || alerts.Len() != 0 {
		t.Fatalf("stdout=%q recipient=%q alerts=%q pending-signal=%d", out.String(), recipient.String(), alerts.String(), len(sigCh))
	}
}

func TestInterruptAfterAlertsWritesEOTWithoutBody(t *testing.T) {
	install(t, fixture{uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}})
	sigCh := make(chan os.Signal, 1)
	watchInterruptFn = func() (chan os.Signal, func()) { return sigCh, func() {} }
	alerts := &interruptingWriteCloser{once: func() { sigCh <- os.Interrupt }}
	openSenderControlTTYFn = func(*tool.RunContext, string) (io.WriteCloser, error) { return alerts, nil }
	var recipient bytes.Buffer
	openTTYFn = func(string) (io.WriteCloser, error) { return nopWriteCloser{&recipient}, nil }
	_, errOut, code := exec(t, "body\n", "bob")
	if code != 0 || errOut != "" {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
	if alerts.String() != "\a\a" || !strings.HasSuffix(recipient.String(), "EOT\n") ||
		strings.Contains(recipient.String(), "body\n") {
		t.Fatalf("recipient=%q alerts=%q pending-signal=%d", recipient.String(), alerts.String(), len(sigCh))
	}
}

func TestMultiLoginStdoutFailureIsReportedBeforeAlerts(t *testing.T) {
	var alerts bytes.Buffer
	install(t, fixture{uid: 1000, myTTY: "pts/1", controlW: &alerts,
		logins: []login{
			{user: "bob", line: "pts/2", mode: writable, when: epoch},
			{user: "bob", line: "pts/4", mode: writable, when: epoch.Add(time.Hour)},
		}})
	var errOut bytes.Buffer
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{
		In: strings.NewReader("body\n"), Out: &shortWriter{short: 1}, Err: &errOut,
	}}
	if code := run(rc, []string{"bob"}); code != 1 || !strings.Contains(errOut.String(), io.ErrShortWrite.Error()) {
		t.Fatalf("exit=%d stderr=%q", code, errOut.String())
	}
	if alerts.Len() != 0 {
		t.Fatalf("alerts emitted after failed stdout notice: %q", alerts.String())
	}
}

func TestRecipientCloseErrorIsReported(t *testing.T) {
	install(t, fixture{uid: 1000, myTTY: "",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}})
	recipient := new(closeErrorWriter)
	openTTYFn = func(string) (io.WriteCloser, error) { return recipient, nil }
	_, errOut, code := exec(t, "body\n", "bob")
	if code != 1 || !strings.Contains(errOut, "terminal close failed") {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
}

func TestSenderTerminalCloseErrorIsReported(t *testing.T) {
	install(t, fixture{uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}})
	control := new(closeErrorWriter)
	openSenderControlTTYFn = func(*tool.RunContext, string) (io.WriteCloser, error) { return control, nil }
	_, errOut, code := exec(t, "body\n", "bob")
	if code != 1 || !strings.Contains(errOut, "sender terminal") || !strings.Contains(errOut, "terminal close failed") {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
}

func TestInactiveRecipientSessionIsRejected(t *testing.T) {
	install(t, fixture{uid: 1000, myTTY: "",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}})
	sessionActiveFn = func(int) bool { return false }
	_, errOut, code := exec(t, "", "bob")
	if code != 1 || !strings.Contains(errOut, "no accessible terminal") {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
}

func TestStaleExistingTTYDoesNotTriggerMultiLoginNotice(t *testing.T) {
	w := install(t, fixture{uid: 1000, myTTY: "pts/1",
		logins: []login{
			{user: "bob", line: "pts/2", mode: writable, when: epoch},
			{user: "bob", line: "pts/4", mode: writable, when: epoch.Add(time.Hour)},
		}})
	sessionOwnsTerminalFn = func(_ int, path string) bool {
		return !strings.HasSuffix(path, "/pts/4")
	}
	out, errOut, code := exec(t, "body\n", "bob")
	if code != 0 || errOut != "" {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
	if out != "" {
		t.Fatalf("stale existing tty triggered multi-login notice: %q", out)
	}
	if got := w.read(t, "pts/2"); !strings.Contains(got, "body\n") {
		t.Fatalf("authenticated terminal did not receive body: %q", got)
	}
	if got := w.read(t, "pts/4"); got != "" {
		t.Fatalf("stale terminal received data: %q", got)
	}
}

// An unreadable accounting database is not "nobody is logged in": the two
// facts want different fixes, so they must not share a diagnostic.
func TestMissingDatabaseIsDistinctFromNotLoggedIn(t *testing.T) {
	install(t, fixture{uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}})
	dbPath = filepath.Join(t.TempDir(), "absent")
	_, errOut, code := exec(t, "", "bob")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if strings.Contains(errOut, "not logged in") {
		t.Errorf("a missing database must not be reported as a logged-out user: %q", errOut)
	}
	if !strings.Contains(errOut, "absent") {
		t.Errorf("the diagnostic should name the database: %q", errOut)
	}
}

// POSIX requires an informational message naming the terminal chosen when the
// recipient is logged in more than once. It is addressed to the SENDER'S
// CONTROLLING TERMINAL, together with the two required alerts: a redirection
// of standard output must not be able to swallow the one answer to "which of
// bob's terminals did that go to?".
func TestMultiLoginNoticeGoesToStdoutAndAlertsGoToControllingTerminal(t *testing.T) {
	var controlBuf bytes.Buffer
	w := install(t, fixture{
		sender: "alice", uid: 1000, myTTY: "pts/1",
		controlW: &controlBuf,
		logins: []login{
			{user: "bob", line: "pts/2", mode: writable, when: epoch},
			{user: "bob", line: "pts/4", mode: writable, when: epoch.Add(time.Hour)},
		},
	})
	out, errOut, code := exec(t, "hi\n", "bob")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	wantNotice := "write: bob is logged in on more than one line; using pts/4\n"
	if out != wantNotice {
		t.Errorf("standard output = %q, want %q", out, wantNotice)
	}
	if got := controlBuf.String(); got != "\a\a" {
		t.Errorf("controlling terminal = %q, want two alerts", got)
	}
	if got := w.read(t, "pts/4"); !strings.Contains(got, "hi") {
		t.Errorf("recipient terminal pts/4 did not receive message: %q", got)
	}
	if got := w.read(t, "pts/2"); got != "" {
		t.Errorf("unchosen terminal pts/2 received %q", got)
	}
}

// A single login is not "more than once": no informational message is owed,
// only the alerts.
func TestSingleLoginEmitsNoInformationalMessage(t *testing.T) {
	var controlBuf bytes.Buffer
	install(t, fixture{
		uid: 1000, myTTY: "pts/1", controlW: &controlBuf,
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	out, errOut, code := exec(t, "hi\n", "bob")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if out != "" {
		t.Errorf("standard output must stay empty, got %q", out)
	}
	if controlBuf.String() != "\a\a" {
		t.Errorf("controlling terminal = %q, want two alerts only", controlBuf.String())
	}
}

// An explicit terminal operand answers the question the informational message
// exists to answer, so POSIX attaches no message to it.
func TestExplicitTerminalOperandSuppressesTheInformationalMessage(t *testing.T) {
	var controlBuf bytes.Buffer
	install(t, fixture{
		uid: 1000, myTTY: "pts/1", controlW: &controlBuf,
		logins: []login{
			{user: "bob", line: "pts/2", mode: writable, when: epoch},
			{user: "bob", line: "pts/4", mode: writable, when: epoch.Add(time.Hour)},
		},
	})
	out, errOut, code := exec(t, "hi\n", "bob", "pts/2")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if out != "" || strings.Contains(controlBuf.String(), "more than one") {
		t.Errorf("stdout=%q control=%q, want no informational message", out, controlBuf.String())
	}
}

// The informational message still goes to stdout when the sender has no
// controlling terminal; only the two terminal alerts are omitted.
func TestNoControllingTerminalStillUsesStandardOutput(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "",
		logins: []login{
			{user: "bob", line: "pts/2", mode: writable, when: epoch},
			{user: "bob", line: "pts/4", mode: writable, when: epoch.Add(time.Hour)},
		},
	})
	out, errOut, code := exec(t, "hi\n", "bob")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if out != "write: bob is logged in on more than one line; using pts/4\n" {
		t.Errorf("stdout = %q, want the POSIX informational message", out)
	}
	if got := w.read(t, "pts/4"); !strings.Contains(got, "hi\nEOT\n") {
		t.Errorf("delivery must still happen without a controlling terminal: %q", got)
	}
}

// A failed sender alert cannot be reported as a successful connection.
func TestControllingTerminalWriteFailureIsFatal(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	openSenderControlTTYFn = func(*tool.RunContext, string) (io.WriteCloser, error) {
		return &failingWriter{}, nil
	}
	out, errOut, code := exec(t, "hi\n", "bob")
	if code != 1 {
		t.Fatalf("exit %d, want 1: %s", code, errOut)
	}
	if out != "" {
		t.Errorf("standard output must stay empty, got %q", out)
	}
	if !strings.Contains(errOut, "sender terminal") {
		t.Errorf("stderr = %q, want a diagnostic naming the sender terminal", errOut)
	}
	if got := w.read(t, "pts/9"); strings.Contains(got, "hi\n") {
		t.Errorf("body must not follow a failed connection alert: %q", got)
	}
}

// POSIX STDERR: "The standard error shall be used only for diagnostic
// messages" - and standard output carries the informational message and
// nothing else. Every diagnosed failure is checked in one place so a new one
// cannot quietly start printing to stdout.
func TestNoDiagnosticEverReachesStandardOutput(t *testing.T) {
	cases := []struct {
		name  string
		fix   fixture
		args  []string
		setup func()
	}{
		{name: "not logged in", args: []string{"carol"},
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}}},
		{name: "messages disabled", args: []string{"bob"},
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{{user: "bob", line: "pts/9", mode: denied, when: epoch}}}},
		{name: "own terminal", args: []string{"bob"},
			fix: fixture{uid: 1000, myTTY: "pts/9",
				logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}}},
		{name: "no such user", args: []string{"bob"},
			fix: fixture{uid: 1000, myTTY: "pts/1", unknown: true,
				logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}}},
		{name: "unsupported platform", args: []string{"bob"},
			fix: fixture{uid: 1000, myTTY: "pts/1", noPlat: true,
				logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}}},
		{name: "open failure", args: []string{"bob"},
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}},
			setup: func() {
				openTTYFn = func(string) (io.WriteCloser, error) { return nil, os.ErrPermission }
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			install(t, tc.fix)
			if tc.setup != nil {
				tc.setup()
			}
			out, errOut, code := exec(t, "hi\n", tc.args...)
			if code == 0 {
				t.Fatalf("exit = 0, want a failure")
			}
			if out != "" {
				t.Errorf("stdout = %q, want empty; diagnostics belong on stderr", out)
			}
			if errOut == "" {
				t.Error("a failure must be diagnosed")
			}
		})
	}
}

type trackingCloser struct {
	bytes.Buffer
	closed bool
}

func (t *trackingCloser) Close() error { t.closed = true; return nil }

func TestSenderControlTerminalIsClosed(t *testing.T) {
	install(t, fixture{uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}})
	control := new(trackingCloser)
	openSenderControlTTYFn = func(*tool.RunContext, string) (io.WriteCloser, error) { return control, nil }
	if _, e, code := exec(t, "", "bob"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	if !control.closed || control.String() != "\a\a" {
		t.Fatalf("control closed=%v data=%q", control.closed, control.String())
	}
}

// slowReader deliberately advertises Len while retaining an unconstrained Read
// implementation. Len is not a non-blocking contract and must not be used as
// evidence that an arbitrary caller-owned reader is safe to enter.
type slowReader struct {
	chunks []string
	n      int
}

func (r *slowReader) Len() int {
	total := 0
	for _, chunk := range r.chunks[r.n:] {
		total += len(chunk)
	}
	return total
}

func (r *slowReader) Read(p []byte) (int, error) {
	if r.n >= len(r.chunks) {
		return 0, io.EOF
	}
	c := r.chunks[r.n]
	r.n++
	return copy(p, c), nil
}

func TestArbitraryLenReaderIsRejectedBeforeVisibleSideEffects(t *testing.T) {
	var alerts bytes.Buffer
	w := install(t, fixture{uid: 1000, myTTY: "pts/1", controlW: &alerts,
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}})
	var out, errb bytes.Buffer
	r := &slowReader{chunks: []string{"one\n", "two\n", "tail"}}
	rc := &tool.RunContext{
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: r, Out: &out, Err: &errb},
	}
	if code := run(rc, []string{"bob"}); code != 1 || !strings.Contains(errb.String(), "cannot be interrupted safely") {
		t.Fatalf("exit = %d stderr=%q", code, errb.String())
	}
	if r.n != 0 || w.read(t, "pts/9") != "" || alerts.Len() != 0 || out.Len() != 0 {
		t.Fatalf("preflight had side effects: reads=%d recipient=%q alerts=%q stdout=%q",
			r.n, w.read(t, "pts/9"), alerts.String(), out.String())
	}
}

type opaqueBlockingReader struct{ called bool }

func (r *opaqueBlockingReader) Read([]byte) (int, error) {
	r.called = true
	select {}
}

func TestOpaqueBlockingReaderIsRejectedWithoutReadOrLeak(t *testing.T) {
	r := new(opaqueBlockingReader)
	before := goroutineCount()
	_, err := prepareInput(r)
	if err == nil || !strings.Contains(err.Error(), "cannot be interrupted safely") {
		t.Fatalf("error = %v", err)
	}
	if r.called {
		t.Fatal("opaque reader was entered and could block forever")
	}
	if after := settledGoroutineCount(before); after > before {
		t.Fatalf("goroutines before=%d after=%d", before, after)
	}
}

type deadlineBlockingReader struct {
	mu       sync.Mutex
	deadline time.Time
}

func (r *deadlineBlockingReader) SetReadDeadline(deadline time.Time) error {
	r.mu.Lock()
	r.deadline = deadline
	r.mu.Unlock()
	return nil
}

func (r *deadlineBlockingReader) Read([]byte) (int, error) {
	r.mu.Lock()
	deadline := r.deadline
	r.mu.Unlock()
	if wait := time.Until(deadline); wait > 0 {
		time.Sleep(wait)
	}
	return 0, os.ErrDeadlineExceeded
}

func TestDeadlineGenericReaderIsRejectedWithoutMutatingCaller(t *testing.T) {
	r := new(deadlineBlockingReader)
	if _, err := prepareInput(r); err == nil || !strings.Contains(err.Error(), "cannot be interrupted safely") {
		t.Fatalf("error=%v", err)
	}
	r.mu.Lock()
	deadline := r.deadline
	r.mu.Unlock()
	if !deadline.IsZero() {
		t.Fatalf("caller-owned deadline was mutated to %v", deadline)
	}
}

func TestCanonicalVEOL(t *testing.T) {
	w := install(t, fixture{
		sender: "alice", uid: 1000, myTTY: "pts/1", veol: '\x03',
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	if _, e, code := exec(t, "first line\x03second line\x03", "bob"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	got := w.read(t, "pts/9")
	if !strings.Contains(got, "first line\nsecond line\nEOT\n") {
		t.Errorf("VEOL lines were not properly framed: %q", got)
	}
}

// ---------------------------------------------------------------------------
// LC_CTYPE, BEL, and UTF-8
// ---------------------------------------------------------------------------

// execEnv is exec with an invocation environment, so LC_CTYPE resolution runs
// the same way it does for a real caller.
func execEnv(t *testing.T, env []string, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:   t.TempDir(),
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = run(rc, args)
	return out.String(), errb.String(), code
}

// POSIX: "Typing <alert> shall write the <alert> character to the recipient's
// terminal." BEL is the one control character the standard names as
// deliverable, so it must arrive as the byte 0x07 — rendering it as ^G would
// silence the alert the sender asked for.
func TestTypedBELReachesTheRecipientAsAByte(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	if _, e, code := exec(t, "wake\aup\n", "bob"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	got := w.read(t, "pts/9")
	if !strings.Contains(got, "wake\aup\n") {
		t.Errorf("recipient terminal = %q, want a raw BEL byte", got)
	}
	if strings.Contains(got, "^G") {
		t.Errorf("BEL was rewritten to caret notation: %q", got)
	}
}

// BEL survives even where the locale's classifier rejects it: the pass-through
// is a POSIX rule about the alert character, not a consequence of LC_CTYPE.
func TestBELSurvivesALocaleThatDoesNotClassifyItAsPrintable(t *testing.T) {
	old := openCTypeFn
	defer func() { openCTypeFn = old }()
	openCTypeFn = func(string) (ctypeProvider, error) {
		return &fakeCType{
			print: map[byte]bool{'a': true, 'b': true},
			space: map[byte]bool{'\n': true},
		}, nil
	}
	classes := loadCharClasses([]string{"LC_ALL=test_8bit"})
	if classes.pass['\a'] {
		t.Fatal("fixture is wrong: BEL must not be in the pass table for this test to mean anything")
	}
	if got := sanitize("a\ab\n", classes); got != "a\ab\n" {
		t.Errorf("sanitize = %q, want the BEL byte preserved", got)
	}
}

// A UTF-8 LC_CTYPE must not shred multi-byte characters into meta notation:
// POSIX sends characters from the print and space classifications through, and
// in a UTF-8 locale those classifications are properties of the CHARACTER, not
// of each byte that encodes it.
func TestUTF8LocalePreservesMultiByteCharacters(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	const body = "héllo — 日本語 🌍\n"
	if _, e, code := execEnv(t, []string{"LC_ALL=en_US.UTF-8"}, body, "bob"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	if got := w.read(t, "pts/9"); !strings.Contains(got, body) {
		t.Errorf("recipient terminal = %q, want the UTF-8 bytes unchanged", got)
	}
}

func TestUTF8LocaleStillRendersControlAndInvalidBytes(t *testing.T) {
	classes := loadCharClasses([]string{"LC_ALL=en_US.UTF-8"})
	if !classes.multibyte {
		t.Fatal("en_US.UTF-8 must select multi-byte classification")
	}
	cases := []struct{ in, want string }{
		{"h\u00e9llo\n", "h\u00e9llo\n"}, // printable multi-byte passes through
		{"\U0001f30d", "\U0001f30d"},     // outside the BMP, still printable
		{"\x1b[2J", "^[[2J"},             // ESC is still rendered
		{"bel\a", "bel\a"},               // BEL is still preserved
		{"\xffbad", "M-^?bad"},           // invalid UTF-8 byte in meta notation
		{"a\u00a0b", "a\u00a0b"},         // NBSP is a space character
		{"a\u200bb", "aM-bM-^@M-^Kb"},    // ZWSP is format, not print or space
		{"\u0007\u0301", "\a\u0301"},     // BEL then a printable combining mark
	}
	for _, tc := range cases {
		if got := sanitize(tc.in, classes); got != tc.want {
			t.Errorf("sanitize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A locale pkg/ctype cannot serve — the common case on every non-glibc host —
// must degrade to the C locale's classes, not cost the recipient the message.
func TestUnsupportedLocaleFallsBackToTheCLocaleAndStillDelivers(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	openCTypeFn = func(string) (ctypeProvider, error) {
		return nil, errors.New("ctype: unsupported locale")
	}
	out, errOut, code := execEnv(t, []string{"LC_ALL=fr_FR.ISO-8859-15"}, "plain\n", "bob")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty", out)
	}
	if got := w.read(t, "pts/9"); !strings.Contains(got, "plain\nEOT\n") {
		t.Errorf("recipient terminal = %q", got)
	}
	classes := loadCharClasses([]string{"LC_ALL=fr_FR.ISO-8859-15"})
	if classes.multibyte || !classes.pass['a'] || classes.pass[0xe9] {
		t.Error("fallback classes are not the C locale's")
	}
}

func TestUTF8LocaleNameSpellings(t *testing.T) {
	yes := []string{"en_US.UTF-8", "en_US.utf8", "C.UTF-8", "de_DE.UTF-8@euro", "zh_CN.Utf-8", "UTF-8", "utf8"}
	no := []string{"C", "POSIX", "de_DE", "de_DE.ISO-8859-1", "en_US.iso885915"}
	for _, n := range yes {
		if !isUTF8Locale(n) {
			t.Errorf("isUTF8Locale(%q) = false, want true", n)
		}
	}
	for _, n := range no {
		if isUTF8Locale(n) {
			t.Errorf("isUTF8Locale(%q) = true, want false", n)
		}
	}
}

// ---------------------------------------------------------------------------
// Canonical line delimiters
// ---------------------------------------------------------------------------

// POSIX delimits an input record by "an NL, EOF, or EOL special character".
// EOL is a per-terminal setting, so recognising NL alone would buffer a whole
// session's typing on a terminal configured with a different EOL.
func TestCanonicalEOLAndNewlineBothDelimit(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1", veol: '\x1d',
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	if _, e, code := exec(t, "eol\x1dnl\nmixed\x1d", "bob"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	if got := w.read(t, "pts/9"); !strings.Contains(got, "eol\nnl\nmixed\nEOT\n") {
		t.Errorf("recipient terminal = %q", got)
	}
}

// _POSIX_VDISABLE (a zero EOL) means the terminal has no second delimiter.
// Treating a NUL byte as one would split a line at every NUL in the input.
func TestDisabledEOLDoesNotDelimitOnNUL(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	if _, e, code := exec(t, "a\x00b\n", "bob"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	if got := w.read(t, "pts/9"); !strings.Contains(got, "a^@b\nEOT\n") {
		t.Errorf("recipient terminal = %q, want one line with the NUL in caret notation", got)
	}
}

// ---------------------------------------------------------------------------
// Interrupt
// ---------------------------------------------------------------------------

// waitForBanner blocks until the banner has reached the fake terminal, so a
// test can signal the process only once write has installed its handler.
// Sleeping a fixed interval instead would kill the test binary on a slow host,
// because an unhandled SIGINT terminates it.
func waitForBanner(t *testing.T, w *world, line string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(w.devs[line]); err == nil && strings.Contains(string(b), "Message from") {
			// The banner is written before signal.Notify; give the handler
			// registration room to complete.
			time.Sleep(20 * time.Millisecond)
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("banner never reached %s", line)
}

// POSIX ASYNCHRONOUS EVENTS: "If an interrupt signal is received, write shall
// write an appropriate message on the recipient's terminal and exit with a
// status of zero."
//
// The three things this pins beyond the EOT itself: exit 0, the caller's own
// stdin still open afterwards (write duplicates the descriptor, it does not
// adopt it), and no goroutine left parked in a Read that nothing will ever
// satisfy.
func TestSIGINTWritesEOTReturnsSuccessAndLeaksNothing(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	defer pw.Close()

	before := goroutineCount()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{In: pr, Out: &out, Err: &errb}}
	done := make(chan int, 1)
	go func() { done <- run(rc, []string{"bob"}) }()

	if _, err := pw.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	waitForBanner(t, w, "pts/9")

	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Signal(os.Interrupt); err != nil {
		t.Skipf("cannot send signal: %v", err)
	}

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 on SIGINT", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("write did not return after SIGINT: the sender is blocked")
	}

	if _, err := pr.Stat(); err != nil {
		t.Fatalf("caller-owned input was closed: %v", err)
	}
	if got := w.read(t, "pts/9"); !strings.Contains(got, "hello\nEOT\n") {
		t.Errorf("recipient terminal = %q, want the body then EOT", got)
	}
	if after := settledGoroutineCount(before); after > before {
		t.Errorf("goroutines: %d before, %d after; a reader was left blocked", before, after)
	}
}

// An interrupt part-way through a line must not glue "EOT" onto the sender's
// unfinished text: the recipient would read "partialEOT" as one word.
func TestSIGINTFramesAnUnfinishedLineBeforeEOT(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	defer pw.Close()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{In: pr, Out: &out, Err: &errb}}
	done := make(chan int, 1)
	go func() { done <- run(rc, []string{"bob"}) }()

	if _, err := pw.Write([]byte("partial")); err != nil {
		t.Fatal(err)
	}
	waitForBanner(t, w, "pts/9")

	p, _ := os.FindProcess(os.Getpid())
	if err := p.Signal(os.Interrupt); err != nil {
		t.Skipf("cannot send signal: %v", err)
	}

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("write did not return after SIGINT")
	}

	got := w.read(t, "pts/9")
	if !strings.Contains(got, "partial\nEOT\n") {
		t.Errorf("recipient terminal = %q, want the partial line framed before EOT", got)
	}
}

func goroutineCount() int { return runtime.NumGoroutine() }

// settledGoroutineCount gives the runtime a bounded chance to retire the
// goroutines a finished run is still unwinding, so the leak assertion measures
// a leak rather than scheduling latency.
func settledGoroutineCount(target int) int {
	n := runtime.NumGoroutine()
	for i := 0; i < 100 && n > target; i++ {
		time.Sleep(10 * time.Millisecond)
		runtime.Gosched()
		n = runtime.NumGoroutine()
	}
	return n
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

// A resolved classifier is read-only and a delivery owns nothing shared, so
// several deliveries can run at once. Worth pinning under -race: the classifier
// used to be built per call, and a future cache would be the obvious place to
// introduce a data race.
func TestConcurrentDeliveriesShareTheClassifierSafely(t *testing.T) {
	classes := loadCharClasses([]string{"LC_ALL=en_US.UTF-8"})
	const n = 8
	var wg sync.WaitGroup
	bufs := make([]bytes.Buffer, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf("line %d héllo\a\n", i)
			errs[i] = deliver(&bufs[i], strings.NewReader(body), "alice", "pts/1", "bob", classes)
		}(i)
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("delivery %d: %v", i, errs[i])
		}
		want := fmt.Sprintf("line %d héllo\a\nEOT\n", i)
		if !strings.HasSuffix(bufs[i].String(), want) {
			t.Errorf("delivery %d = %q, want suffix %q", i, bufs[i].String(), want)
		}
	}
}
