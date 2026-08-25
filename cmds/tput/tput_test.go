package tputcmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/cmds/internal/terminfo"
	"github.com/qiangli/coreutils/tool"
)

// writeEntry files a compiled fixture in a private database directory.
func writeEntry(t *testing.T, dir, name string, f terminfo.Fixture, hexBucket bool) {
	t.Helper()
	if err := terminfo.WriteEntry(dir, name, f, hexBucket); err != nil {
		t.Fatal(err)
	}
}

// runIn drives the command against a private terminfo directory, so no test
// depends on which ncurses the host happens to ship.
func runIn(t *testing.T, dir string, env []string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:   t.TempDir(),
		Env:   append([]string{"TERMINFO=" + dir}, env...),
		Stdio: tool.Stdio{Out: &out, Err: &errb},
	}
	code := run(rc, args)
	return out.String(), errb.String(), code
}

// fixtureDir writes the demo entry (plus any extras) into a fresh database.
func fixtureDir(t *testing.T, extra ...terminfo.Fixture) string {
	t.Helper()
	dir := t.TempDir()
	writeEntry(t, dir, "demo", terminfo.DemoFixture(), false)
	for _, f := range extra {
		writeEntry(t, dir, f.Names[0], f, false)
	}
	return dir
}

// THE HEART OF THE COMMAND. POSIX gives tput five exit statuses and scripts
// branch on them; the status IS the output for a boolean. Each case below is
// one of the five.
func TestExitStatuses(t *testing.T) {
	dir := fixtureDir(t)

	t.Run("0 — a string capability was emitted", func(t *testing.T) {
		out, _, code := runIn(t, dir, nil, "-T", "demo", "clear")
		if code != exitOK {
			t.Errorf("exit %d, want 0", code)
		}
		if out != "\x1b[H\x1b[2J" {
			t.Errorf("output %q", out)
		}
	})

	t.Run("0 — a true boolean writes nothing", func(t *testing.T) {
		out, _, code := runIn(t, dir, nil, "-T", "demo", "am")
		if code != exitOK {
			t.Errorf("exit %d, want 0", code)
		}
		if out != "" {
			t.Errorf("a boolean must not write to stdout, got %q", out)
		}
	})

	t.Run("0 — a numeric capability, including zero", func(t *testing.T) {
		out, _, code := runIn(t, dir, nil, "-T", "demo", "xmc")
		if code != exitOK || out != "0\n" {
			t.Errorf("got (%q, %d), want (\"0\\n\", 0) — a present zero is not absent", out, code)
		}
	})

	t.Run("1 — a false boolean", func(t *testing.T) {
		out, errb, code := runIn(t, dir, nil, "-T", "demo", "hc")
		if code != exitAbsent {
			t.Errorf("exit %d, want 1", code)
		}
		if out != "" || errb != "" {
			t.Errorf("a false boolean writes nothing: out=%q err=%q", out, errb)
		}
	})

	t.Run("1 — a capability this terminal does not have", func(t *testing.T) {
		for _, capName := range []string{"lm", "cuu1", "bw"} {
			out, _, code := runIn(t, dir, nil, "-T", "demo", capName)
			if code != exitAbsent {
				t.Errorf("%s: exit %d, want 1", capName, code)
			}
			if out != "" {
				t.Errorf("%s: output %q, want nothing", capName, out)
			}
		}
	})

	t.Run("2 — usage errors", func(t *testing.T) {
		for _, args := range [][]string{
			{},                             // no operand
			{"-T", "demo"},                 // still no operand
			{"-T", "demo", "--bogus-flag"}, // an unsupported flag must fail loudly
			{"-Q", "clear"},                // an unsupported short flag likewise
		} {
			_, errb, code := runIn(t, dir, nil, args...)
			if code != exitUsage {
				t.Errorf("%v: exit %d, want 2", args, code)
			}
			if errb == "" {
				t.Errorf("%v: a usage error must explain itself", args)
			}
		}
	})

	t.Run("0 — no terminal type selects the conservative default", func(t *testing.T) {
		for _, env := range [][]string{nil, {"TERM="}} {
			out, errb, code := runIn(t, dir, env, "longname")
			if code != exitOK || out != "80-column dumb tty\n" || errb != "" {
				t.Errorf("env=%v: got (out=%q, err=%q, code=%d)", env, out, errb, code)
			}
		}
	})

	t.Run("3 — unknown terminal type", func(t *testing.T) {
		_, errb, code := runIn(t, dir, nil, "-T", "no-such-terminal-type", "clear")
		if code != exitUnknownTT {
			t.Errorf("exit %d, want 3", code)
		}
		if !strings.Contains(errb, "unknown terminal") {
			t.Errorf("stderr %q", errb)
		}
	})

	t.Run("4 — the operand is not a capability name", func(t *testing.T) {
		for _, capName := range []string{"nosuchcap", "kf99", "CUP"} {
			out, errb, code := runIn(t, dir, nil, "-T", "demo", capName)
			if code != exitBadCap {
				t.Errorf("%s: exit %d, want 4", capName, code)
			}
			if out != "" {
				t.Errorf("%s: output %q", capName, out)
			}
			if !strings.Contains(errb, capName) {
				t.Errorf("%s: stderr %q should name the operand", capName, errb)
			}
		}
	})
}

