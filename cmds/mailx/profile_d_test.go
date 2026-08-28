package mailxcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// seedSenderMailbox writes a system mailbox holding one message per sender and
// returns the environment that selects it.
func seedSenderMailbox(t *testing.T, senders ...string) (string, []string) {
	t.Helper()
	d := t.TempDir()
	spool := filepath.Join(d, "spool")
	if err := os.MkdirAll(spool, 0o755); err != nil {
		t.Fatal(err)
	}
	var mbox bytes.Buffer
	for i, sender := range senders {
		mbox.WriteString("From " + sender + " Wed Aug 26 12:30:00 2026\n")
		mbox.WriteString("Date: Wed, 26 Aug 2026 12:30:00 +0000\n")
		mbox.WriteString("From: " + sender + "\n")
		mbox.WriteString("To: alice\n")
		mbox.WriteString("Subject: subject " + string(rune('1'+i)) + "\n\n")
		mbox.WriteString("body " + string(rune('1'+i)) + "\n\n")
	}
	if err := os.WriteFile(filepath.Join(spool, "alice"), mbox.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return d, []string{"LOGNAME=alice", "HOME=" + d, "MAILX_SPOOL=" + spool, "MAILRC=" + filepath.Join(d, "none.rc"), "MAILX_SYSTEM_RC=" + filepath.Join(d, "none.rc")}
}

// POSIX admits a plain address as a msglist selector -- "any address as shown
// in a header summary shall be matchable in this form" -- and a login name may
// contain a <hyphen>. Only a fully numeric pair is the "n-m" range form.
func TestPOSIXAddressSelectorAcceptsHyphenatedLogin(t *testing.T) {
	d, env := seedSenderMailbox(t, "mary-ann", "bob")
	stdout, stderr, code := runMailxEnv(t, d, "from mary-ann\nexit\n", env, "-N")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stderr, "invalid message range") {
		t.Fatalf("hyphenated address rejected as a range: %q", stderr)
	}
	if !strings.Contains(stdout, "mary-ann") || strings.Contains(stdout, "bob") {
		t.Fatalf("address selector picked the wrong messages: %q", stdout)
	}

	stdout, stderr, code = runMailxEnv(t, d, "from 1-2\nexit\n", env, "-N")
	if code != 0 {
		t.Fatalf("range: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "mary-ann") || !strings.Contains(stdout, "bob") {
		t.Fatalf("numeric range lost a message: %q", stdout)
	}
}

// POSIX: "If both retain and discard commands are given, discard commands
// shall be ignored" -- the retained list wins whole, and a later discard of a
// retained header-field must not withdraw it.
func TestPOSIXRetainOverridesDiscardOfTheSameField(t *testing.T) {
	d, env := seedSenderMailbox(t, "bob")
	stdout, stderr, code := runMailxEnv(t, d, "retain subject\ndiscard subject\nprint 1\nexit\n", env, "-N")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "Subject: subject 1") {
		t.Fatalf("retained header-field suppressed by a later discard: %q", stdout)
	}
	for _, unwanted := range []string{"Date:", "From: bob", "To: alice"} {
		if strings.Contains(stdout, unwanted) {
			t.Fatalf("retain did not drop %q: %q", unwanted, stdout)
		}
	}

	// The reverse order keeps the same meaning: retain still wins.
	stdout, stderr, code = runMailxEnv(t, d, "discard subject\nretain subject\nprint 1\nexit\n", env, "-N")
	if code != 0 {
		t.Fatalf("reverse order: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "Subject: subject 1") || strings.Contains(stdout, "To: alice") {
		t.Fatalf("reverse order output=%q", stdout)
	}

	// With no retained header-field, discard alone still suppresses.
	stdout, stderr, code = runMailxEnv(t, d, "discard subject\nprint 1\nexit\n", env, "-N")
	if code != 0 {
		t.Fatalf("discard only: code=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stdout, "Subject:") || !strings.Contains(stdout, "To: alice") {
		t.Fatalf("discard-only output=%q", stdout)
	}
}

// POSIX: "crt=number ... If it is set to null, the value used is
// implementation-defined." A bare "set crt" therefore still enables
// pagination; this implementation uses the screenful size.
func TestPOSIXNullCrtKeepsPaginationEnabled(t *testing.T) {
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}}
	s := &mailSession{rc: rc, invoked: cmd, vars: defaultVariables(rc)}

	if n, set := s.crtLines(); set || n != 0 {
		t.Fatalf("unset crt: n=%d set=%v", n, set)
	}
	if err := s.cmdSet([]string{"crt"}); err != nil {
		t.Fatal(err)
	}
	n, set := s.crtLines()
	if !set || n != 20 {
		t.Fatalf("null crt: n=%d set=%v, want the screenful size", n, set)
	}
	if err := s.cmdSet([]string{"screen=8"}); err != nil {
		t.Fatal(err)
	}
	if n, set = s.crtLines(); !set || n != 8 {
		t.Fatalf("null crt with screen=8: n=%d set=%v", n, set)
	}
	if err := s.cmdSet([]string{"crt=40"}); err != nil {
		t.Fatal(err)
	}
	if n, set = s.crtLines(); !set || n != 40 {
		t.Fatalf("crt=40: n=%d set=%v", n, set)
	}
	if err := s.cmdSet([]string{"nocrt"}); err != nil {
		t.Fatal(err)
	}
	if n, set = s.crtLines(); set || n != 0 {
		t.Fatalf("nocrt: n=%d set=%v", n, set)
	}
}

