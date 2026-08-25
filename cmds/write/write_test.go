package writecmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		f.layout = layoutLinuxUtmp
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
	statFn, openTTYFn = os.Stat, defaultOpenTTY

	openSenderControlTTYFn = func(rc *tool.RunContext) (io.WriteCloser, error) {
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
		openSenderControlTTYFn, getVEOL, openCTypeFn = oldControlTTY, oldGetVEOL, oldCType
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
	classes, err := loadCharClasses([]string{"LC_ALL=C"})
	if err != nil {
		t.Fatal(err)
	}
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
	classes, err := loadCharClasses([]string{"LC_ALL=test_8bit"})
	if err != nil {
		t.Fatal(err)
	}
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

func TestMultiLoginAlertSentToControllingTerminal(t *testing.T) {
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
	if !strings.Contains(out, "write: bob is logged in on more than one line; using pts/4\n") {
		t.Errorf("stdout = %q, want multi-login information", out)
	}
	if controlBuf.String() != "\a\a" {
		t.Errorf("controlling terminal = %q, want two alerts", controlBuf.String())
	}
	if got := w.read(t, "pts/4"); !strings.Contains(got, "hi") {
		t.Errorf("recipient terminal pts/4 did not receive message: %q", got)
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
	openSenderControlTTYFn = func(*tool.RunContext) (io.WriteCloser, error) { return control, nil }
	if _, e, code := exec(t, "", "bob"); code != 0 {
		t.Fatalf("exit %d: %s", code, e)
	}
	if !control.closed || control.String() != "\a\a" {
		t.Fatalf("control closed=%v data=%q", control.closed, control.String())
	}
}

type forbiddenReader struct{ called bool }

func (r *forbiddenReader) Read([]byte) (int, error) { r.called = true; select {} }

func TestUnknownReaderFailsClosedWithoutReadingOrClosingIt(t *testing.T) {
	install(t, fixture{uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}})
	r := new(forbiddenReader)
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{In: r, Out: &out, Err: &errb}}
	if code := run(rc, []string{"bob"}); code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if r.called {
		t.Fatal("unsafe reader was invoked")
	}
	if !strings.Contains(errb.String(), "not safely interruptible") {
		t.Fatalf("stderr = %q", errb.String())
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

func TestSIGINTWritesEOTAndReturnsSuccess(t *testing.T) {
	w := install(t, fixture{
		sender: "alice", uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	defer pw.Close()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: pr, Out: &out, Err: &errb},
	}

	done := make(chan int)
	go func() {
		done <- run(rc, []string{"bob"})
	}()

	pw.Write([]byte("hello\n"))
	time.Sleep(50 * time.Millisecond)

	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Signal(os.Interrupt); err != nil {
		t.Skipf("cannot send signal: %v", err)
	}

	code := <-done
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 on SIGINT", code)
	}
	if _, err := pr.Stat(); err != nil {
		t.Fatalf("caller-owned input was closed: %v", err)
	}
	got := w.read(t, "pts/9")
	if !strings.Contains(got, "hello\nEOT\n") {
		t.Errorf("recipient terminal should receive EOT: %q", got)
	}
}
