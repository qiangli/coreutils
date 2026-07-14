package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// A REAL TURN BOUNDARY.
//
// Everywhere else in this package, bashy decides a turn has ended by watching for
// SILENCE — 25 seconds of no output (Session.WaitIdle). That is a heuristic and it
// is wrong in both directions: an agent that pauses to think looks finished, and
// an agent that renders a spinner never does. It also costs 25 seconds on every
// single turn, which is the reason `meet --steerable` is a flag rather than the
// default.
//
// A tool that declares `events_arg:` gets to just SAY when it is done, and bashy
// believes it — because that is a fact the agent reported, not a silence bashy
// interpreted. Today exactly one tool can: ycode, the first-party harness. That
// is the whole point of having one.
//
// The events are NDJSON, one object per line:
//
//	{"type":"turn.start","data":{"prompt":"..."}}
//	{"type":"tool.call","data":{"name":"read_file","input":{...}}}
//	{"type":"turn.end","data":{"status":"ok","text":"..."}}
//
// tool.call is the other half of the prize, and the one the fleet-evidence rule
// has been asking for since the beginning: a tool call as STRUCTURED DATA, rather
// than a line scraped back out of a terminal and guessed at.

// Event is one line of a tool's event channel.
type Event struct {
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

const (
	EventTurnStart = "turn.start"
	EventToolCall  = "tool.call"
	EventTurnEnd   = "turn.end"
)

// turnEndData is the payload we care about on turn.end.
type turnEndData struct {
	Status string `json:"status"`
	Text   string `json:"text"`
}

// eventTail follows a tool's NDJSON event file and reports turns as they end.
//
// It tails rather than reads-once because the file is being written by a live
// process: the turn we are waiting for has not happened yet when we start
// watching.
type eventTail struct {
	path   string
	offset int64
}

// WaitTurnEnd blocks until the tool reports that the turn is over, and returns
// the answer it reported.
//
// It returns `ok=false` if the channel produced no turn.end before ctx ended —
// which the caller must treat as "I do not know", NOT as "the turn finished".
// Falling back to a silence heuristic there is correct; pretending the turn ended
// is not.
func (e *eventTail) WaitTurnEnd(ctx context.Context) (text string, ok bool, err error) {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		evs, rerr := e.drain()
		if rerr != nil && !os.IsNotExist(rerr) {
			return "", false, rerr
		}
		for _, ev := range evs {
			if ev.Type != EventTurnEnd {
				continue
			}
			var d turnEndData
			if err := json.Unmarshal(ev.Data, &d); err != nil {
				// A turn.end we cannot parse is not a turn.end. Say so rather than
				// silently treating a malformed line as a completed turn.
				return "", false, fmt.Errorf("chat: malformed turn.end on the event channel: %w", err)
			}
			return d.Text, true, nil
		}
		select {
		case <-ctx.Done():
			return "", false, ctx.Err()
		case <-tick.C:
		}
	}
}

// drain reads whatever has been appended since the last call.
//
// It consumes only COMPLETE lines. The file is being appended to by a live
// process, so the last line is routinely half-written — and bufio.Scanner hands
// that fragment back as if it were a token, with no way to tell it apart from a
// finished one. Trusting it meant advancing past the torn line and LOSING the
// event when it was finally written in full. (A test caught this. The obvious
// implementation is wrong.)
//
// So: the offset stops at the last newline. A partial line stays unread until it
// has an ending.
func (e *eventTail) drain() ([]Event, error) {
	f, err := os.Open(e.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.Seek(e.offset, io.SeekStart); err != nil {
		return nil, err
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	// Everything up to and including the final newline is complete; the remainder
	// is a line still being written.
	nl := bytes.LastIndexByte(buf, '\n')
	if nl < 0 {
		return nil, nil // nothing whole to read yet
	}
	complete := buf[:nl+1]
	e.offset += int64(len(complete))

	var out []Event
	for _, line := range bytes.Split(complete, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			// A complete line that is not JSON is a broken contract, not a torn read.
			// Skip it rather than abort the turn, but do not pretend it was an event.
			continue
		}
		out = append(out, ev)
	}
	return out, nil
}
