package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/qiangli/coreutils/pkg/room"
)

// ATTACH — point a coach at a session that is ALREADY RUNNING.
//
// `coach --agent A -m TASK` LAUNCHES the agent and watches it. There was no way
// to attach to a session already in flight, and that gap has a cost: an agy
// worker running under weave executed a completely unrelated task from stale
// session state, never touched its assigned issue, and reported success — exit
// 0, zero commits. A supervisor could have caught it in the first minute.
// Nothing could be attached, so nobody did.
//
// This is WIRING, not new machinery. The three primitives already existed:
// NewCtlSteerer (steer through a control socket), NewLineCoach (a coach fed any
// line stream) and feedPty/tripPty (pty-scrape loop detection). What was missing
// was the composition: resolving a live room member into a coach.
//
// THE INVARIANT (docs/live-agent-coaching-design.md): a coach is a REPORT
// CHANNEL, NEVER AN AUTHOR. It may press ESC and say a sentence; it may NOT
// write files, commit, or merge. Attach does not widen that authority — it is
// ESC + Say, exactly as a launched coach. --read-only makes even that a
// detect-only report (no intervention at all).
//
// ESC FIRST, THEN SPEAK: every agent TUI queues a Say and reads it only between
// turns, so an agent stuck in a tool loop never reaches the moment it would read
// it. ctlSteerer already interrupts before speaking; the shared intervene()
// preserves that ordering.
//
// DETACH IS CLEAN: attach never owns the coachee. --timeout or Ctrl-C cancels
// the watch; the watcher goroutine exits; the coachee keeps running. Attach
// never kills, never sends an ESC storm on exit.

// attachCoach resolves a live room member and runs a coach over it, returning the
// coach (so a caller can Wait/Report) and starting a watcher goroutine that ends
// when ctx does. The card must be live and steerable; this function refuses a
// card with no control socket or a dead pid.
//
// readOnly makes the coach DETECT-ONLY: it watches and records trips but its
// steerer is a no-op, so nothing is sent to the coachee. A coach is already a
// report channel, never an author; read-only drops even ESC+Say, for an operator
// who wants to observe with zero chance of intervention. The control-socket
// requirement still applies — a member that cannot be steered is not
// "attachable", even for observation.
//
// It runs the SAME trip implementation a launched coach uses: pty mode goes
// through NewLineCoach → Write → feedPty → tripPty → (*Coach).intervene (the
// exact path watchPty drives), and event mode goes through onToolCall → decide →
// (*Coach).intervene. Both modes funnel every steer through (*Coach).intervene —
// that one function is the shared trip delivery for launched and attached coaches
// alike.
//
// detachSafe by construction: the watcher only tails the member's output/events
// and (at most) writes frames to its control socket; cancelling ctx ends the
// watch and leaves the coachee untouched.
func attachCoach(ctx context.Context, card room.Card, pol CoachPolicy, readOnly bool) (*Coach, error) {
	if card.CtlSock == "" {
		return nil, fmt.Errorf("coach attach: %s has no control socket — nothing to steer through (it is not a steerable member)", card.ID)
	}
	if !room.PidAlive(card.PID) {
		return nil, fmt.Errorf("coach attach: %s is not running (pid %d is gone)", card.ID, card.PID)
	}

	// Build the steerer BEFORE starting the watcher, so there is no race between a
	// read-only swap and intervene() reading c.steer from the watch goroutine.
	// read-only uses a no-op steerer (NewCtlSteerer("") sends nothing); the real
	// path steers through the member's control socket.
	var steer Steerer = NewCtlSteerer(card.CtlSock)
	if readOnly {
		steer = NewCtlSteerer("")
	}
	if pol.Steer == "" {
		pol.Steer = DefaultCoachPolicy().Steer
	}

	// Event path: the member speaks a structured event channel (a first-party
	// harness reporting tool.call as data). Tail its events file and feed
	// tool.call through decide — the PRECISE signal, the same one watchEvents
	// drives for a launched coach. The events file is not carried on the card
	// (the card shape is read-only here), so it is reconstructed from the card's
	// binding+pid — the exact key sessionEventsPath used when the member joined.
	// Falls back to pty when the file is not reachable, but only if that fallback
	// itself can be opened: attach must never succeed without a signal source.
	if card.Events {
		if evPath := cardEventsFile(card); evPath != "" {
			tail := &attachLineTail{path: evPath}
			if lines, err := tail.skipToEnd(); err == nil {
				c := newCoach(pol)
				c.steer = steer
				c.agent = card.Binding
				c.mode = "events"
				c.recordPriorToolCalls(eventsFromLines(lines))
				go func() {
					defer func() { close(c.done) }()
					watchAttachedEvents(ctx, c, tail, card.PID)
				}()
				return c, nil
			}
		}
	}

	// Pty path (and the events fallback): tail the member's capture and feed
	// NewLineCoach's Write — the GENERIC "output flowing, distinct content not
	// growing" signal that works for every tool (agy and any third-party CLI).
	// This is the same feedPty/tripPty/intervene path the launched pty-mode coach
	// drives through watchPty.
	if card.LogPath == "" {
		return nil, fmt.Errorf("coach attach: %s has no log to tail and no reachable event channel — nothing to watch", card.ID)
	}
	coach := NewLineCoach(pol, steer)
	coach.agent = card.Binding
	tail := &attachLineTail{path: card.LogPath}
	prior, err := tail.skipToEnd()
	if err != nil {
		return nil, fmt.Errorf("coach attach: %s cannot open log %q — nothing to watch: %w", card.ID, card.LogPath, err)
	}
	coach.recordPriorPty(completeLines(prior))
	go func() {
		defer func() { close(coach.done) }()
		watchAttachedLog(ctx, coach, card, tail)
	}()
	return coach, nil
}

