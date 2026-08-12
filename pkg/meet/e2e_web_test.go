package meet

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/qiangli/coreutils/pkg/chat"
)

// End-to-end over the path a browser actually takes: HTTP in, WebSocket out.
//
// serve_test.go covers the surface's contract — status codes, authorization,
// identity seating, the lease's 409 — and says why it stops short:
//
//	"The routes that run agents (round, poll, ask, address, converge) are
//	 exercised only as far as their 202/409 contract: actually running one would
//	 spawn a real CLI, which is the one thing a hermetic suite must never do."
//
// That left the MVP's whole point untested: a human addresses an agent and the
// reply comes back. The 202 proves the request was accepted, which is exactly
// the kind of evidence dhnt/docs/fleet-evidence-invariant.md forbids treating as
// success — the fleet A/B's headline is that all three harnesses exited 0 when
// they failed. So these tests assert on the OBSERVABLE result: the frame that
// reaches the browser, and the transcript the room keeps.
//
// apiRunner (api.go) is what makes that hermetic. Nothing else is substituted:
// the same mux, lease, engine, transcript and live tee run as in production.

// withFakeAgent points the exported verbs at a canned runner for one test.
func withFakeAgent(t *testing.T, reply string) {
	t.Helper()
	old := apiRunner
	apiRunner = func() chat.Runner { return fakeRunner{reply: reply} }
	t.Cleanup(func() { apiRunner = old })
}

// drainHistory reads the opening info frame and the backlog, leaving the
// connection positioned on whatever happens NEXT — which is what a browser that
// has finished painting is looking at.
func drainHistory(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if readFrame(t, conn).Kind == "history-end" {
			return
		}
	}
	t.Fatal("no history-end marker in the first 200 frames")
}

// awaitEvent reads until an `event` frame satisfies want, and fails if the
// stream goes quiet first. Returning the event (not a bool) lets a caller assert
// on the speaker as well as the text.
func awaitEvent(t *testing.T, conn *websocket.Conn, what string, want func(Event) bool) Event {
	t.Helper()
	for i := 0; i < 200; i++ {
		f := readFrame(t, conn)
		if f.Kind != "event" {
			continue // live chunks, notes, markers — not the recorded turn
		}
		var ev Event
		if err := json.Unmarshal(f.Data, &ev); err != nil {
			t.Fatalf("an event frame must carry an Event: %v (%s)", err, f.Data)
		}
		if want(ev) {
			return ev
		}
	}
	t.Fatalf("no event frame matching %s arrived", what)
	return Event{}
}

// address is the browser's move: POST the verb, expect 202 and a job ref.
func address(t *testing.T, srv *httptest.Server, room, agent, text string) JobRef {
	t.Helper()
	resp, body := doJSON(t, srv, "POST", "/api/rooms/"+room+"/address",
		map[string]string{"agent": agent, "text": text})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("address %s: status %d, want 202 (%s)", agent, resp.StatusCode, body)
	}
	var job JobRef
	if err := json.Unmarshal(body, &job); err != nil {
		t.Fatalf("a 202 must carry a JobRef: %v (%s)", err, body)
	}
	if job.ID == "" || job.Room != room {
		t.Fatalf("JobRef = %+v, want an id and room %s", job, room)
	}
	return job
}

// waitForTurns blocks until the transcript holds n turns by speaker. The verbs
// are asynchronous by design (handleAsync answers 202 and works in a goroutine),
// so a test that read the transcript immediately would be racing the room rather
// than testing it.
func waitForTurns(t *testing.T, id, speaker string, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(turnsBy(t, id, speaker)) >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d turn(s) by %s (have %d)", n, speaker, len(turnsBy(t, id, speaker)))
}

func turnsBy(t *testing.T, id, speaker string) []Event {
	t.Helper()
	events, err := readTranscript(id)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	var out []Event
	for _, e := range events {
		if e.Kind == "turn" && e.Speaker == speaker {
			out = append(out, e)
		}
	}
	return out
}

// E1 — the MVP's core loop. A human addresses one agent and the reply reaches
// the browser on the live stream, then survives in the transcript.
//
// The two assertions are deliberately separate. A reply that is recorded but
// never streamed leaves the tab silent until a reload; one that is streamed but
// never recorded vanishes on reload. Both have to hold.
func TestE2EAddressAnAgentAndTheReplyReachesTheBrowser(t *testing.T) {
	st := newRoom(t) // Participants: ["codex"]
	seatEverything(t)
	withFakeAgent(t, "on it")
	srv := serveTest(t)

	conn := dialObserve(t, srv, st.ID)
	defer conn.Close()
	drainHistory(t, conn)

	address(t, srv, st.ID, "codex", "status?")

	ev := awaitEvent(t, conn, `codex's turn`, func(e Event) bool {
		return e.Kind == "turn" && e.Speaker == "codex"
	})
	if !strings.Contains(ev.Text, "on it") {
		t.Errorf("streamed turn = %q, want the agent's reply", ev.Text)
	}

	if got := turnsBy(t, st.ID, "codex"); len(got) != 1 {
		t.Fatalf("transcript has %d codex turns, want 1 — a streamed reply that is not recorded is lost on reload", len(got))
	} else if !strings.Contains(got[0].Text, "on it") {
		t.Errorf("recorded turn = %q, want the agent's reply", got[0].Text)
	}
}