// "absent from this terminal" (1) and "no such capability" (4) are DIFFERENT
// answers, and telling them apart is the whole reason the name tables exist
// independently of any one entry.
func TestAbsentIsNotUnknown(t *testing.T) {
	dir := fixtureDir(t)
	if _, _, code := runIn(t, dir, nil, "-T", "demo", "cuu1"); code != exitAbsent {
		t.Errorf("a real capability this terminal lacks: exit %d, want 1", code)
	}
	if _, _, code := runIn(t, dir, nil, "-T", "demo", "cuu99"); code != exitBadCap {
		t.Errorf("a capability that does not exist: exit %d, want 4", code)
	}
}

func TestParameterizedCapability(t *testing.T) {
	dir := fixtureDir(t)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"cup", "5", "10"}, "\x1b[6;11H"},
		{[]string{"cup", "0", "0"}, "\x1b[1;1H"},
		{[]string{"cup", "5"}, "\x1b[6;1H"},              // a missing operand is zero
		{[]string{"cup", "5", "10", "99"}, "\x1b[6;11H"}, // extra operands are ignored
		// With NO operands the raw template is emitted, which is how a script
		// asks for the uninstantiated string.
		{[]string{"cup"}, "\x1b[%i%p1%d;%p2%dH"},
	} {
		out, errb, code := runIn(t, dir, nil, append([]string{"-T", "demo"}, c.args...)...)
		if code != exitOK {
			t.Errorf("%v: exit %d (%s)", c.args, code, errb)
		}
		if out != c.want {
			t.Errorf("%v: got %q, want %q", c.args, out, c.want)
		}
	}
}

// Padding is a directive to the output driver, not text to print.
func TestPaddingIsNotEmitted(t *testing.T) {
	dir := fixtureDir(t)
	out, _, code := runIn(t, dir, nil, "-T", "demo", "el")
	if code != exitOK || out != "\x1b[K" {
		t.Errorf("got (%q, %d), want (\"\\x1b[K\", 0)", out, code)
	}
}

// $TERM supplies the type when -T does not; -T wins when both are present.
func TestTerminalTypeResolution(t *testing.T) {
	other := terminfo.DemoFixture()
	other.Names = []string{"other", "the other terminal"}
	other.Strs["clear"] = "<other>"
	dir := fixtureDir(t, other)

	out, _, code := runIn(t, dir, []string{"TERM=other"}, "clear")
	if code != exitOK || out != "<other>" {
		t.Errorf("$TERM: got (%q, %d)", out, code)
	}
	out, _, code = runIn(t, dir, []string{"TERM=other"}, "-T", "demo", "clear")
	if code != exitOK || out != "\x1b[H\x1b[2J" {
		t.Errorf("-T must override $TERM: got (%q, %d)", out, code)
	}
}

// lines and cols describe the window in front of the user, not the entry's
// 80x24 default — reporting 132 into a 200-column window is a wrong answer
// that looks right.
func TestScreenSizeEnvironmentOverride(t *testing.T) {
	dir := fixtureDir(t)
	for _, c := range []struct {
		env  []string
		cap  string
		want string
	}{
		{nil, "cols", "132\n"},
		{nil, "lines", "50\n"},
		{[]string{"COLUMNS=200"}, "cols", "200\n"},
		{[]string{"LINES=17"}, "lines", "17\n"},
		{[]string{"COLUMNS=nonsense"}, "cols", "132\n"},
		{[]string{"COLUMNS=0"}, "cols", "132\n"},
		{[]string{"COLUMNS=200"}, "lines", "50\n"},
	} {
		out, _, code := runIn(t, dir, c.env, "-T", "demo", c.cap)
		if code != exitOK || out != c.want {
			t.Errorf("env=%v %s: got (%q, %d), want %q", c.env, c.cap, out, code, c.want)
		}
	}
}

// The override applies only to the two screen-geometry capabilities.
func TestOtherNumericsIgnoreTheEnvironment(t *testing.T) {
	dir := fixtureDir(t)
	out, _, code := runIn(t, dir, []string{"COLUMNS=200", "LINES=17"}, "-T", "demo", "xmc")
	if code != exitOK || out != "0\n" {
		t.Errorf("got (%q, %d)", out, code)
	}
}

