package mailxcmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mailxpkg "github.com/qiangli/coreutils/pkg/mailx"
	"github.com/qiangli/coreutils/tool"
)

type mailxFailWriter struct {
	err   error
	short bool
}

func (w mailxFailWriter) Write(p []byte) (int, error) {
	if w.short && len(p) > 0 {
		return len(p) - 1, nil
	}
	return 0, w.err
}

func invoke(t *testing.T, stdin string, args ...string) (string, string, int, string) {
	t.Helper()
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Dir:   dir,
		Env:   []string{"LOGNAME=alice", "HOME=" + dir, "MAILX_SPOOL=spool"},
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errOut},
	}
	code := run(rc, args)
	return out.String(), errOut.String(), code, dir
}

func withClock(t *testing.T) {
	t.Helper()
	old := nowFn
	nowFn = func() time.Time { return time.Date(2026, 8, 26, 12, 30, 0, 0, time.UTC) }
	t.Cleanup(func() { nowFn = old })
}

func TestSendDeliversMboxToEachLocalRecipient(t *testing.T) {
	withClock(t)
	_, stderr, code, dir := invoke(t, "hello\nFrom body\n", "-s", "greeting", "bob", "carol")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	for _, user := range []string{"bob", "carol"} {
		entries, err := mailxpkg.ReadMbox(filepath.Join(dir, "spool", user))
		if err != nil || len(entries) != 1 {
			t.Fatalf("%s mailbox: entries=%d err=%v", user, len(entries), err)
		}
		msg := entries[0].Message
		if got := msg.HeaderValues("Subject"); len(got) != 1 || got[0] != "greeting" {
			t.Errorf("%s Subject = %#v", user, got)
		}
		if got := string(msg.Body); got != "hello\nFrom body\n" {
			t.Errorf("%s body = %q", user, got)
		}
	}
}

func TestRemoteRecipientFailsBeforeAnyDelivery(t *testing.T) {
	_, stderr, code, dir := invoke(t, "body\n", "bob", "remote@example.test")
	if code != 1 || !strings.Contains(stderr, "remote delivery is not supported") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "spool")); !os.IsNotExist(err) {
		t.Fatalf("spool created despite rejected recipient: %v", err)
	}
}

func TestSendBCCDoesNotExposeHeader(t *testing.T) {
	withClock(t)
	_, stderr, code, dir := invoke(t, "secret\n", "-b", "bob", "carol")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	entries, err := mailxpkg.ReadMbox(filepath.Join(dir, "spool", "bob"))
	if err != nil {
		t.Fatal(err)
	}
	if got := entries[0].Message.HeaderValues("Bcc"); len(got) != 0 {
		t.Fatalf("Bcc leaked into delivered message: %#v", got)
	}
}

func TestHeaderRecipientMode(t *testing.T) {
	withClock(t)
	raw := "To: bob\nCc: carol\nSubject: from headers\n\nbody\n"
	_, stderr, code, dir := invoke(t, raw, "-t")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	for _, user := range []string{"bob", "carol"} {
		entries, err := mailxpkg.ReadMbox(filepath.Join(dir, "spool", user))
		if err != nil || len(entries) != 1 {
			t.Fatalf("%s: %#v, %v", user, entries, err)
		}
		if got := string(entries[0].Message.Body); got != "body\n" {
			t.Errorf("body = %q", got)
		}
	}
}

