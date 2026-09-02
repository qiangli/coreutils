package meet

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/lockfile"
)

// DefaultRoomLimit follows the public board's default so agents learn one
// rule for bounded broadcast reads.
const DefaultRoomLimit = bus.DefaultBoardLimit

const maxTranscriptLine = 4 * 1024 * 1024

// Unread returns parsed transcript events this reader has not seen.
//
// Events addressed to the reader, or text that starts with @reader, are the
// directed signals Event carries today, so they are returned in full. Events
// authored by the reader are acknowledged by the caller's high-water mark but
// are not inbound work and are therefore never returned. Everything else is
// room broadcast history, trimmed to limit with older reporting how many events
// were hidden.
func Unread(id, reader string, limit int) (directed, other []Event, older int, err error) {
	directed, other, older, _, err = UnreadThrough(id, reader, limit)
	return directed, other, older, err
}

// UnreadThrough is Unread plus the exact transcript high-water mark represented
// by the returned snapshot. A caller that renders the whole snapshot can pass
// through to MarkSeenThrough afterwards without consuming an event appended
// concurrently between the read and the acknowledgement.
func UnreadThrough(id, reader string, limit int) (directed, other []Event, older int, through int64, err error) {
	dr, or, older, through, err := UnreadRecords(id, reader, limit)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	for _, record := range dr {
		directed = append(directed, record.Event)
	}
	for _, record := range or {
		other = append(other, record.Event)
	}
	return directed, other, older, through, nil
}

// UnreadRecord keeps a transcript event paired with its durable sequence.
type UnreadRecord struct {
	Seq   int64 `json:"seq"`
	Event Event `json:"event"`
}

// UnreadRecords is the cursor-safe, sequence-preserving Meet snapshot used by
// aggregate readers. It never marks the room; acknowledge through the returned
// high-water mark only after every returned record was rendered.
func UnreadRecords(id, reader string, limit int) (directed, other []UnreadRecord, older int, through int64, err error) {
	events, err := readRoomTranscript(id)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	through = int64(len(events))
	at := SeenSeq(id, reader)
	for i, e := range events {
		seq := int64(i + 1)
		if seq <= at {
			continue
		}
		// A room transcript contains both directions. The receive-side cursor
		// must pass the reader's own outbound records without handing them back
		// as inbound work: otherwise an inbox watch wakes on the status message
		// it just sent and can miss the reply it was waiting for.
		if authoredBy(e, reader) {
			continue
		}
		record := UnreadRecord{Seq: seq, Event: e}
		if directedEvent(e, reader) {
			directed = append(directed, record)
			continue
		}
		other = append(other, record)
	}
	if limit > 0 && len(other) > limit {
		older = len(other) - limit
		other = other[len(other)-limit:]
	}
	return directed, other, older, through, nil
}

func authoredBy(e Event, reader string) bool {
	reader = strings.TrimSpace(strings.TrimPrefix(reader, "@"))
	return reader != "" && strings.EqualFold(strings.TrimSpace(e.Speaker), reader)
}

func directedEvent(e Event, reader string) bool {
	reader = strings.TrimSpace(strings.TrimPrefix(reader, "@"))
	if reader == "" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(e.To), reader) {
		return true
	}
	// A seat address is resolved HERE, at read time, and never at write time.
	// Mail addressed to `conductor:99` is directed at whoever holds that lease
	// when it is read, so a handover re-targets it instead of orphaning it
	// against the name of whoever held it when it was sent.
	if holder, ok := bus.RoleHolderFor(strings.TrimSpace(e.To)); ok &&
		strings.EqualFold(strings.TrimSpace(holder), reader) {
		return true
	}
	text := strings.TrimSpace(e.Text)
	if !strings.HasPrefix(text, "@") {
		return false
	}
	target, _, _ := strings.Cut(text[1:], " ")
	target = strings.TrimRight(strings.TrimSpace(target), ":,")
	return strings.EqualFold(target, reader)
}

// WaitForRoom blocks until this reader has unread room events or bound expires.
// A timeout is a successful empty read.
func WaitForRoom(ctx context.Context, id, reader string, limit int, bound time.Duration) error {
	return waitForRoom(ctx, id, reader, limit, bound)
}

func waitForRoom(ctx context.Context, id, reader string, limit int, bound time.Duration) error {
	if bound <= 0 {
		return nil
	}
	timer := time.NewTimer(bound)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		directed, other, _, err := Unread(id, reader, limit)
		if err != nil {
			return err
		}
		if len(directed)+len(other) > 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		case <-ticker.C:
		}
	}
}

// AppendEvent appends one transcript event under a lock distinct from run.lock.
//
// It exists for board/chat-mode writers that may append while a round holds the
// run lease. The lock serializes transcript writes only, and the full-write loop
// refuses the torn-line failure mode that would otherwise strand readers.
func AppendEvent(id string, e Event) error {
	dir, err := storeDir(id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	host, _ := os.Hostname()
	l, err := lockfile.Acquire(filepath.Join(dir, "append.lock"), lockfile.Holder{
		Name: host, PID: os.Getpid(), Intent: "append meeting transcript", Since: nowFn(),
	})
	if err != nil {
		return fmt.Errorf("meet: locking append transcript: %w", err)
	}
	defer l.Release()

	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	f, err := os.OpenFile(filepath.Join(dir, "transcript.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for len(b) > 0 {
		n, err := f.Write(b)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}

func readRoomTranscript(id string) ([]Event, error) {
	dir, err := storeDir(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(dir, "transcript.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []Event
	r := bufio.NewReaderSize(f, maxTranscriptLine+1)
	for {
		line, err := r.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			return nil, fmt.Errorf("meet: transcript line exceeds %d bytes", maxTranscriptLine)
		}
		if len(line) > maxTranscriptLine+1 || (len(line) == maxTranscriptLine+1 && line[len(line)-1] != '\n') {
			return nil, fmt.Errorf("meet: transcript line exceeds %d bytes", maxTranscriptLine)
		}
		if len(line) > 0 {
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) > maxTranscriptLine {
				return nil, fmt.Errorf("meet: transcript line exceeds %d bytes", maxTranscriptLine)
			}
			if len(trimmed) > 0 {
				var e Event
				if json.Unmarshal(trimmed, &e) == nil {
					out = append(out, e)
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
	}
}