func TestInitAndReset(t *testing.T) {
	dir := fixtureDir(t)

	out, _, code := runIn(t, dir, nil, "-T", "demo", "init")
	if code != exitOK || out != "<is1><is2><is3>" {
		t.Errorf("init: got (%q, %d)", out, code)
	}

	// reset prefers the rs* set and falls back per part to the init strings,
	// so an entry defining only rs2 still emits a full sequence.
	out, _, code = runIn(t, dir, nil, "-T", "demo", "reset")
	if code != exitOK || out != "<is1><rs2><is3>" {
		t.Errorf("reset: got (%q, %d)", out, code)
	}
}

func TestPOSIXOperationSequence(t *testing.T) {
	dir := fixtureDir(t)

	out, errb, code := runIn(t, dir, nil, "-T", "demo", "clear", "init", "reset")
	if code != exitOK || out != "\x1b[H\x1b[2J<is1><is2><is3><is1><rs2><is3>" || errb != "" {
		t.Errorf("got (out=%q, err=%q, code=%d)", out, errb, code)
	}

	// Once the first operand selects POSIX operation mode, every following
	// operand must also be an operation; it is not a parameter to clear.
	out, errb, code = runIn(t, dir, nil, "-T", "demo", "clear", "5")
	if code != exitBadCap || out != "\x1b[H\x1b[2J" || !strings.Contains(errb, "5") {
		t.Errorf("non-operation: got (out=%q, err=%q, code=%d)", out, errb, code)
	}
}

func TestPOSIXOperationSequenceAvailabilityAndErrors(t *testing.T) {
	t.Run("missing operation continues", func(t *testing.T) {
		dir := t.TempDir()
		f := terminfo.DemoFixture()
		delete(f.Strs, "clear")
		writeEntry(t, dir, "demo", f, false)

		out, errb, code := runIn(t, dir, nil, "-T", "demo", "clear", "init")
		if code != exitOK || out != "<is1><is2><is3>" || errb != "" {
			t.Errorf("got (out=%q, err=%q, code=%d)", out, errb, code)
		}
	})

	t.Run("all unsupported operations are successful no-ops", func(t *testing.T) {
		var errb bytes.Buffer
		out := &countingFailingWriter{}
		rc := &tool.RunContext{
			Dir:   t.TempDir(),
			Env:   []string{"TERM=dumb"},
			Stdio: tool.Stdio{Out: out, Err: &errb},
		}
		if code := run(rc, []string{"clear", "init", "reset"}); code != exitOK {
			t.Errorf("exit %d, want %d", code, exitOK)
		}
		if out.writes != 0 || errb.String() != "" {
			t.Errorf("writes=%d, stderr=%q", out.writes, errb.String())
		}
	})

	t.Run("write error", func(t *testing.T) {
		dir := fixtureDir(t)
		var errb bytes.Buffer
		out := &countingFailingWriter{}
		rc := &tool.RunContext{
			Dir:   t.TempDir(),
			Env:   []string{"TERMINFO=" + dir},
			Stdio: tool.Stdio{Out: out, Err: &errb},
		}
		if code := run(rc, []string{"-T", "demo", "clear", "init"}); code != exitError {
			t.Errorf("exit %d, want %d", code, exitError)
		}
		if out.writes != 1 || !strings.Contains(errb.String(), "write") {
			t.Errorf("writes=%d, stderr=%q", out.writes, errb.String())
		}
	})
}

