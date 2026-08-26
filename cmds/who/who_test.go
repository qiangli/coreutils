package whocmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/cmds/internal/session"
	"github.com/qiangli/coreutils/tool"
)

type whoFaultWriter struct {
	err   error
	short bool
	calls int
}

func (w *whoFaultWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.short && len(p) != 0 {
		return len(p) - 1, nil
	}
	return 0, w.err
}

func TestWhoPropagatesStdoutFailures(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("alice pts/7 1700000000 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		out  *whoFaultWriter
	}{
		{name: "explicit error", out: &whoFaultWriter{err: errors.New("injected stdout failure")}},
		{name: "short write", out: &whoFaultWriter{short: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errb bytes.Buffer
			rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"TZ=UTC", "LC_ALL=C"}, Stdio: tool.Stdio{Out: tc.out, Err: &errb}}
			if code := run(rc, []string{"utmp"}); code == 0 {
				t.Fatalf("stdout failure returned success: stderr=%q", errb.String())
			}
			if got := strings.Count(errb.String(), "who: write error:"); got != 1 {
				t.Fatalf("write-error diagnostics=%d, want 1: %q", got, errb.String())
			}
			if tc.out.calls != 1 {
				t.Fatalf("stdout writes=%d, want failure suppression after first call", tc.out.calls)
			}
		})
	}
}

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
	state := byte('+')
	if runtime.GOOS == "windows" {
		state = '?'
	}
	want := fmt.Sprintf("bob %c %s %s\n", state, tty, time.Unix(1, 0).Local().Format("Jan _2 15:04"))
	if out.String() != want {
		t.Fatalf("-T exact output=%q, want %q", out.String(), want)
	}
	if runtime.GOOS == "windows" {
		return
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
	want = fmt.Sprintf("bob - %s %s\n", tty, time.Unix(1, 0).Local().Format("Jan _2 15:04"))
	if out.String() != want {
		t.Fatalf("-T exact output after chmod=%q, want %q", out.String(), want)
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
	if !strings.Contains(out.String(), "system boot") {
		t.Fatalf("expected system boot record in output, got %q", out.String())
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
	content := "bob pts/1 1720000000 host\nghost pts/9 1720000000 host DEAD_PROCESS id=ts/9 term=9 exit=3\n"
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
	if !strings.Contains(out.String(), "id=ts/9 term=9 exit=3") {
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
	if got := terminalState(session.Record{Type: "DEAD_PROCESS", TTY: "pts/1"}); got != ' ' {
		t.Fatalf("dead terminalState=%q, want space", got)
	}
	if got := terminalState(session.Record{Type: "BOOT_TIME"}); got != ' ' {
		t.Fatalf("boot terminalState=%q, want space", got)
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
	if got := displayName(session.Record{Type: "BOOT_TIME"}); got != "system boot" {
		t.Fatalf("boot displayName=%q, want system boot", got)
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

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("no zoneinfo for %s: %v", name, err)
	}
	return loc
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

func TestWhoMissingNamedFileFailsClosed(t *testing.T) {
	dir := t.TempDir()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	if code := run(rc, []string{"missing-utmp"}); code == 0 || !strings.Contains(errb.String(), "no such file") {
		t.Fatalf("missing explicit database: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestWhoDeadWithTIncludesExit(t *testing.T) {
	dir := t.TempDir()
	content := "ghost pts/9 1720000000 host DEAD_PROCESS id=ts/9 term=9 exit=3\n"
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"TZ=UTC"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	want := "ghost   pts/9 Jul  3 09:46 id=ts/9 term=9 exit=3\n"
	if code := run(rc, []string{"-T", "-d", "utmp"}); code != 0 || out.String() != want {
		t.Fatalf("-T -d: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestWhoTExactNoOptionalComment(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("alice pts/7 1700000000 remote.example USER_PROCESS pid=333\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"TZ=UTC", "LC_ALL=C"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	if code := run(rc, []string{"-T", "utmp"}); code != 0 || out.String() != "alice ? pts/7 Nov 14 22:13\n" {
		t.Fatalf("-T exact fields: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestWhoTLoginHasNoStateField(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("LOGIN tty2 1700000000 host LOGIN_PROCESS id=ty2 pid=222\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"TZ=UTC", "LC_ALL=C"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	if code := run(rc, []string{"-T", "-l", "utmp"}); code != 0 || out.String() != "LOGIN tty2 Nov 14 22:13\n" {
		t.Fatalf("-T -l exact fields: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	out.Reset()
	if code := run(rc, []string{"-H", "-T", "-l", "utmp"}); code != 0 || out.String() != "NAME     LINE         TIME\nLOGIN tty2 Nov 14 22:13\n" {
		t.Fatalf("-H -T -l exact fields: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if stateFieldExists(session.Record{Type: "LOGIN_PROCESS"}) || stateFieldExists(session.Record{Type: "INIT_PROCESS"}) {
		t.Fatal("LOGIN_PROCESS and INIT_PROCESS must not expose a state field")
	}
}

func TestWhoShortSuppressesUserActivityAndPID(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("alice pts/missing 1700000000 remote USER_PROCESS pid=333\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"TZ=UTC", "LC_ALL=C"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	want := "NAME     LINE         TIME             COMMENT\n" +
		"alice    pts/missing  Nov 14 22:13     (remote)\n"
	if code := run(rc, []string{"-H", "-s", "-u", "utmp"}); code != 0 || out.String() != want {
		t.Fatalf("-s -u exact suppression: code=%d out=%q want=%q err=%q", code, out.String(), want, errb.String())
	}
}

func TestWhoLCtimeProviderAndFailClosedResidual(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("alice pts/7 1709251200 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	invoke := func(env []string) (int, string, string) {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: env, Stdio: tool.Stdio{Out: &out, Err: &errb}}
		return run(rc, []string{"utmp"}), out.String(), errb.String()
	}
	if code, out, errout := invoke([]string{"TZ=UTC", "LC_TIME=de_DE.UTF-8"}); code != 0 || out != "alice    pts/7        Mär  1 00:00     (host)\n" {
		t.Fatalf("German LC_TIME: code=%d out=%q err=%q", code, out, errout)
	}
	if code, out, errout := invoke([]string{"TZ=UTC", "LC_TIME=fr_FR.UTF-8"}); code == 0 || out != "" || !strings.Contains(errout, `LC_TIME locale "fr_FR.UTF-8" is unavailable`) {
		t.Fatalf("unsupported LC_TIME residual: code=%d out=%q err=%q", code, out, errout)
	}
}

func TestWhoHonorsPOSIXDSTRules(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob pts/1 1720000000 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"TZ=XXX3YYY,M1.1.0,M12.1.0"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	if code := run(rc, []string{"utmp"}); code != 0 || out.String() != "bob      pts/1        Jul  3 07:46     (host)\n" {
		t.Fatalf("POSIX DST rule: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestRunLevelDecode(t *testing.T) {
	current, previous := runLevel(int('S') + int('5')*256)
	if current != 'S' || previous != '5' {
		t.Fatalf("runLevel=(%q,%q), want S,5", current, previous)
	}
}

func TestWhoAllIsExactAndTruthful(t *testing.T) {
	dir := t.TempDir()
	content := strings.Join([]string{
		"reboot ~ 1700000000 ~ BOOT_TIME",
		"runlevel ~ 1700000001 ~ RUN_LVL pid=13651",
		"old | 1699996400 ~ OLD_TIME",
		"new { 1700000003 ~ NEW_TIME",
		"alice pts/missing 1700000004 host USER_PROCESS pid=333",
		"ghost pts/9 1700000005 host DEAD_PROCESS id=p/9 term=9 exit=3",
		"acct ~ 1700000006 ~ ACCOUNTING",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"TZ=UTC"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	if code := run(rc, []string{"-a", "utmp"}); code != 0 {
		t.Fatalf("-a: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	got := out.String()
	for _, want := range []string{"system boot", "run-level S", "last=5", "clock change", "alice", "id=p/9 term=9 exit=3"} {
		if !strings.Contains(got, want) {
			t.Fatalf("-a missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "NAME") || strings.Contains(got, "acct") || strings.Contains(got, "Nov 14 21:13") {
		t.Fatalf("-a included heading, ACCOUNTING, or OLD_TIME: %q", got)
	}
	if !strings.Contains(got, "alice    ?") || !strings.Contains(got, " ?       333") {
		t.Fatalf("-a must report unknown terminal state/idle truthfully: %q", got)
	}
}

func TestWhoUnknownDeadExitFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("ghost pts/9 1700000000 host DEAD_PROCESS\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"TZ=UTC"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	if code := run(rc, []string{"-d", "utmp"}); code == 0 || out.Len() != 0 || !strings.Contains(errb.String(), "exit status is unavailable") {
		t.Fatalf("unknown exit must fail before output: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}
