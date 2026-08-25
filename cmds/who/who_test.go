package whocmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/cmds/internal/session"
	"github.com/qiangli/coreutils/tool"
)

func TestWhoFileAndCount(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob pts/1 1 host\nalice tty1 2 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"-q", "utmp"})
	if code != 0 || !strings.Contains(out.String(), "# users=2") {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestWhoHelp(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"--help"})
	if code != 0 || !strings.Contains(out.String(), "Usage: who") {
		t.Fatalf("--help: code=%d out=%q", code, out.String())
	}
}

func TestWhoAliasHelpVersion(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"-h"})
	if code != 0 || !strings.Contains(out.String(), "Usage: who") {
		t.Fatalf("-h: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	out.Reset()
	code = run(rc, []string{"-V"})
	if code != 0 || !strings.Contains(out.String(), "qiangli/coreutils") {
		t.Fatalf("-V: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestWhoTimeFormat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob pts/1 1720000000 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"utmp"})
	if code != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	want := time.Unix(1720000000, 0).Local().Format("Jan _2 15:04")
	if !strings.Contains(out.String(), want) {
		t.Fatalf("expected time %q in output, got %q", want, out.String())
	}
}

func TestWhoWritable(t *testing.T) {
	dir := t.TempDir()
	tty := filepath.Join(dir, "faketty")
	if err := os.WriteFile(tty, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tty, 0o660); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob "+tty+" 1 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"--writable", "utmp"})
	if code != 0 {
		t.Fatalf("--writable: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	// -T short form uses the exact POSIX "%s %c %s %s\n" layout.
	if runtime.GOOS == "windows" {
		// Windows has no Unix tty group-write permission model, so the
		// writable status is unknowable ('?'), and os.Chmod cannot flip
		// it (chmod only toggles the read-only attribute).
		if !strings.Contains(out.String(), "bob ? "+tty) {
			t.Fatalf("expected writable status '?' for bob on windows, got %q", out.String())
		}
		return
	}
	if !strings.Contains(out.String(), "bob + "+tty) {
		t.Fatalf("expected writable status '+' for bob, got %q", out.String())
	}

	// Remove group write: status should flip to '-'.
	if err := os.Chmod(tty, 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	code = run(rc, []string{"--writable", "utmp"})
	if code != 0 {
		t.Fatalf("--writable after chmod: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "bob - "+tty) {
		t.Fatalf("expected writable status '-' for bob, got %q", out.String())
	}
}

func TestWhoIdle(t *testing.T) {
	dir := t.TempDir()
	tty := filepath.Join(dir, "faketty")
	if err := os.WriteFile(tty, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob "+tty+" 1 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Recent activity: idle should be '.'.
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"-u", "utmp"})
	if code != 0 {
		t.Fatalf("-u: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), " .") {
		t.Fatalf("expected idle '.' for active terminal, got %q", out.String())
	}

	// Stale activity: idle should be 'old'.
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(tty, old, old); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	code = run(rc, []string{"-u", "utmp"})
	if code != 0 {
		t.Fatalf("-u stale: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), " old") {
		t.Fatalf("expected idle 'old' for stale terminal, got %q", out.String())
	}
}

func TestWhoOperands(t *testing.T) {
	// Only the POSIX operand shapes are accepted: `who [file]` or `who am i`.
	// Arbitrary two words, a file+am+i combination, or 4+ operands are all
	// rejected as usage errors (exit 2) — never a silent guess.
	rejected := [][]string{
		{"mom", "likes"},      // arbitrary two words -> not "am i"
		{"foo", "bar"},        // arbitrary two words
		{"utmp", "am", "i"},   // nonstandard file + am + i
		{"am", "x", "y", "z"}, // 4 operands
	}
	for _, args := range rejected {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
		code := run(rc, args)
		if code != 2 {
			t.Fatalf("args %v: expected usage error (exit 2), got code=%d out=%q err=%q", args, code, out.String(), errb.String())
		}
		if !strings.Contains(errb.String(), "extra operand") {
			t.Fatalf("args %v: expected extra operand error, got %q", args, errb.String())
		}
	}

	// `who am i` and `who am I` are the accepted -m spelling. With no stdin
	// tty and an empty default database they simply produce no rows (exit 0).
	for _, args := range [][]string{{"am", "i"}, {"am", "I"}} {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
		code := run(rc, args)
		if code != 0 {
			t.Fatalf("args %v: expected code 0, got %d err=%q", args, code, errb.String())
		}
	}
}

func TestWhoBootTime(t *testing.T) {
	dir := t.TempDir()
	content := "reboot ~ 1720000000 ~ BOOT_TIME\nbob pts/1 1720000000 host\n"
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"-b", "utmp"})
	if code != 0 {
		t.Fatalf("-b: code=%d err=%q", code, errb.String())
	}
	if !strings.Contains(out.String(), "reboot") {
		t.Fatalf("expected reboot record in output, got %q", out.String())
	}
	if strings.Contains(out.String(), "bob") {
		t.Fatalf("did not expect bob in output for -b, got %q", out.String())
	}
}

func TestWhoMessageOption(t *testing.T) {
	dir := t.TempDir()
	tty := filepath.Join(dir, "faketty")
	if err := os.WriteFile(tty, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tty, 0o660); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob "+tty+" 1 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"-w", "utmp"})
	if code != 0 {
		t.Fatalf("-w: code=%d err=%q", code, errb.String())
	}
	if !strings.Contains(out.String(), "bob") {
		t.Fatalf("expected bob in output, got %q", out.String())
	}
	if runtime.GOOS == "windows" {
		if !strings.Contains(out.String(), "bob ? "+tty) {
			t.Fatalf("expected message status '?' on windows, got %q", out.String())
		}
		return
	}
	if !strings.Contains(out.String(), "bob + "+tty) {
		t.Fatalf("expected message status '+' for bob, got %q", out.String())
	}
}

// TestWhoQuietIgnoresOtherOptions asserts -q counts the logged-in users
// BEFORE any selection filter runs and ignores every other option. With
// -b also given, a naive implementation would count the boot record; -q
// must still report only the two real users.
func TestWhoQuietIgnoresOtherOptions(t *testing.T) {
	dir := t.TempDir()
	content := "reboot ~ 1720000000 ~ BOOT_TIME\nbob pts/1 1720000000 host\nalice tty1 1720000000 host\n"
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"-q", "-b", "utmp"})
	if code != 0 {
		t.Fatalf("-q -b: code=%d err=%q", code, errb.String())
	}
	if !strings.Contains(out.String(), "# users=2") {
		t.Fatalf("expected '# users=2' ignoring -b, got %q", out.String())
	}
	if strings.Contains(out.String(), "reboot") {
		t.Fatalf("-q must not count boot record, got %q", out.String())
	}
	if !strings.Contains(out.String(), "bob") || !strings.Contains(out.String(), "alice") {
		t.Fatalf("expected both user names, got %q", out.String())
	}
}

