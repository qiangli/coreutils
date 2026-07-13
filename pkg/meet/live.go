package meet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Watching a meeting should feel like sitting in one, not like reading the
// minutes afterwards. A turn is a whole model call — often minutes long — so a
// transcript that only gains an entry when an agent FINISHES leaves a watcher
// staring at nothing and wondering whether anything is alive.
//
// So the meeting also writes a LIVE channel: each line of an agent's answer, as
// the agent writes it. `observe` tails it, and you watch the argument being made
// rather than being told, later, that it was.
//
// Two channels, one truth:
//
//   - transcript.jsonl is the RECORD. One sanitized event per completed turn.
//     It is what the minutes are built from and what gets replayed as prompt
//     context to the next agent. Interleaving thousands of partial lines into it
//     would bloat it and quietly change what "the record" means.
//   - live.jsonl is the VIEW. Ephemeral, derived, line-granular, and safe to
//     lose: delete it and the meeting is unharmed, because every line it carried
//     also lands, whole, in the transcript when the turn completes.
//
// The live channel is a tee of the agent's stdout (see chat.Options.Stream), so
// it cannot show a watcher anything the record will not also contain. Observing
// never changes what is recorded.
//
// Granularity is a LINE, not a token. The agent CLIs bashy drives emit complete
// lines on stdout; there is no token channel to subscribe to without going
// around them straight to a provider API, which would abandon the harness (and
// its tools, its sandbox, its shell-forcing) entirely. A line is the finest
// grain honestly available here.

// liveKind values, kept distinct from transcript Event kinds so a reader can
// never mistake a view record for a record record.
const (
	liveSpeaking = "speaking" // an agent took the floor
	liveLine     = "line"     // one line of what it is saying
	liveSpoke    = "spoke"    // it finished; the whole turn is now in the transcript
)

// LiveEvent is one line of the live channel.
type LiveEvent struct {
	Kind    string    `json:"kind"`
	Round   int       `json:"round"`
	Speaker string    `json:"speaker"`
	Role    string    `json:"role,omitempty"`
	Text    string    `json:"text,omitempty"`
	Status  string    `json:"status,omitempty"` // on `spoke`
	TS      time.Time `json:"ts"`

	// CtlSock is set on `speaking`: the socket that steers THIS turn.
	//
	// It rides the live channel because the live channel already answers "who has
	// the floor right now" — a `speaking` with no matching `spoke` IS the current
	// speaker. Keeping the steer address in the same record means `meet say`
	// cannot address an agent that has already finished, which is the one way a
	// steer goes quietly nowhere.
	CtlSock string `json:"ctl_sock,omitempty"`
}

func livePath(id string) (string, error) {
	dir, err := storeDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "live.jsonl"), nil
}

func appendLive(id string, e LiveEvent) {
	// Best-effort by design. The live channel is a view: if it cannot be written
	// the meeting must still run and still be recorded. A watcher's convenience
	// is never a reason to fail a turn.
	path, err := livePath(id)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

// liveWriter turns an agent's stdout into line events on the live channel.
//
// It is handed to chat as an io.Writer and receives whatever chunks the process
// happens to flush — which do NOT align with lines. So it buffers, emits only
// complete lines, and holds the trailing partial one until it is finished. A
// half-line published as a line would show a watcher a sentence the agent never
// wrote.
type liveWriter struct {
	id      string
	round   int
	speaker string
	role    string

	mu  sync.Mutex
	buf bytes.Buffer
}

func newLiveWriter(st *State, speaker, role, ctlSock string) *liveWriter {
	w := &liveWriter{id: st.ID, round: st.Round, speaker: speaker, role: role}
	appendLive(w.id, LiveEvent{
		Kind: liveSpeaking, Round: w.round, Speaker: speaker, Role: role,
		CtlSock: ctlSock, TS: nowFn(),
	})
	return w
}

// ctlSockPath is where this turn's steer socket lives.
//
// Deliberately SHORT, and NOT under the meeting's own directory. A unix socket
// address is capped at ~104 bytes, and a meeting id is a slug of its topic —
// `~/.bashy/meet/2026-07-13-should-the-cache-be-write-through-0e1e/…` blows the
// cap and the bind fails. agentpty degrades to a polling file channel when that
// happens, so steering would still work, just silently worse. Hashing keeps the
// address inside the limit for any topic anyone can type.
func ctlSockPath(st *State, speaker string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".bashy", "ctl")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00%d\x00%s", st.ID, st.Round, speaker))
	return filepath.Join(dir, hex.EncodeToString(sum[:])[:12]+".sock")
}

func (w *liveWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			// Not a whole line yet — put it back and wait for the rest.
			w.buf.Reset()
			w.buf.WriteString(line)
			break
		}
		w.emit(line)
	}
	// Always report the full write: the tee must never make the agent's own
	// stdout write appear to fail.
	return len(p), nil
}

// emit publishes one line, sanitized the same way the recorded turn will be.
//
// Sanitizing here is not cosmetic. The raw stream carries ANSI escapes and
// control bytes; shown verbatim they would garble the watcher's terminal, and —
// worse — the watcher would see something different from what the transcript
// ends up storing. The view must agree with the record.
func (w *liveWriter) emit(line string) {
	text := strings.TrimRight(sanitizeTurn(line), "\n")
	if strings.TrimSpace(text) == "" {
		return
	}
	appendLive(w.id, LiveEvent{
		Kind: liveLine, Round: w.round, Speaker: w.speaker, Role: w.role,
		Text: text, TS: nowFn(),
	})
}

// close flushes a trailing line with no newline and marks the floor free.
func (w *liveWriter) close(status string) {
	w.mu.Lock()
	if rest := w.buf.String(); rest != "" {
		w.buf.Reset()
		w.emit(rest)
	}
	w.mu.Unlock()
	appendLive(w.id, LiveEvent{
		Kind: liveSpoke, Round: w.round, Speaker: w.speaker, Role: w.role,
		Status: status, TS: nowFn(),
	})
}