// watchAttachedLog tails card.LogPath and pumps every new byte into the coach's
// Write (which normalizes lines, runs the signal panel, and intervenes through
// the shared (*Coach).intervene). It ends when the coachee's pid is gone or ctx
// is cancelled — and in both cases it leaves the coachee alone (no kill, no ESC).
func watchAttachedLog(ctx context.Context, coach *Coach, card room.Card, tail *attachLineTail) {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		// A path-based tail can follow truncation and rotation. Errors are
		// transient while a writer replaces a file, so keep polling; an attach
		// whose source is absent from the start is rejected synchronously.
		if lines, err := tail.next(); err == nil {
			for _, line := range lines {
				_, _ = coach.Write(append(append([]byte{}, line...), '\n'))
			}
		}
		if !room.PidAlive(card.PID) {
			return // coachee ended on its own — do not disturb it
		}
		select {
		case <-ctx.Done():
			return // detach: leave the coachee running
		case <-tick.C:
		}
	}
}

// watchAttachedEvents tails a reconstructed NDJSON events file from the point
// of attachment and feeds each new tool.call through onToolCall → decide →
// intervene — the precise event path, identical to a launched coach's
// watchEvents. Calls already present remain reportable session history, but
// never seed the live detector or cause an intervention.
func watchAttachedEvents(ctx context.Context, coach *Coach, tail *attachLineTail, pid int) {
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		if lines, err := tail.next(); err == nil {
			for _, ev := range eventsFromLines(lines) {
				if ev.Type == EventToolCall {
					coach.onToolCall(ev)
				}
			}
		}
		if !room.PidAlive(pid) {
			return // coachee ended on its own — do not disturb it
		}
		select {
		case <-ctx.Done():
			return // detach: leave the coachee running
		case <-tick.C:
		}
	}
}

// attachLineTail mirrors pkg/meet's lineTail discipline for both attach signal
// paths. It reads only complete lines, carries a trailing fragment in rem until
// a later poll completes it, and follows a path across truncation or rotation.
type attachLineTail struct {
	path string
	off  int64
	rem  []byte
	file os.FileInfo
	end  []byte
}

// skipToEnd snapshots the attachment boundary in one open/read. Complete lines
// are returned as report-only history; a trailing incomplete line is retained
// in rem so its future continuation is delivered once, whole.
func (t *attachLineTail) skipToEnd() ([][]byte, error) {
	f, err := os.Open(t.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	t.file = fi
	t.off = int64(len(data))
	t.rememberEnd(data)
	return t.split(data), nil
}

// next returns complete lines appended since the previous call. A replacement
// inode or a shorter same-inode file establishes a fresh stream boundary:
// an unfinished record from the old generation cannot be joined to the new one.
func (t *attachLineTail) next() ([][]byte, error) {
	f, err := os.Open(t.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if t.file == nil || !os.SameFile(t.file, fi) || fi.Size() < t.off || !t.sameEnd(f) {
		t.file = fi
		t.off = 0
		t.rem = nil
		t.end = nil
	}
	if _, err := f.Seek(t.off, io.SeekStart); err != nil {
		return nil, err
	}
	chunk, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	t.off += int64(len(chunk))
	t.rememberEnd(chunk)
	return t.split(chunk), nil
}

func (t *attachLineTail) sameEnd(f *os.File) bool {
	if len(t.end) == 0 {
		return true
	}
	got := make([]byte, len(t.end))
	n, err := f.ReadAt(got, t.off-int64(len(t.end)))
	return err == nil && n == len(got) && bytes.Equal(got, t.end)
}

func (t *attachLineTail) rememberEnd(chunk []byte) {
	const anchor = 64
	data := append(append([]byte{}, t.end...), chunk...)
	if len(data) > anchor {
		data = data[len(data)-anchor:]
	}
	t.end = data
}

func (t *attachLineTail) split(chunk []byte) [][]byte {
	data := append(t.rem, chunk...)
	cut := bytes.LastIndexByte(data, '\n')
	if cut < 0 {
		t.rem = append([]byte{}, data...)
		return nil
	}
	t.rem = append([]byte{}, data[cut+1:]...)
	var lines [][]byte
	for _, line := range bytes.Split(data[:cut], []byte{'\n'}) {
		lines = append(lines, append([]byte{}, line...))
	}
	return lines
}

func eventsFromLines(lines [][]byte) []Event {
	var events []Event
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err == nil {
			events = append(events, ev)
		}
	}
	return events
}

func completeLines(lines [][]byte) []byte {
	if len(lines) == 0 {
		return nil
	}
	return append(bytes.Join(lines, []byte{'\n'}), '\n')
}

// cardEventsFile reconstructs the structured-events file path a member writes, by
// mirroring sessionEventsPath with the card's own pid (a card's PID is the
// session-owner pid — set to os.Getpid() when the member joined). Returns "" when
// the home directory is unavailable. Best-effort: a missing file makes the caller
// fall back to the pty path.
func cardEventsFile(card room.Card) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".bashy", "events")
	return filepath.Join(dir, shortHash(card.Binding+"\x00"+strconv.Itoa(card.PID))+".ndjson")
}