// E1 (N-party) — the room grows mid-conversation and BOTH agents answer.
//
// This is the "1:1 or meet with N agents" requirement in one test: the room
// starts as a human plus one assistant and becomes a meeting without being
// recreated, which is the design's one-room-type claim.
func TestE2EInviteASecondAgentMidConversationAndBothReply(t *testing.T) {
	st := newRoom(t)
	seatEverything(t)
	withFakeAgent(t, "ack")
	srv := serveTest(t)

	address(t, srv, st.ID, "codex", "first")
	waitForTurns(t, st.ID, "codex", 1)

	resp, body := doJSON(t, srv, "POST", "/api/rooms/"+st.ID+"/invite",
		map[string]string{"agent": "claude"})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("invite: status %d, want 204 (%s)", resp.StatusCode, body)
	}

	address(t, srv, st.ID, "claude", "second")
	waitForTurns(t, st.ID, "claude", 1)

	// The roster the browser reads must show both, or the composer cannot offer
	// the agent that just spoke.
	resp, body = doJSON(t, srv, "GET", "/api/rooms/"+st.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("room get: status %d (%s)", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "codex") || !strings.Contains(string(body), "claude") {
		t.Errorf("room must list both agents after the invite: %s", body)
	}
}

// E3 — parity. serve.go claims the HTTP surface and the CLI "cannot drift"
// because they share machinery. That is an argument; this is the evidence.
//
// The same exchange driven through the API function the cobra verb calls and
// through the HTTP route must produce the same transcript shape — same kinds,
// same speakers, same text.
func TestE2EWebAndCLIProduceTheSameTranscript(t *testing.T) {
	st := newRoom(t)
	seatEverything(t)
	withFakeAgent(t, "same answer")
	srv := serveTest(t)

	// The CLI path: the exported verb, called directly.
	if _, err := Address(t.Context(), st.ID, "codex", "over the CLI"); err != nil {
		t.Fatalf("Address: %v", err)
	}
	viaCLI := turnsBy(t, st.ID, "codex")

	// The browser path: same verb, over HTTP.
	address(t, srv, st.ID, "codex", "over the web")
	waitForTurns(t, st.ID, "codex", len(viaCLI)+1)

	all := turnsBy(t, st.ID, "codex")
	web := all[len(all)-1]
	cli := viaCLI[len(viaCLI)-1]

	if web.Kind != cli.Kind || web.Speaker != cli.Speaker {
		t.Errorf("web turn {%s,%s} != cli turn {%s,%s}", web.Kind, web.Speaker, cli.Kind, cli.Speaker)
	}
	if web.Text != cli.Text {
		t.Errorf("the two surfaces recorded different text:\n  cli: %q\n  web: %q", cli.Text, web.Text)
	}
}

// A turn that FAILS must say so in the room. startJob's comment is explicit that
// "a 202 followed by silence would leave the room believing a round is still
// coming" — this is the test that keeps that true.
func TestE2EAFailedTurnIsRecordedNotSilent(t *testing.T) {
	st := newRoom(t)
	seatEverything(t)
	old := apiRunner
	apiRunner = func() chat.Runner { return errRunner{} }
	t.Cleanup(func() { apiRunner = old })
	srv := serveTest(t)

	conn := dialObserve(t, srv, st.ID)
	defer conn.Close()
	drainHistory(t, conn)

	address(t, srv, st.ID, "codex", "this will fail")

	ev := awaitEvent(t, conn, "a failure the room can see", func(e Event) bool {
		if e.Kind == "note" {
			return true
		}
		// A turn recorded with a non-zero status is an equally honest report.
		return e.Kind == "turn" && e.Status != "" && e.Status != statusOK
	})
	if strings.TrimSpace(ev.Text) == "" {
		t.Error("a failed turn must leave a message a human can read, not an empty marker")
	}
}

// errRunner is an agent that cannot run — the crash/not-installed case.
type errRunner struct{}

func (errRunner) Run(_ context.Context, _ string, _ []string, _ string) (string, int, error) {
	return "", 127, errors.New("meet_test: the agent could not be started")
}
