package chat

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/room"
)

// ctlSockServer is a minimal stand-in for a live coachee's control socket: it
// accepts the frames NewCtlSteerer writes (one connection per ESC/Say) and
// accumulates them so a test can assert a steer actually crossed the wire. It
// models "the coachee": leaving its listener open proves attach did not kill it.
type ctlSockServer struct {
	ln     net.Listener
	mu     bytesMutex
	path   string
	closed bool
}

// bytesMutex guards a byte buffer with a mutex.
type bytesMutex struct {
	mu sync.RWMutex
	b  []byte
}

func (bm *bytesMutex) add(p []byte) {
	bm.mu.Lock()
	bm.b = append(bm.b, p...)
	bm.mu.Unlock()
}

func (bm *bytesMutex) get() []byte {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	out := make([]byte, len(bm.b))
	copy(out, bm.b)
	return out
} // newCtlSockServer listens on a short unix socket path (the ~104-byte address
// cap rules out a long tempdir path) and serves frames until Close.
func newCtlSockServer(t *testing.T) *ctlSockServer {
	t.Helper()
	// os.TempDir() is short enough; a 12-char hash keeps the full path well under
	// the unix-socket address limit.
	name := "coach-test-" + shortHash(time.Now().Format(time.RFC3339Nano)) + ".sock"
	path := filepath.Join(os.TempDir(), name)
	// A leftover from a previous run blocks the bind.
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen %s: %v", path, err)
	}
	s := &ctlSockServer{ln: ln}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed (Close, or the test ended)
			}
			data, _ := io.ReadAll(conn)
			conn.Close()
			s.mu.add(data)
		}
	}()
	// The socket file is the coachee's footprint; remove it when the test ends so
	// the temp dir stays clean. The listener itself is closed by Close().
	t.Cleanup(func() { s.Close(); _ = os.Remove(path) })
	s.path = path
	return s
}

func countEsc(b []byte) int { return bytes.Count(b, []byte("\x00R")) }

// settle waits for the server's recorded byte count to stop growing — i.e. for
// every connection the coach already opened to be drained by the server's reader
// goroutine. It returns once two samples taken settleWindow apart are equal (or
// the overall cap elapses). This removes a race where a frame sent just before
// detach is recorded by the server AFTER the coach has exited.
func settle(s *ctlSockServer) []byte {
	const (
		window = 120 * time.Millisecond
		cap    = 1500 * time.Millisecond
	)
	deadline := time.Now().Add(cap)
	prev := len(s.mu.get())
	for {
		time.Sleep(window)
		cur := s.mu.get()
		if len(cur) == prev || time.Now().After(deadline) {
			return cur
		}
		prev = len(cur)
	}
}

