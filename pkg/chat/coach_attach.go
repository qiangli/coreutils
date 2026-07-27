package chat

import (
	"context"
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
	// Falls back to pty when the file is not reachable (a card that claims events
	// but whose file is gone, e.g. after a host restart).
	if card.Events {
		if evPath := cardEventsFile(card); evPath != "" {
			if _, err := os.Stat(evPath); err == nil {
				c := newCoach(pol)
				c.steer = steer
				c.agent = card.Binding
				c.mode = "events"
				tail := &eventTail{path: evPath}
				if evs, err := tail.drain(); err == nil {
					c.recordPriorToolCalls(evs)
				}
				tail.skipToEnd()
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
	log, err := os.Open(card.LogPath)
	if err == nil {
		// Fix the attachment boundary before the watcher starts. Preserve bytes
		// already present as report-only history, then return to that exact
		// boundary so appends racing with setup are still observed live.
		var end int64
		end, err = skipToEnd(log)
		if err == nil && end > 0 {
			if _, err = log.Seek(0, io.SeekStart); err == nil {
				var prior []byte
				prior, err = io.ReadAll(io.LimitReader(log, end))
				if err == nil {
					coach.recordPriorPty(prior)
				}
			}
		}
		if err == nil {
			_, err = log.Seek(end, io.SeekStart)
		}
		if err != nil {
			_ = log.Close()
			log = nil
		}
	}
	go func() {
		defer func() { close(coach.done) }()
		watchAttachedLog(ctx, coach, card, log)
	}()
	return coach, nil
}

// watchAttachedLog tails card.LogPath and pumps every new byte into the coach's
// Write (which normalizes lines, runs the signal panel, and intervenes through
// the shared (*Coach).intervene). It ends when the coachee's pid is gone or ctx
// is cancelled — and in both cases it leaves the coachee alone (no kill, no ESC).
func watchAttachedLog(ctx context.Context, coach *Coach, card room.Card, f *os.File) {
	if f == nil {
		return // nothing to tail; the coach simply has no signal
	}
	defer f.Close()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	buf := make([]byte, 4096)
	for {
		// Drain whatever has been appended since the last poll. A regular file
		// keeps its offset, so a read that hit EOF picks up new appends on the
		// next tick — the same follow loop cmd_interact.go's attach uses.
		for {
			nr, rerr := f.Read(buf)
			if nr > 0 {
				_, _ = coach.Write(buf[:nr])
			}
			if rerr != nil || nr == 0 {
				break
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
func watchAttachedEvents(ctx context.Context, coach *Coach, tail *eventTail, pid int) {
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		if evs, err := tail.drain(); err == nil {
			for _, ev := range evs {
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

// skipToEnd establishes the common attachment boundary for append-only files:
// bytes after the returned offset are live output; bytes before it are history.
func skipToEnd(f *os.File) (int64, error) {
	return f.Seek(0, io.SeekEnd)
}

// eventTail.skipToEnd mirrors pkg/meet's lineTail discipline using the same
// attachment-boundary helper as the pty capture tail.
func (e *eventTail) skipToEnd() {
	f, err := os.Open(e.path)
	if err != nil {
		return
	}
	defer f.Close()
	if end, err := skipToEnd(f); err == nil {
		e.offset = end
	}
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