func seedMailbox(t *testing.T, dir string, subjects ...string) string {
	t.Helper()
	path := filepath.Join(dir, "spool", "alice")
	for i, subject := range subjects {
		msg := &mailxpkg.Message{Headers: []mailxpkg.Header{{Name: "From", Value: "bob"}, {Name: "Subject", Value: subject}}, Body: []byte("body " + subject + "\n")}
		if err := mailxpkg.AppendMbox(path, "bob", time.Unix(int64(i), 0), msg); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestExistAndHeadersModes(t *testing.T) {
	dir := t.TempDir()
	path := seedMailbox(t, dir, "one", "two")
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Dir: dir, Env: []string{"LOGNAME=alice", "MAIL=" + path}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
	if code := run(rc, []string{"-e"}); code != 0 || out.Len() != 0 {
		t.Fatalf("-e: exit %d output %q", code, out.String())
	}
	out.Reset()
	if code := run(rc, []string{"-H"}); code != 0 {
		t.Fatalf("-H exit %d: %s", code, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "one") || !strings.Contains(got, "two") {
		t.Fatalf("headers = %q", got)
	}
}

func TestOutputFailuresReachCommandBoundary(t *testing.T) {
	dir := t.TempDir()
	path := seedMailbox(t, dir, "one")
	for _, tc := range []struct {
		name string
		out  io.Writer
		want string
	}{
		{name: "error", out: mailxFailWriter{err: errors.New("output unavailable")}, want: "output unavailable"},
		{name: "short write", out: mailxFailWriter{short: true}, want: "short write"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errOut bytes.Buffer
			rc := &tool.RunContext{
				Dir:   dir,
				Env:   []string{"LOGNAME=alice", "MAIL=" + path},
				Stdio: tool.Stdio{In: strings.NewReader(""), Out: tc.out, Err: &errOut},
			}
			if code := run(rc, []string{"-H", "-n"}); code != 1 {
				t.Fatalf("exit %d, want 1", code)
			}
			if got := errOut.String(); !strings.Contains(got, "mailx: write error:") || !strings.Contains(got, tc.want) {
				t.Fatalf("diagnostic = %q", got)
			}
		})
	}
}

func TestFileWriteHelperRejectsShortWrites(t *testing.T) {
	if err := writeBytes(mailxFailWriter{short: true}, []byte("data")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("write error=%v, want %v", err, io.ErrShortWrite)
	}
}

func TestDeleteQuitRewritesMailbox(t *testing.T) {
	dir := t.TempDir()
	path := seedMailbox(t, dir, "one", "two")
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Dir: dir, Env: []string{"LOGNAME=alice", "MAIL=" + path}, Stdio: tool.Stdio{In: strings.NewReader("d 1\nq\n"), Out: &out, Err: &errOut}}
	if code := run(rc, []string{"-N"}); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	entries, err := mailxpkg.ReadMbox(path)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%d err=%v", len(entries), err)
	}
	if got := entries[0].Message.HeaderValues("Subject"); len(got) != 1 || got[0] != "two" {
		t.Fatalf("remaining subject %#v", got)
	}
}

type triggeringReader struct {
	reader  *strings.Reader
	trigger func()
}

func (r *triggeringReader) Read(p []byte) (int, error) {
	if r.trigger != nil {
		trigger := r.trigger
		r.trigger = nil
		trigger()
	}
	return r.reader.Read(p)
}

func TestDeleteQuitPreservesConcurrentAppend(t *testing.T) {
	dir := t.TempDir()
	path := seedMailbox(t, dir, "one", "two")
	appended := &mailxpkg.Message{Headers: []mailxpkg.Header{{Name: "From", Value: "carol"}, {Name: "Subject", Value: "arrived-late"}}, Body: []byte("new body\n")}
	var appendErr error
	in := &triggeringReader{
		reader: strings.NewReader("d 1\nq\n"),
		trigger: func() {
			appendErr = mailxpkg.AppendMbox(path, "carol", time.Unix(10, 0), appended)
		},
	}
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Dir: dir, Env: []string{"LOGNAME=alice", "MAIL=" + path}, Stdio: tool.Stdio{In: in, Out: &out, Err: &errOut}}
	if code := run(rc, []string{"-N"}); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	if appendErr != nil {
		t.Fatalf("interleaved append: %v", appendErr)
	}
	entries, err := mailxpkg.ReadMbox(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want two retained messages", len(entries))
	}
	got := []string{entries[0].Message.HeaderValues("Subject")[0], entries[1].Message.HeaderValues("Subject")[0]}
	if got[0] != "two" || got[1] != "arrived-late" {
		t.Fatalf("remaining subjects = %#v", got)
	}
}

