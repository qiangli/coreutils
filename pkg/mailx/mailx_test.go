package mailx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseMessagePreservesFoldedHeaders(t *testing.T) {
	raw := []byte("Subject: hello\r\n\tworld\r\nTo: a@example.com\r\n\r\nbody\r\n")
	msg, err := ParseMessage(raw)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	if got := msg.HeaderValues("subject"); len(got) != 1 || got[0] != "hello\nworld" {
		t.Fatalf("subject = %#v, want folded value", got)
	}
	if got := string(msg.Body); got != "body\r\n" {
		t.Fatalf("body = %q", got)
	}
	if got := string(msg.Bytes()); got != string(raw) {
		t.Fatalf("roundtrip mismatch:\n got %q\nwant %q", got, raw)
	}
}

func TestParseMessageRejectsMalformedHeader(t *testing.T) {
	_, err := ParseMessage([]byte("broken\n\nbody"))
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed", err)
	}
}

// A Message built by hand (no RawHeaders/Header.Raw, as a future caller that
// edits Headers post-parse would produce) must still serialize a multi-line
// Value as an RFC-style continuation, or Bytes() emits data ParseMessage
// itself rejects.
func TestMessageBytesFoldsMultilineHeaderValueWithoutRaw(t *testing.T) {
	msg := &Message{
		Headers: []Header{{Name: "Subject", Value: "hello\nworld"}},
		Body:    []byte("body\n"),
	}
	out := msg.Bytes()
	reparsed, err := ParseMessage(out)
	if err != nil {
		t.Fatalf("ParseMessage(Bytes()) = %v; serialized %q was not re-parseable", err, out)
	}
	if got := reparsed.HeaderValues("Subject"); len(got) != 1 || got[0] != "hello\nworld" {
		t.Fatalf("subject = %#v, want folded value round-tripped", got)
	}
}

func TestAppendMboxWritesEnvelopeAndEscapesFromLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spool.mbox")
	msg, err := ParseMessage([]byte("Subject: hi\n\nFrom inside\nplain\n"))
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	when := time.Date(2026, 8, 12, 17, 4, 5, 0, time.UTC)
	if err := AppendMbox(path, "agent@example.com", when, msg); err != nil {
		t.Fatalf("AppendMbox: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	wantPrefix := "From agent@example.com Wed Aug 12 17:04:05 2026\nSubject: hi\n\n>From inside\nplain\n\n"
	if string(got) != wantPrefix {
		t.Fatalf("mailbox = %q, want %q", got, wantPrefix)
	}
}

func TestAppendMboxReturnsBusyWhenLockExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spool.mbox")
	if err := os.WriteFile(path+".lock", []byte("held"), 0o600); err != nil {
		t.Fatalf("WriteFile(lock): %v", err)
	}
	msg, err := ParseMessage([]byte("Subject: hi\n\nbody\n"))
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	err = AppendMbox(path, "agent@example.com", time.Unix(0, 0), msg)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("mailbox file exists after busy lock, want untouched (stat err = %v)", statErr)
	}
}

// A lock file that outlives its writer (crash, kill -9) is never broken or
// retried — acquireLock treats it exactly like a live holder and returns
// ErrBusy forever. This pins that policy: it's explicitly unsupported, not
// an oversight.
func TestAppendMboxStaleLockNeverRecovers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spool.mbox")
	if err := os.WriteFile(path+".lock", []byte("stale"), 0o600); err != nil {
		t.Fatalf("WriteFile(lock): %v", err)
	}
	msg, err := ParseMessage([]byte("Subject: hi\n\nbody\n"))
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	for i := range 3 {
		if err := AppendMbox(path, "agent@example.com", time.Unix(0, 0), msg); !errors.Is(err, ErrBusy) {
			t.Fatalf("attempt %d: err = %v, want ErrBusy", i, err)
		}
	}
}

func TestAppendMboxCreatesMissingParentDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "spool.mbox")
	msg, err := ParseMessage([]byte("Subject: hi\n\nbody\n"))
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	if err := AppendMbox(path, "agent@example.com", time.Unix(0, 0), msg); err != nil {
		t.Fatalf("AppendMbox: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("mailbox not created: %v", err)
	}
}

func TestAppendMboxRejectsNilMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spool.mbox")
	err := AppendMbox(path, "agent@example.com", time.Unix(0, 0), nil)
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("mailbox file exists after nil message, want untouched (stat err = %v)", statErr)
	}
}