// The init/reset FILE capability names a file whose bytes are written out.
func TestInitFile(t *testing.T) {
	dir := t.TempDir()
	payload := filepath.Join(t.TempDir(), "init.seq")
	if err := os.WriteFile(payload, []byte("<file>"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := terminfo.DemoFixture()
	f.Strs["if"] = payload
	writeEntry(t, dir, "demo", f, false)

	out, _, code := runIn(t, dir, nil, "-T", "demo", "init")
	if code != exitOK || out != "<is1><is2><file><is3>" {
		t.Errorf("got (%q, %d)", out, code)
	}

	// An unreadable file is skipped: the rest of the sequence is still worth
	// sending, and failing would break a terminal for a missing extra.
	f.Strs["if"] = filepath.Join(t.TempDir(), "gone")
	dir2 := t.TempDir()
	writeEntry(t, dir2, "demo", f, false)
	out, _, code = runIn(t, dir2, nil, "-T", "demo", "init")
	if code != exitOK || out != "<is1><is2><is3>" {
		t.Errorf("missing init file: got (%q, %d)", out, code)
	}
}

// An entry with no init strings at all emits nothing and still succeeds.
func TestInitWithNoInitStrings(t *testing.T) {
	dir := t.TempDir()
	f := terminfo.Fixture{Names: []string{"bare", "a bare terminal"}, Strs: map[string]string{"bel": "\a"}}
	writeEntry(t, dir, "bare", f, false)
	out, _, code := runIn(t, dir, nil, "-T", "bare", "init")
	if code != exitOK || out != "" {
		t.Errorf("got (%q, %d)", out, code)
	}
}

func TestLongname(t *testing.T) {
	dir := fixtureDir(t)
	out, _, code := runIn(t, dir, nil, "-T", "demo", "longname")
	if code != exitOK || out != "a demo terminal\n" {
		t.Errorf("got (%q, %d)", out, code)
	}
}

// A user-defined capability is a real capability of that terminal, so it
// resolves rather than being reported as a nonexistent name.
func TestUserDefinedCapability(t *testing.T) {
	dir := t.TempDir()
	f := terminfo.DemoFixture()
	f.ExtStrs = map[string]string{"E3": "\x1b[3J"}
	f.ExtBools = map[string]bool{"AX": true}
	f.ExtNums = map[string]int{"U8": 1}
	writeEntry(t, dir, "demo", f, false)

	if out, _, code := runIn(t, dir, nil, "-T", "demo", "E3"); code != exitOK || out != "\x1b[3J" {
		t.Errorf("E3: got (%q, %d)", out, code)
	}
	if _, _, code := runIn(t, dir, nil, "-T", "demo", "AX"); code != exitOK {
		t.Errorf("AX: exit %d, want 0", code)
	}
	if out, _, code := runIn(t, dir, nil, "-T", "demo", "U8"); code != exitOK || out != "1\n" {
		t.Errorf("U8: got (%q, %d)", out, code)
	}
	// Still unknown when the entry does not define it.
	dir2 := fixtureDir(t)
	if _, _, code := runIn(t, dir2, nil, "-T", "demo", "E3"); code != exitBadCap {
		t.Errorf("E3 on an entry without it: exit %d, want 4", code)
	}
}

// A capability string the entry got wrong must be reported, not printed as
// half-instantiated garbage.
func TestMalformedCapabilityStringIsReported(t *testing.T) {
	dir := t.TempDir()
	f := terminfo.DemoFixture()
	f.Strs["cup"] = "\x1b[%p1%Zm"
	writeEntry(t, dir, "demo", f, false)

	out, errb, code := runIn(t, dir, nil, "-T", "demo", "cup", "1", "2")
	if code != exitError {
		t.Errorf("exit %d, want %d", code, exitError)
	}
	if out != "" {
		t.Errorf("output %q, want nothing", out)
	}
	if !strings.Contains(errb, "cup") {
		t.Errorf("stderr %q should name the capability", errb)
	}
}

func TestHelpAndVersion(t *testing.T) {
	dir := fixtureDir(t)
	out, _, code := runIn(t, dir, nil, "--help")
	if code != 0 {
		t.Errorf("--help exit %d", code)
	}
	for _, want := range []string{"tput", "-T", "capname", "xterm"} {
		if !strings.Contains(out, want) {
			t.Errorf("--help output does not mention %q:\n%s", want, out)
		}
	}
	if out, _, code := runIn(t, dir, nil, "--version"); code != 0 || !strings.Contains(out, "tput") {
		t.Errorf("--version: got (%q, %d)", out, code)
	}
}

// A tool must report a failed write rather than exiting 0 on output nobody
// received.
func TestWriteErrorIsReported(t *testing.T) {
	dir := fixtureDir(t)
	for _, capName := range []string{"clear", "init", "longname", "xmc"} {
		var errb bytes.Buffer
		rc := &tool.RunContext{
			Dir:   t.TempDir(),
			Env:   []string{"TERMINFO=" + dir},
			Stdio: tool.Stdio{Out: failingWriter{}, Err: &errb},
		}
		if code := run(rc, []string{"-T", "demo", capName}); code != exitError {
			t.Errorf("%s: exit %d, want %d", capName, code, exitError)
		}
		if !strings.Contains(errb.String(), "write") {
			t.Errorf("%s: stderr %q", capName, errb.String())
		}
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

type countingFailingWriter struct{ writes int }

func (w *countingFailingWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, errors.New("output unavailable")
}

func TestRegisteredUnderItsOwnName(t *testing.T) {
	got := tool.Lookup("tput")
	if got == nil {
		t.Fatal("tput is not registered")
	}
	if got.Run == nil || got.Synopsis == "" || got.Usage == "" {
		t.Errorf("incomplete registration: %+v", got)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:   t.TempDir(),
		Env:   []string{"TERMINFO=" + fixtureDir(t)},
		Stdio: tool.Stdio{Out: &out, Err: &errb},
	}
	// Dispatch through the registered Tool, not the local run, so the name,
	// parsing and diagnostics are all covered.
	if code := got.Run(rc, []string{"-T", "demo", "bel"}); code != exitOK || out.String() != "\a" {
		t.Errorf("got (%q, %d) err=%q", out.String(), code, errb.String())
	}
}