// TestWhoDeadProcess covers -d selection and the mandatory termination/exit
// values rendered for a dead process.
func TestWhoDeadProcess(t *testing.T) {
	dir := t.TempDir()
	content := "bob pts/1 1720000000 host\nghost pts/9 1720000000 host DEAD_PROCESS\n"
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"-d", "utmp"})
	if code != 0 {
		t.Fatalf("-d: code=%d err=%q", code, errb.String())
	}
	if !strings.Contains(out.String(), "ghost") {
		t.Fatalf("expected dead process in output, got %q", out.String())
	}
	if strings.Contains(out.String(), "bob") {
		t.Fatalf("-d must not list live users, got %q", out.String())
	}
	if !strings.Contains(out.String(), "term=0 exit=0") {
		t.Fatalf("expected term/exit values for dead process, got %q", out.String())
	}
}

// TestExitStatus checks the exit-field rendering directly, including a
// non-zero termination/exit and the id= prefix.
func TestExitStatus(t *testing.T) {
	got := exitStatus(session.Record{Type: "DEAD_PROCESS", ID: "s/9", Term: 9, Exit: 3})
	if got != "id=s/9 term=9 exit=3" {
		t.Fatalf("exitStatus=%q", got)
	}
	got = exitStatus(session.Record{Type: "DEAD_PROCESS", Term: 0, Exit: 0})
	if got != "term=0 exit=0" {
		t.Fatalf("exitStatus (no id)=%q", got)
	}
}

// TestTerminalState verifies the -T '%c' handling: a dead process and
// records with no live terminal report '?', while a normal record consults
// the tty writable bit.
func TestTerminalState(t *testing.T) {
	if got := terminalState(session.Record{Type: "DEAD_PROCESS", TTY: "pts/1"}); got != '?' {
		t.Fatalf("dead terminalState=%q, want '?'", got)
	}
	if got := terminalState(session.Record{Type: "BOOT_TIME"}); got != '?' {
		t.Fatalf("boot terminalState=%q, want '?'", got)
	}
	if got := terminalState(session.Record{Type: "USER_PROCESS", TTY: ""}); got != '?' {
		t.Fatalf("no-tty terminalState=%q, want '?'", got)
	}
}