func TestFileOptionWithoutOperandReadsMBOX(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "saved-mail")
	msg := &mailxpkg.Message{Headers: []mailxpkg.Header{{Name: "From", Value: "bob"}, {Name: "Subject", Value: "saved"}}, Body: []byte("body\n")}
	if err := mailxpkg.AppendMbox(path, "bob", time.Unix(0, 0), msg); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Dir: dir, Env: []string{"LOGNAME=alice", "MBOX=" + path}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
	if code := run(rc, []string{"-f", "-H"}); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "saved") {
		t.Fatalf("headers = %q", out.String())
	}
}

func TestFileOptionConsumesExplicitOperand(t *testing.T) {
	dir := t.TempDir()
	path := seedMailbox(t, dir, "explicit")
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Dir: dir, Env: []string{"LOGNAME=alice"}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
	if code := run(rc, []string{"-f", path, "-H"}); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "explicit") {
		t.Fatalf("headers = %q", out.String())
	}
}

func TestFileOptionOperandCanFollowOtherOptions(t *testing.T) {
	dir := t.TempDir()
	path := seedMailbox(t, dir, "after-options")
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Dir: dir, Env: []string{"LOGNAME=alice"}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
	if code := run(rc, []string{"-f", "-H", "--", path}); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "after-options") {
		t.Fatalf("headers = %q", out.String())
	}
}

func TestRecordByFirstRecipient(t *testing.T) {
	withClock(t)
	_, stderr, code, dir := invoke(t, "record me\n", "-F", "bob", "carol")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	entries, err := mailxpkg.ReadMbox(filepath.Join(dir, "bob"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || string(entries[0].Message.Body) != "record me\n" {
		t.Fatalf("record entries = %#v", entries)
	}
	if _, err := os.Stat(filepath.Join(dir, "carol")); !os.IsNotExist(err) {
		t.Fatalf("unexpected record for second recipient: %v", err)
	}
}

func TestExitDiscardsDeletion(t *testing.T) {
	dir := t.TempDir()
	path := seedMailbox(t, dir, "one")
	rc := &tool.RunContext{Dir: dir, Env: []string{"LOGNAME=alice", "MAIL=" + path}, Stdio: tool.Stdio{In: strings.NewReader("d\nx\n"), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}}
	if code := run(rc, []string{"-N"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	entries, err := mailxpkg.ReadMbox(path)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%d err=%v", len(entries), err)
	}
}

func TestMailAliasIsExecutable(t *testing.T) {
	alias := tool.Lookup("mail")
	if alias == nil || alias.Run == nil {
		t.Fatal("mail alias is not registered")
	}
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Dir: t.TempDir(), Env: []string{"LOGNAME=alice", "HOME=" + t.TempDir()}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
	if code := alias.Run(rc, []string{"-e"}); code != 1 {
		t.Fatalf("mail -e exit %d, want 1", code)
	}
}

func TestMailAliasOwnsHelpAndDiagnostics(t *testing.T) {
	alias := tool.Lookup("mail")
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Dir: t.TempDir(), Env: []string{"LOGNAME=alice", "HOME=" + t.TempDir()}, Stdio: tool.Stdio{In: strings.NewReader("body\n"), Out: &out, Err: &errOut}}
	if code := alias.Run(rc, []string{"remote@example.test"}); code != 1 {
		t.Fatalf("diagnostic exit = %d", code)
	}
	if got := errOut.String(); !strings.HasPrefix(got, "mail: ") || strings.Contains(got, "mailx: ") {
		t.Fatalf("alias diagnostic = %q", got)
	}
	out.Reset()
	errOut.Reset()
	if code := alias.Run(rc, []string{"--help"}); code != 0 {
		t.Fatalf("help exit = %d, stderr %q", code, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "Usage: mail ") || strings.Contains(got, "Usage: mailx ") {
		t.Fatalf("alias help = %q", got)
	}
}
