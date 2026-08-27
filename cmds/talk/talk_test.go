package talkcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/cmds/internal/session"
	"github.com/qiangli/coreutils/tool"
)

type fakeConversation struct {
	id       string
	joined   bool
	joinErr  error
	sent     []string
	incoming []string
	closed   bool
	cleaned  bool
	closeErr error
	cleanErr error
}

type passthroughDisplay struct{}

func (*passthroughDisplay) Local(data []byte, accept bool) ([]string, bool, error) {
	if !accept {
		return nil, false, nil
	}
	return []string{string(data)}, false, nil
}
func (*passthroughDisplay) Remote(string) error     { return nil }
func (*passthroughDisplay) PeerClosed(string) error { return nil }
func (*passthroughDisplay) Close() error            { return nil }

func (f *fakeConversation) ID() string          { return f.id }
func (f *fakeConversation) Join() (bool, error) { return f.joined, f.joinErr }
func (f *fakeConversation) Send(s string) error { f.sent = append(f.sent, s); return nil }
func (f *fakeConversation) Poll() ([]string, bool, error) {
	in := f.incoming
	f.incoming = nil
	return in, f.closed, nil
}
func (f *fakeConversation) Close() error   { f.closed = true; return f.closeErr }
func (f *fakeConversation) Cleanup() error { f.cleaned = true; return f.cleanErr }

func installFakes(t *testing.T, conv *fakeConversation) *string {
	t.Helper()
	oldTerminal, oldSessions, oldPermission := isTerminalFn, readSessionsFn, recipientPermittedFn
	oldCurrent, oldLookup, oldNotify := currentAccountFn, lookupAccountFn, notifyTTYFn
	oldOpen, oldInput, oldPoll := openConversationFn, newInputFn, pollInterval
	oldInterrupt, oldOwner, oldAlive, oldRoot := watchInterruptFn, fileOwnerUIDFn, processAliveFn, validateSharedRootFn
	oldCheckTerminal, oldDisplay := checkTerminalCapabilitiesFn, newDisplayFn
	t.Cleanup(func() {
		isTerminalFn, readSessionsFn, recipientPermittedFn = oldTerminal, oldSessions, oldPermission
		currentAccountFn, lookupAccountFn, notifyTTYFn = oldCurrent, oldLookup, oldNotify
		openConversationFn, newInputFn, pollInterval = oldOpen, oldInput, oldPoll
		watchInterruptFn, fileOwnerUIDFn, processAliveFn = oldInterrupt, oldOwner, oldAlive
		validateSharedRootFn = oldRoot
		checkTerminalCapabilitiesFn, newDisplayFn = oldCheckTerminal, oldDisplay
	})
	isTerminalFn = func(any) bool { return true }
	currentAccountFn = func() (account, error) { return account{Name: "alice", UID: "1001"}, nil }
	lookupAccountFn = func(name string) (account, error) {
		if name == "bob" {
			return account{Name: "bob", UID: "1002"}, nil
		}
		if name == "alice" {
			return account{Name: "alice", UID: "1001"}, nil
		}
		return account{}, errors.New("unknown")
	}
	readSessionsFn = func([]string) ([]session.Record, error) {
		return []session.Record{{User: "bob", TTY: "pts/7", Type: "USER_PROCESS"}}, nil
	}
	recipientPermittedFn = func(session.Record, account) (bool, error) { return true, nil }
	notice := ""
	notifyTTYFn = func(_ session.Record, _ account, text string) error { notice += text; return nil }
	openConversationFn = func(string, account, account) (conversation, error) { return conv, nil }
	pollInterval = time.Millisecond
	watchInterruptFn = func() (<-chan os.Signal, func()) { return make(chan os.Signal), func() {} }
	checkTerminalCapabilitiesFn = func(*tool.RunContext) error { return nil }
	newDisplayFn = func(*tool.RunContext, string, string) (display, error) { return &passthroughDisplay{}, nil }
	return &notice
}