// The pagination decision itself -- terminal standard output plus a message
// longer than crt -- is verified in process; only the pipe through PAGER
// still needs a terminal.
func TestPOSIXPaginationDecisionRequiresTerminalAndCrt(t *testing.T) {
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}}
	s := &mailSession{rc: rc, invoked: cmd, vars: defaultVariables(rc)}

	old := mailxIsTerminal
	mailxIsTerminal = func(any) bool { return true }
	t.Cleanup(func() { mailxIsTerminal = old })

	if s.shouldPage(1000) {
		t.Fatal("paginated with crt unset")
	}
	if err := s.cmdSet([]string{"crt", "screen=8"}); err != nil {
		t.Fatal(err)
	}
	if s.shouldPage(8) {
		t.Fatal("paginated a message no longer than crt")
	}
	if !s.shouldPage(9) {
		t.Fatal("did not paginate a message longer than null crt")
	}
	mailxIsTerminal = func(any) bool { return false }
	if s.shouldPage(9) {
		t.Fatal("paginated when standard output is not a terminal")
	}
}

// ~a and ~A are defined as ~i sign and ~i Sign, so ~i has to expand the \t and
// \n sequences those variables recognize.
func TestPOSIXInsertVariableMatchesSignEscape(t *testing.T) {
	rc := &tool.RunContext{Dir: t.TempDir(), Stdio: tool.Stdio{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}}
	input := make(chan composeRead, 8)
	s := &mailSession{rc: rc, invoked: cmd, user: "alice", vars: defaultVariables(rc), input: input, interrupt: make(chan os.Signal), stopInterrupt: func() {}, receive: true}
	s.vars["sign"] = `one\ntwo`
	s.vars["Sign"] = `alpha\tbeta`
	for _, line := range []string{"body\n", "~a\n", "~i sign\n", "~A\n", "~i Sign\n", "~.\n"} {
		input <- composeRead{line: line}
	}
	body, _, _, _, _, send, err := s.readComposition(nil, nil, nil, "")
	if err != nil || !send {
		t.Fatalf("send=%v err=%v", send, err)
	}
	want := "body\none\ntwo\none\ntwo\nalpha\tbeta\nalpha\tbeta\n"
	if string(body) != want {
		t.Fatalf("body=%q want=%q", body, want)
	}
}