func TestAppendMboxConcurrentAppendsDoNotCorruptMailbox(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spool.mbox")

	const attempts = 20
	var wg sync.WaitGroup
	var mu sync.Mutex
	var successes int
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			msg, err := ParseMessage(fmt.Appendf(nil, "Subject: msg-%d\n\nbody-%d\n", i, i))
			if err != nil {
				t.Errorf("ParseMessage(%d): %v", i, err)
				return
			}
			err = AppendMbox(path, "agent@example.com", time.Unix(int64(i), 0), msg)
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
				return
			}
			if !errors.Is(err, ErrBusy) {
				t.Errorf("AppendMbox(%d): unexpected err %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	if successes == 0 {
		t.Fatal("no concurrent append succeeded")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	fromLines := bytes.Count(got, []byte("\nFrom ")) + boolToInt(bytes.HasPrefix(got, []byte("From ")))
	if fromLines != successes {
		t.Fatalf("mailbox has %d envelope lines, want %d matching successful appends; content:\n%q", fromLines, successes, got)
	}
	// Every successful append must have left the mailbox ending with the
	// standard blank-line separator, i.e. no interleaved/truncated writes.
	if len(got) > 0 && !bytes.HasSuffix(got, []byte("\n\n")) {
		t.Fatalf("mailbox does not end with a blank-line separator: %q", got)
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestLocalMboxTransportUsesInjectedClock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spool.mbox")
	msg, err := ParseMessage([]byte("Subject: hi\n\nbody\n"))
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	when := time.Date(2026, 8, 12, 9, 30, 0, 0, time.UTC)
	tx := LocalMboxTransport{
		MailboxPath: path,
		Sender:      "transport@example.com",
		Now:         func() time.Time { return when },
	}
	if err := tx.Deliver(context.Background(), msg, []string{"to@example.com"}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.HasPrefix(string(got), "From transport@example.com Wed Aug 12 09:30:00 2026\n") {
		t.Fatalf("mailbox prefix = %q", bytes.SplitN(got, []byte{'\n'}, 2)[0])
	}
}

func TestAppendMboxRejectsEnvelopeSenderInjection(t *testing.T) {
	dir := t.TempDir()
	msg, err := ParseMessage([]byte("Subject: hi\n\nbody\n"))
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	badSenders := []string{
		"sender@example.com\r\nFrom evil@example.com",
		"sender@example.com\nFrom evil@example.com",
		"sender@example.com\r",
		"sender@example.com\x00",
		"sender\x07@example.com",
		"sender\x1b@example.com",
		"sender\x7f@example.com",
	}
	for i, s := range badSenders {
		path := filepath.Join(dir, fmt.Sprintf("sub_%d", i), "spool.mbox")
		err := AppendMbox(path, s, time.Unix(0, 0), msg)
		if !errors.Is(err, ErrMalformed) {
			t.Errorf("sender %q: err = %v, want ErrMalformed", s, err)
		}
		if _, statErr := os.Stat(filepath.Dir(path)); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("sender %q left directory behind", s)
		}
		if _, statErr := os.Stat(path + ".lock"); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("sender %q left lock file behind", s)
		}
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("sender %q left mailbox file behind", s)
		}
	}
}

func TestAppendMboxRejectsHeaderNameInjection(t *testing.T) {
	badNames := []string{
		"",
		"Sub ject",
		"Sub:ject",
		"Sub\r\nject",
		"Sub\nject",
		"Sub\x00ject",
		"Sub\x07ject",
		"Header\x80",
	}
	for _, name := range badNames {
		msg := &Message{
			Headers: []Header{{Name: name, Value: "hello"}},
			Body:    []byte("body\n"),
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "sub", "spool.mbox")
		err := AppendMbox(path, "agent@example.com", time.Unix(0, 0), msg)
		if !errors.Is(err, ErrMalformed) {
			t.Errorf("header name %q: err = %v, want ErrMalformed", name, err)
		}
		if _, statErr := os.Stat(filepath.Dir(path)); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("header name %q left directory behind", name)
		}
	}
}

func TestAppendMboxRejectsHeaderValueInjection(t *testing.T) {
	badValues := []string{
		"hello\r\nTo: evil@example.com",
		"hello\rworld",
		"hello\x00world",
		"hello\x07world",
		"hello\x1bworld",
		"hello\n\nBody injection",
		"hello\n",
		"\nhello",
	}
	for _, val := range badValues {
		msg := &Message{
			Headers: []Header{{Name: "Subject", Value: val}},
			Body:    []byte("body\n"),
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "sub", "spool.mbox")
		err := AppendMbox(path, "agent@example.com", time.Unix(0, 0), msg)
		if !errors.Is(err, ErrMalformed) {
			t.Errorf("header value %q: err = %v, want ErrMalformed", val, err)
		}
		if _, statErr := os.Stat(filepath.Dir(path)); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("header value %q left directory behind", val)
		}
	}
}

func TestParseMessageRejectsInjections(t *testing.T) {
	badRaw := [][]byte{
		[]byte("Sub ject: hello\n\nbody\n"),
		[]byte(": hello\n\nbody\n"),
		[]byte("Subject\x07: hello\n\nbody\n"),
		[]byte("Subject: hello\x00world\n\nbody\n"),
		[]byte("Subject: hello\x07world\n\nbody\n"),
	}
	for i, raw := range badRaw {
		_, err := ParseMessage(raw)
		if !errors.Is(err, ErrMalformed) {
			t.Errorf("badRaw[%d] %q: err = %v, want ErrMalformed", i, raw, err)
		}
	}
}

func TestAppendMboxNoArtifactOnNilOrInvalidMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "spool.mbox")

	// Nil message
	err := AppendMbox(path, "agent@example.com", time.Unix(0, 0), nil)
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("nil msg err = %v, want ErrMalformed", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "a")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("nil message created directory 'a'")
	}

	// Invalid message
	invalidMsg := &Message{
		Headers: []Header{{Name: "Bad Header Name", Value: "val"}},
	}
	err = AppendMbox(path, "agent@example.com", time.Unix(0, 0), invalidMsg)
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("invalid msg err = %v, want ErrMalformed", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "a")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("invalid message created directory 'a'")
	}
}