func invoke(t *testing.T, in string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(),
		Env:   []string{"USER=mallory", "LOGNAME=mallory", "TERM=xterm"},
		Stdio: tool.Stdio{In: io.NopCloser(strings.NewReader(in)), Out: &out, Err: &errOut}}
	return run(rc, args), out.String(), errOut.String()
}

func TestRunUsesOSIdentityNotEnvironmentAndNotifiesTTY(t *testing.T) {
	conv := &fakeConversation{id: "private", joined: true}
	notice := installFakes(t, conv)
	code, stdout, stderr := invoke(t, "hello\n", "bob")
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(*notice, "Message from alice") || strings.Contains(*notice, "mallory") {
		t.Fatalf("tty notice = %q", *notice)
	}
	if len(conv.sent) != 1 || conv.sent[0] != "hello\n" || !conv.closed || !conv.cleaned {
		t.Fatalf("conversation = %#v", conv)
	}
	if !strings.Contains(stdout, "private session private connected") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestConversationCloseAndCleanupFailuresSetErrorStatus(t *testing.T) {
	for _, tc := range []struct {
		name string
		conv *fakeConversation
		want string
	}{
		{name: "close", conv: &fakeConversation{id: "private", joined: true, closeErr: errors.New("close failed")}, want: "close private local session"},
		{name: "cleanup", conv: &fakeConversation{id: "private", joined: true, cleanErr: errors.New("cleanup failed")}, want: "clean up private local session"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			installFakes(t, tc.conv)
			code, stdout, stderr := invoke(t, "", "bob")
			if code != 1 || stderr != "" || !strings.Contains(stdout, tc.want) {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			if !tc.conv.closed || !tc.conv.cleaned {
				t.Fatalf("conversation=%#v", tc.conv)
			}
		})
	}
}

type talkShortWriter struct{}

func (talkShortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

func TestTalkWriteHelperRejectsShortWrites(t *testing.T) {
	if err := writeBytes(talkShortWriter{}, []byte("data")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("write error=%v, want %v", err, io.ErrShortWrite)
	}
}

func TestDefaultAlwaysRequiresWhoAndMesg(t *testing.T) {
	for _, tc := range []struct {
		name      string
		sessions  []session.Record
		permitted bool
		want      string
	}{
		{"logged-out", nil, true, "not a local logged-in user"},
		{"mesg-no", []session.Record{{User: "bob", TTY: "pts/7", Type: "USER_PROCESS"}}, false, "denying terminal messages"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conv := &fakeConversation{id: "x", joined: true}
			installFakes(t, conv)
			readSessionsFn = func([]string) ([]session.Record, error) { return tc.sessions, nil }
			recipientPermittedFn = func(session.Record, account) (bool, error) { return tc.permitted, nil }
			opened := false
			openConversationFn = func(string, account, account) (conversation, error) { opened = true; return conv, nil }
			code, stdout, _ := invoke(t, "", "bob")
			if code == 0 || !strings.Contains(stdout, tc.want) || opened {
				t.Fatalf("exit=%d stdout=%q opened=%v", code, stdout, opened)
			}
		})
	}
}

func TestTerminalSelectionAndNotificationFailureFailClosed(t *testing.T) {
	conv := &fakeConversation{id: "x", joined: true}
	installFakes(t, conv)
	if code, stdout, _ := invoke(t, "", "bob", "pts/99"); code == 0 || !strings.Contains(stdout, "not logged in on local terminal") {
		t.Fatalf("wrong tty exit=%d stdout=%q", code, stdout)
	}
	notifyTTYFn = func(session.Record, account, string) error { return errors.New("tty vanished") }
	if code, stdout, _ := invoke(t, "", "bob", "pts/7"); code == 0 || !strings.Contains(stdout, "tty vanished") || !conv.cleaned {
		t.Fatalf("notify exit=%d stdout=%q cleaned=%v", code, stdout, conv.cleaned)
	}
}

func TestRecipientSelectionSkipsStaleTTYWhenAnotherIsUsable(t *testing.T) {
	conv := &fakeConversation{id: "x", joined: true}
	notice := installFakes(t, conv)
	readSessionsFn = func([]string) ([]session.Record, error) {
		return []session.Record{
			{User: "bob", TTY: "pts/stale", Type: "USER_PROCESS"},
			{User: "bob", TTY: "pts/usable", Type: "USER_PROCESS"},
		}, nil
	}
	recipientPermittedFn = func(record session.Record, _ account) (bool, error) {
		if record.TTY == "pts/stale" {
			return false, os.ErrNotExist
		}
		return true, nil
	}
	code, _, _ := invoke(t, "", "bob")
	if code != 0 || !strings.Contains(*notice, "connection requested") {
		t.Fatalf("exit=%d notice=%q", code, *notice)
	}
}

func TestRemoteRejectedBeforeSessionOrNotification(t *testing.T) {
	conv := &fakeConversation{id: "x", joined: true}
	notice := installFakes(t, conv)
	opened := false
	openConversationFn = func(string, account, account) (conversation, error) { opened = true; return conv, nil }
	for _, address := range []string{"bob@example.com", "host:bob", "host!bob", "/bob", `host\bob`} {
		code, stdout, _ := invoke(t, "", address)
		if code == 0 || !strings.Contains(stdout, "localhost-only") {
			t.Fatalf("%q exit=%d stdout=%q", address, code, stdout)
		}
	}
	if opened || *notice != "" {
		t.Fatalf("opened=%v notice=%q", opened, *notice)
	}
}

func TestSyntaxAndTerminalRequirements(t *testing.T) {
	conv := &fakeConversation{id: "x", joined: true}
	installFakes(t, conv)
	for _, args := range [][]string{nil, {"bob", "pts/7", "extra"}, {"-x", "bob"}} {
		code, stdout, stderr := invoke(t, "", args...)
		if code == 0 || stdout == "" || stderr != "" {
			t.Fatalf("args=%q exit=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
	isTerminalFn = func(any) bool { return false }
	if code, stdout, _ := invoke(t, "", "bob"); code == 0 || !strings.Contains(stdout, "standard input is not a terminal") {
		t.Fatalf("exit=%d stdout=%q", code, stdout)
	}
}

func TestLocalUnixTransportIsPrivateAuthenticatedEphemeralAndConverges(t *testing.T) {
	if _, err := net.ResolveUnixAddr("unixgram", filepath.Join(t.TempDir(), "probe")); err != nil {
		t.Skip(err)
	}
	oldLookup, oldOwner, oldAlive, oldRoot := lookupAccountFn, fileOwnerUIDFn, processAliveFn, validateSharedRootFn
	t.Cleanup(func() {
		lookupAccountFn, fileOwnerUIDFn, processAliveFn, validateSharedRootFn = oldLookup, oldOwner, oldAlive, oldRoot
	})
	lookupAccountFn = func(name string) (account, error) {
		switch name {
		case "alice":
			return account{Name: "alice", UID: "1001"}, nil
		case "bob":
			return account{Name: "bob", UID: "1002"}, nil
		}
		return account{}, errors.New("unknown")
	}
	fileOwnerUIDFn = func(path string) (string, error) {
		base := filepath.Base(path)
		for _, uid := range []string{"1001", "1002"} {
			if strings.Contains(base, "-ep-"+uid+"-") || strings.Contains(base, "-sock-"+uid+"-") {
				return uid, nil
			}
		}
		return "", errors.New("unknown owner")
	}
	processAliveFn = func(int) bool { return true }
	validateSharedRootFn = func(string) error { return nil }
	root, err := os.MkdirTemp("/tmp", "talk-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	a, err := openLocalConversation(root, account{Name: "alice", UID: "1001"}, account{Name: "bob", UID: "1002"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := openLocalConversation(root, account{Name: "bob", UID: "1002"}, account{Name: "alice", UID: "1001"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Cleanup()
	defer b.Cleanup()
	if ok, err := a.Join(); !ok || err != nil {
		t.Fatalf("a join=%v err=%v", ok, err)
	}
	if ok, err := b.Join(); !ok || err != nil {
		t.Fatalf("b join=%v err=%v", ok, err)
	}
	if a.ID() == "" || a.ID() != b.ID() {
		t.Fatalf("ids a=%q b=%q", a.ID(), b.ID())
	}
	for _, c := range []*localConversation{a, b} {
		info, err := os.Lstat(c.socketPath)
		if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o622 {
			t.Fatalf("socket mode=%v err=%v", info, err)
		}
	}
	if err := a.Send("secret text\n"); err != nil {
		t.Fatal(err)
	}
	messages, closed, err := b.Poll()
	if err != nil || closed || len(messages) != 1 || messages[0] != "secret text\n" {
		t.Fatalf("messages=%q closed=%v err=%v", messages, closed, err)
	}
	entries, err := os.ReadDir(a.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.Contains(e.Name(), "-ep-") && !strings.Contains(e.Name(), "-sock-") {
			t.Fatalf("persistent transcript file %q", e.Name())
		}
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	_, closed, err = b.Poll()
	if err != nil || !closed {
		t.Fatalf("close=%v err=%v", closed, err)
	}
	if err := a.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := b.Cleanup(); err != nil {
		t.Fatal(err)
	}
	entries, err = os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("session entries remain: %v", entries)
	}
}

func TestAuthenticatedTransportDiscardsInjectedDatagramAndContinues(t *testing.T) {
	oldLookup, oldOwner, oldAlive, oldRoot := lookupAccountFn, fileOwnerUIDFn, processAliveFn, validateSharedRootFn
	t.Cleanup(func() {
		lookupAccountFn, fileOwnerUIDFn, processAliveFn, validateSharedRootFn = oldLookup, oldOwner, oldAlive, oldRoot
	})
	lookupAccountFn = func(name string) (account, error) {
		if name == "alice" {
			return account{Name: name, UID: "1001"}, nil
		}
		return account{Name: name, UID: "1002"}, nil
	}
	fileOwnerUIDFn = func(path string) (string, error) {
		base := filepath.Base(path)
		if strings.Contains(base, "-ep-1001-") || strings.Contains(base, "-sock-1001-") {
			return "1001", nil
		}
		return "1002", nil
	}
	processAliveFn = func(int) bool { return true }
	validateSharedRootFn = func(string) error { return nil }
	root, err := os.MkdirTemp("/tmp", "talk-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	a, err := openLocalConversation(root, account{Name: "alice", UID: "1001"}, account{Name: "bob", UID: "1002"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := openLocalConversation(root, account{Name: "bob", UID: "1002"}, account{Name: "alice", UID: "1001"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Cleanup()
	defer b.Cleanup()
	a.Join()
	b.Join()
	attacker, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: b.socketPath, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = attacker.Write([]byte(`{"session":"forged"}`))
	_ = attacker.Close()
	if err := a.Send("valid after garbage\n"); err != nil {
		t.Fatal(err)
	}
	messages, closed, err := b.Poll()
	if err != nil || closed || len(messages) != 1 || messages[0] != "valid after garbage\n" {
		t.Fatalf("messages=%q closed=%v err=%v", messages, closed, err)
	}
}

func TestEndpointOwnerMismatchCannotJoin(t *testing.T) {
	oldLookup, oldOwner, oldAlive, oldRoot := lookupAccountFn, fileOwnerUIDFn, processAliveFn, validateSharedRootFn
	t.Cleanup(func() {
		lookupAccountFn, fileOwnerUIDFn, processAliveFn, validateSharedRootFn = oldLookup, oldOwner, oldAlive, oldRoot
	})
	lookupAccountFn = func(name string) (account, error) {
		if name == "alice" {
			return account{Name: name, UID: "1001"}, nil
		}
		return account{Name: name, UID: "1002"}, nil
	}
	processAliveFn = func(int) bool { return true }
	validateSharedRootFn = func(string) error { return nil }
	root, err := os.MkdirTemp("/tmp", "talk-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	a, err := openLocalConversation(root, account{Name: "alice", UID: "1001"}, account{Name: "bob", UID: "1002"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := openLocalConversation(root, account{Name: "bob", UID: "1002"}, account{Name: "alice", UID: "1001"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Cleanup()
	defer b.Cleanup()
	fileOwnerUIDFn = func(path string) (string, error) {
		base := filepath.Base(path)
		if strings.Contains(base, "-ep-1002-") || strings.Contains(base, "-sock-1002-") {
			return "9999", nil
		}
		return "1001", nil
	}
	if joined, err := a.Join(); err != nil || joined {
		t.Fatalf("forged-owner join=%v err=%v", joined, err)
	}
}

func TestCancellationStopsClosableInputReader(t *testing.T) {
	r, w := io.Pipe()
	in, err := newAsyncInput(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := w.Write([]byte("future command\n")); err == nil {
		t.Fatal("cancelled reader still accepted future input")
	}
	_ = w.Close()
}

func TestAsyncInputCloseUnblocksFullEventChannel(t *testing.T) {
	r, w := io.Pipe()
	in, err := newAsyncInput(r)
	if err != nil {
		t.Fatal(err)
	}
	wrote := make(chan struct{})
	go func() {
		_, _ = io.WriteString(w, "one\ntwo\n")
		close(wrote)
	}()
	select {
	case <-wrote:
	case <-time.After(time.Second):
		t.Fatal("writer did not supply both lines")
	}
	if err := in.Close(); err != nil {
		t.Fatalf("close with full event channel: %v", err)
	}
	_ = w.Close()
}

func TestNonClosableEmbeddedInputIsRejected(t *testing.T) {
	if _, err := newAsyncInput(strings.NewReader("finite but not owned")); err == nil {
		t.Fatal("non-closable embedded input was accepted")
	}
}

func TestDefaultTalkRootIsResolvedRootOwnedStickyDirectory(t *testing.T) {
	root := defaultTalkRoot()
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("default root remains a symlink: %s", root)
	}
	if err := validateSharedRoot(root); err != nil {
		t.Fatalf("default root %s: %v", root, err)
	}
}

func TestConverseCancellationWithBlockedPipeReturnsAndClosesReader(t *testing.T) {
	conv := &fakeConversation{id: "x", joined: true}
	installFakes(t, conv)
	r, w := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	rc := &tool.RunContext{Ctx: ctx, Env: []string{"LC_ALL=C"}, Stdio: tool.Stdio{In: r, Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}}
	done := make(chan int, 1)
	go func() { done <- converse(rc, "alice", "bob", conv, make(chan os.Signal)) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit=%d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("converse did not cancel")
	}
	if _, err := w.Write([]byte("stolen\n")); err == nil {
		t.Fatal("reader survived cancellation")
	}
	_ = w.Close()
}

func TestPrintableInputAndUTF8Locales(t *testing.T) {
	body, refresh := printableInput([]byte("ok\a\f\x01\x7f\xff\n"), "POSIX")
	if !refresh || body != "ok\a^A^?\\xFF\n" {
		t.Fatalf("body=%q refresh=%v", body, refresh)
	}
	body, refresh = printableInput([]byte("héllo\t世界\n"), "en_US.UTF-8")
	if refresh || body != "héllo\t世界\n" {
		t.Fatalf("body=%q refresh=%v", body, refresh)
	}
	for _, name := range []string{"UTF-8", "C.UTF-8", "en_US.utf8", "de_DE.UTF-8@euro"} {
		if !isUTF8Locale(name) {
			t.Errorf("%q not UTF-8", name)
		}
	}
}