// dialAble reports whether the coachee's control socket still accepts
// connections — the observable sign the coachee is alive and was not killed.
func (s *ctlSockServer) dialAble() bool {
	c, err := net.DialTimeout("unix", s.path, time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func (s *ctlSockServer) Close() {
	s.mu.mu.Lock()
	if !s.closed {
		s.closed = true
		s.ln.Close()
	}
	s.mu.mu.Unlock()
}

// attachCard builds a room card for a fake coachee served by s, with a temp log
// the test writes a looping stream into.
func attachCard(t *testing.T, s *ctlSockServer, events bool) (room.Card, *os.File) {
	t.Helper()
	logF, err := os.CreateTemp("", "coach-attach-log-*.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { logF.Close(); os.Remove(logF.Name()) })
	card := room.Card{
		ID:      "attach-test-" + shortHash(s.path),
		Tool:    "agy",
		Binding: "agy:test-attach",
		Mode:    "weave",
		CtlSock: s.path,
		LogPath: logF.Name(),
		PID:     os.Getpid(), // alive: the test process
		Events:  events,
	}
	return card, logF
}

// appendLines writes the given lines (newline-terminated) to the coachee's log,
// simulating a live agent churning output a tailing coach will read.
func appendLines(t *testing.T, logF *os.File, lines []string) {
	t.Helper()
	var b strings.Builder
	for _, ln := range lines {
		b.WriteString(ln)
		b.WriteByte('\n')
	}
	if _, err := logF.WriteString(b.String()); err != nil {
		t.Fatal(err)
	}
}

// churnLines is a rotating set of significant, distinct-looking lines that drives
// the pty novelty window below its floor (the same shape TestPtyDetectsChurnLoop
// uses) — i.e. a coachee stuck in a loop.
func churnLines(n int) []string {
	acts := []string{
		"run_tests on package ./attachsvc",
		"read_file internal/attachsvc/svc.go",
		"the tests are still failing here",
		"let me check the implementation again",
	}
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, acts[i%len(acts)])
	}
	return out
}

// SUCCESS CRITERIA 2: a coach attached to a fake member with a working control
// socket, fed a looping line stream, must emit a steer THROUGH THAT SOCKET — ESC
// (the verbatim frame) first, then the spoken steer. This is the end-to-end
// proof that attach wires the room member → steerer → coach → socket.
func TestAttachCoachSteersThroughSocket(t *testing.T) {
	isolateRoom(t)
	srv := newCtlSockServer(t)
	card, logF := attachCard(t, srv, false /* events: pty path */)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	coach, err := attachCoach(ctx, card, DefaultCoachPolicy(), false)
	if err != nil {
		t.Fatalf("attachCoach: %v", err)
	}
	defer coach.Wait()

	// Feed the coachee's loop AFTER attaching: the watcher's file offset is at EOF,
	// so it picks these appends up on its next poll, just like a live run.
	appendLines(t, logF, churnLines(80))

	// The steer is async across a 500ms-tailed read + a socket dial. Poll for it.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		got := srv.mu.get()
		if bytes.Contains(got, []byte("\x00R")) && bytes.Contains(got, []byte("STOP investigating")) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	got := srv.mu.get()
	if !bytes.Contains(got, []byte("\x00R")) {
		t.Fatalf("ESC frame never crossed the control socket; coachee would not be broken out of its loop. got=%q", got)
	}
	if !bytes.Contains(got, []byte("STOP investigating")) {
		t.Errorf("the spoken steer never crossed the control socket. got=%q", got)
	}
	if c := countEsc(got); c == 0 {
		t.Errorf("expected at least one ESC, got %d", c)
	}
	cancel() // detach
}

// SUCCESS CRITERIA 3a: attach REFUSES a member with an empty control socket —
// there is nothing to steer through, so it must not pretend to coach.
func TestAttachCoachRefusesEmptyCtlSock(t *testing.T) {
	isolateRoom(t)
	card := room.Card{ID: "no-sock", Binding: "agy:nosock", PID: os.Getpid(), LogPath: "/dev/null"}
	_, err := attachCoach(context.Background(), card, DefaultCoachPolicy(), false)
	if err == nil {
		t.Fatal("attach must refuse a member with no control socket")
	}
	if !strings.Contains(err.Error(), "control socket") {
		t.Errorf("refusal reason = %q, want it to name the missing control socket", err)
	}
}

// SUCCESS CRITERIA 3b: attach REFUSES a member whose pid is gone — a dead
// session. (The room prunes dead cards on read, but attach checks liveness
// itself so it never coaches a card a race just left behind.)
func TestAttachCoachRefusesDeadSession(t *testing.T) {
	isolateRoom(t)
	srv := newCtlSockServer(t)
	card, _ := attachCard(t, srv, false)
	card.PID = 999999 // not running
	_, err := attachCoach(context.Background(), card, DefaultCoachPolicy(), false)
	if err == nil {
		t.Fatal("attach must refuse a dead session (pid gone)")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("refusal reason = %q, want it to say the session is not running", err)
	}
}

// SUCCESS CRITERIA 3b (the resolver side): the SAME resolver `chat steer` uses
// (findMember) reports an unknown session rather than guessing. attach routes its
// target through that resolver, so it inherits this refusal for free.
func TestAttachCoachResolverRefusesUnknownSession(t *testing.T) {
	isolateRoom(t) // empty room — nothing matches
	if _, err := findMember("no-such-session"); err == nil {
		t.Fatal("findMember must refuse an unknown session id rather than guess")
	}
}

// SUCCESS CRITERIA 3 (the discoverable prefix): a member resolves by an
// unambiguous id prefix, mirroring `chat steer` — so an operator types less.
func TestAttachResolvesUnambiguousPrefix(t *testing.T) {
	isolateRoom(t)
	srv := newCtlSockServer(t)
	card, _ := attachCard(t, srv, false)
	if err := room.Join(card); err != nil {
		t.Fatal(err)
	}
	defer room.Leave(card.ID)

	// Resolve on a short unambiguous prefix of the id.
	got, ok, err := room.Find(card.ID[:12])
	if err != nil || !ok {
		t.Fatalf("room.Find(prefix) = %v,%v,%v; want the member resolved", got, ok, err)
	}
	if got.ID != card.ID {
		t.Errorf("resolved id = %q, want %q", got.ID, card.ID)
	}
}

// SUCCESS CRITERIA 4: detach leaves the coachee running. Cancelling the watch
// context (Ctrl-C / --timeout) ends the watcher; the coachee is neither killed
// nor pelted with an ESC storm as attach exits.
func TestAttachCoachDetachLeavesCoacheeRunning(t *testing.T) {
	isolateRoom(t)
	srv := newCtlSockServer(t)
	card, logF := attachCard(t, srv, false)

	pol := DefaultCoachPolicy()
	ctx, cancel := context.WithCancel(context.Background())

	coach, err := attachCoach(ctx, card, pol, false)
	if err != nil {
		t.Fatalf("attachCoach: %v", err)
	}

	appendLines(t, logF, churnLines(120))

	// Let the coach read the loop and steer a bounded number of times.
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if countEsc(srv.mu.get()) >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()     // detach (Ctrl-C / --timeout)
	coach.Wait() // watcher drains and exits

	// A frame the watcher sent just before exiting may still be in flight: the
	// sender has closed its side of the socket, but the server's reader goroutine
	// has not necessarily ReadAll'd+recorded it yet. settle() waits for the byte
	// count to stop growing, so atDetach is the true post-detach baseline rather
	// than a snapshot that a draining connection is about to bump.
	atDetach := settle(srv)
	escAtDetach := countEsc(atDetach)

	// Hold the watch dead for a while and assert NOTHING new crosses the socket —
	// no ESC storm on exit.
	time.Sleep(700 * time.Millisecond)
	after := settle(srv)
	if len(after) != len(atDetach) {
		t.Errorf("ESC storm on detach: %d bytes at detach, %d after — detach must not pelt the coachee", len(atDetach), len(after))
	}
	if countEsc(after) != escAtDetach {
		t.Errorf("ESC count rose after detach: %d -> %d — detach must leave the coachee alone", escAtDetach, countEsc(after))
	}
	// The coachee's control socket still accepts connections: attach did not kill
	// it. (attach never owns the coachee process.)
	if !srv.dialAble() {
		t.Errorf("coachee's control socket is gone after detach — attach must leave the coachee running")
	}
	// And the steers never exceeded the cap (no storm while running either).
	if max := countEsc(after); max > pol.MaxSteers {
		t.Errorf("ESC count %d exceeds MaxSteers %d — the cap must bound interventions", max, pol.MaxSteers)
	}
}

// --read-only is a watch with NO intervention: the coach sees the loop (it trips
// internally — the report records it) but sends NOTHING across the control
// socket. The invariant is that a coach is a report channel; read-only drops even
// ESC+Say, for an operator who wants to observe with zero chance of disturbance.
func TestAttachCoachReadOnlySendsNothing(t *testing.T) {
	isolateRoom(t)
	srv := newCtlSockServer(t)
	card, logF := attachCard(t, srv, false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	coach, err := attachCoach(ctx, card, DefaultCoachPolicy(), true /* read-only */)
	if err != nil {
		t.Fatalf("attachCoach: %v", err)
	}

	appendLines(t, logF, churnLines(120))

	// Give the watcher plenty of time to read the loop and (internally) trip.
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		// read-only must send nothing, so the only exit is the cap being hit
		// internally OR the deadline; either way the socket stays empty.
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	coach.Wait()

	got := srv.mu.get()
	if len(got) != 0 {
		t.Errorf("read-only attach sent %d bytes across the control socket — it must watch without intervening: %q", len(got), got)
	}
	// But the coach DID watch: it recorded the loop as a trip in its report.
	rep := coach.Report()
	if len(rep.Steers) == 0 {
		t.Errorf("read-only attach recorded no trips — it must still WATCH (the report is the point); total=%d distinct=%d", rep.Total, rep.Distinct)
	}
	if rep.Total == 0 {
		t.Errorf("read-only attach saw no output lines — the watcher did not tail the log")
	}
}

// The structured-event path reconstructs the events file from the card's
// binding+pid — the exact key sessionEventsPath used when the member joined. A
// session-launched card (PID = os.Getpid() of the launcher, here the test) must
// resolve to the SAME path sessionEventsPath would produce, so the precise event
// signal is reachable from a card without widening the card shape.
func TestCardEventsFileMatchesSessionConvention(t *testing.T) {
	binding := "ycode:glm-5.2"
	pid := os.Getpid()
	card := room.Card{Binding: binding, PID: pid, Events: true}
	got := cardEventsFile(card)
	// sessionEventsPath keys on binding + os.Getpid(); with the card's pid set to
	// the current pid, the two must agree, so the precise event signal is reachable
	// from a card without widening the card shape.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".bashy", "events", shortHash(binding+"\x00"+strconv.Itoa(pid))+".ndjson")
	if got != want {
		t.Errorf("cardEventsFile = %q\nwant sessionEventsPath-style %q", got, want)
	}
	if !strings.HasSuffix(got, ".ndjson") {
		t.Errorf("cardEventsFile = %q, want an .ndjson path", got)
	}
}