// TestWhoLoginProcessName confirms a LOGIN_PROCESS with an empty user
// renders the conventional "LOGIN" name in the -T short form.
func TestWhoLoginProcessName(t *testing.T) {
	dir := t.TempDir()
	// Empty user field: use a placeholder the text parser treats as user,
	// then rely on displayName's LOGIN substitution via an explicit record.
	if got := displayName(session.Record{Type: "LOGIN_PROCESS"}); got != "LOGIN" {
		t.Fatalf("login displayName=%q, want LOGIN", got)
	}
	if got := displayName(session.Record{Type: "BOOT_TIME"}); got != "reboot" {
		t.Fatalf("boot displayName=%q, want reboot", got)
	}
	_ = dir
}

// TestWhoHonorsTZ confirms the displayed time uses the invocation's TZ
// (rc.Env), not the host process zone — for both an IANA name and a POSIX
// TZ string.
func TestWhoHonorsTZ(t *testing.T) {
	dir := t.TempDir()
	const epoch = 1720000000
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob pts/1 1720000000 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		tz   string
		want *time.Location
	}{
		{"UTC", time.UTC},
		// A synthetic std name with a fixed offset: not a zoneinfo entry, so
		// it deterministically exercises the POSIX-TZ parser on every host.
		{"XYZ8", time.FixedZone("XYZ", -8*3600)},
		{"America/New_York", mustLoad(t, "America/New_York")},
	}
	for _, tc := range cases {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"TZ=" + tc.tz}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
		code := run(rc, []string{"utmp"})
		if code != 0 {
			t.Fatalf("TZ=%s: code=%d err=%q", tc.tz, code, errb.String())
		}
		want := time.Unix(epoch, 0).In(tc.want).Format("Jan _2 15:04")
		if !strings.Contains(out.String(), want) {
			t.Fatalf("TZ=%s: expected time %q, got %q", tc.tz, want, out.String())
		}
	}
}

// TestPosixTZ pins the standard-time parsing of POSIX TZ strings that are not
// IANA zoneinfo names, independent of the host's tzdata.
func TestPosixTZ(t *testing.T) {
	cases := []struct {
		tz     string
		name   string
		offset int // seconds east of UTC
		ok     bool
	}{
		{"EST5EDT", "EST", -5 * 3600, true},
		{"PST8PDT", "PST", -8 * 3600, true},
		{"GMT0", "GMT", 0, true},
		{"<+08>-8", "+08", 8 * 3600, true},
		{"IST-5:30", "IST", 5*3600 + 30*60, true},
		{"CET-1CEST", "CET", 1 * 3600, true},
		{"", "", 0, false},
		{":/etc/localtime", "", 0, false},
	}
	for _, tc := range cases {
		loc := posixTZ(tc.tz)
		if !tc.ok {
			if loc != nil {
				t.Fatalf("posixTZ(%q)=%v, want nil", tc.tz, loc)
			}
			continue
		}
		if loc == nil {
			t.Fatalf("posixTZ(%q)=nil, want zone", tc.tz)
		}
		name, off := time.Now().In(loc).Zone()
		if name != tc.name || off != tc.offset {
			t.Fatalf("posixTZ(%q)=(%q,%d), want (%q,%d)", tc.tz, name, off, tc.name, tc.offset)
		}
	}
}

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("no zoneinfo for %s: %v", name, err)
	}
	return loc
}

// TestEffectiveLocale pins the POSIX LC precedence: LC_ALL > LC_TIME > LANG,
// with the codeset suffix stripped.
func TestEffectiveLocale(t *testing.T) {
	mk := func(env ...string) *tool.RunContext {
		return &tool.RunContext{Env: env}
	}
	cases := []struct {
		env  []string
		want string
	}{
		{[]string{"LC_ALL=C", "LC_TIME=fr_FR", "LANG=de_DE"}, "C"},
		{[]string{"LC_TIME=fr_FR.UTF-8", "LANG=de_DE"}, "fr_FR"},
		{[]string{"LANG=de_DE.UTF-8"}, "de_DE"},
		{[]string{"LC_ALL=C.UTF-8"}, "C"},
		{nil, ""},
	}
	for _, tc := range cases {
		if got := effectiveLocale(mk(tc.env...)); got != tc.want {
			t.Fatalf("effectiveLocale(%v)=%q, want %q", tc.env, got, tc.want)
		}
	}
}

// TestWhoNamedFileError preserves named-file error handling: a read error on
// an explicit operand (here, a directory) is reported and exits non-zero,
// rather than being silently treated as an empty database.
func TestWhoNamedFileError(t *testing.T) {
	dir := t.TempDir()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"."}) // Dir itself: reading a directory errors
	if code == 0 {
		t.Fatalf("expected non-zero exit for directory operand, got 0 out=%q", out.String())
	}
	if !strings.Contains(errb.String(), "who:") {
		t.Fatalf("expected 'who:' error, got %q", errb.String())
	}
}
