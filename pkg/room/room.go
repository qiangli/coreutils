// Package room is the same-host "host room": the canonical registry of live
// agentic-tool instances (membership) plus an append-only event log (timeline).
//
// It is the P0 rung of the agent room mesh (docs/agent-room-mesh-design.md):
// discovery is membership, connection is the card's control socket, and the
// timeline is the coordination stream the notification bus, coach, and task
// continuity record all converge on. The Card and Event shapes are deliberately
// projection-friendly (A2A Agent Card / Matrix event) so the same-host store can
// later be pushed to a cloudbox and, eventually, federated — without changing the
// shape a member publishes.
package room

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Card is one live instance's membership record — who it is, what it is bound to,
// and how to reach it on this host.
type Card struct {
	ID        string   `json:"id"`
	Principal string   `json:"principal,omitempty"` // who launched it
	Tool      string   `json:"tool"`
	Model     string   `json:"model,omitempty"`
	Binding   string   `json:"binding"` // tool:model
	Nick      string   `json:"nick,omitempty"`
	Band      int      `json:"band,omitempty"`
	Mode      string   `json:"mode,omitempty"` // interactive | weave | foreman | meet
	Task      string   `json:"task,omitempty"` // what it is working on, if known
	Caps      []string `json:"caps,omitempty"`
	CtlSock   string   `json:"ctl_sock,omitempty"` // same-host reach
	LogPath   string   `json:"log_path,omitempty"`
	// PID is the process whose liveness the membership tracks — the room prunes a
	// card whose pid is gone on read, so it never asserts a dead member is live.
	PID    int    `json:"pid"`
	Cwd    string `json:"cwd,omitempty"`
	Native bool   `json:"native,omitempty"` // self-governing harness (ycode)
	Events bool   `json:"events,omitempty"` // speaks a structured event channel
	Joined string `json:"joined"`
}

// Event is one timeline entry — a join/leave/steer/status/note the room records.
type Event struct {
	Seq       int64  `json:"seq"`
	TS        string `json:"ts"`
	Type      string `json:"type"` // join | leave | steer | interrupt | status | note | notify
	Actor     string `json:"actor,omitempty"`
	Target    string `json:"target,omitempty"`
	Body      string `json:"body,omitempty"`
	Principal string `json:"principal,omitempty"` // who sent this notification (REQUIRED for notify)
	Topic     string `json:"topic,omitempty"`     // topic broadcast key
	Room      string `json:"room,omitempty"`      // room-scoped addressing
	To        string `json:"to,omitempty"`        // 1:1 recipient (session or role)
}

const (
	EventJoin      = "join"
	EventLeave     = "leave"
	EventSteer     = "steer"
	EventInterrupt = "interrupt"
	EventGrant     = "grant"
	EventStatus    = "status"
	EventNote      = "note"
	EventNotify    = "notify"
)

var appendMu sync.Mutex

// Dir is the room root (~/.bashy/room), overridable with $BASHY_ROOM_DIR so a test
// gets an isolated room.
func Dir() string {
	if d := strings.TrimSpace(os.Getenv("BASHY_ROOM_DIR")); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "bashy-room")
	}
	return filepath.Join(home, ".bashy", "room")
}

func membersDir() (string, error) {
	d := filepath.Join(Dir(), "members")
	return d, os.MkdirAll(d, 0o700)
}

func timelinePath() (string, error) {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return "", err
	}
	return filepath.Join(Dir(), "timeline.jsonl"), nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// Join publishes a membership card and records a join event.
func Join(c Card) error {
	dir, err := membersDir()
	if err != nil {
		return err
	}
	if c.Joined == "" {
		c.Joined = now()
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, c.ID+".json"), b, 0o600); err != nil {
		return err
	}
	return Emit(Event{Type: EventJoin, Actor: c.Principal, Target: c.ID, Body: c.Binding})
}

// Leave removes a membership card and records a leave event.
func Leave(id string) {
	dir, err := membersDir()
	if err != nil {
		return
	}
	_ = os.Remove(filepath.Join(dir, id+".json"))
	_ = Emit(Event{Type: EventLeave, Target: id})
}

// Members returns the live membership, newest first, pruning any card whose pid is
// gone (a crash left the file behind). Reading IS the reconciliation — no sweeper,
// so the board never asserts a dead member is live.
func Members() ([]Card, error) {
	dir, err := membersDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Card
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var c Card
		if json.Unmarshal(b, &c) != nil || c.ID == "" {
			continue
		}
		if !PidAlive(c.PID) {
			_ = os.Remove(p)
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Joined > out[j].Joined })
	return out, nil
}

// Find resolves an id to a live member. A unique id/nick prefix matches, so an
// operator can type `elif` when only one such member is up.
func Find(id string) (Card, bool, error) {
	id = strings.TrimSpace(id)
	members, err := Members()
	if err != nil {
		return Card{}, false, err
	}
	if id == "" {
		if len(members) == 1 {
			return members[0], true, nil
		}
		return Card{}, false, nil
	}
	for _, c := range members {
		if c.ID == id {
			return c, true, nil
		}
	}
	var pref []Card
	for _, c := range members {
		if strings.HasPrefix(c.ID, id) || strings.EqualFold(c.Nick, id) {
			pref = append(pref, c)
		}
	}
	if len(pref) == 1 {
		return pref[0], true, nil
	}
	return Card{}, false, nil // 0 or ambiguous — caller reports the count
}

// Emit appends an event to the timeline. Seq is the append order (line count),
// assigned under a lock so concurrent emitters do not collide.
func Emit(e Event) error {
	path, err := timelinePath()
	if err != nil {
		return err
	}
	appendMu.Lock()
	defer appendMu.Unlock()
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

// Timeline returns the last n events (all when n <= 0), oldest-first.
func Timeline(n int) ([]Event, error) {
	path, err := timelinePath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var all []Event
	seq := int64(0)
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e Event
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		seq++
		e.Seq = seq
		all = append(all, e)
	}
	if n > 0 && len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}

// Notify publishes a notification event to the timeline after enforcing the
// REPORT/AUTHOR invariant: every notification must carry a non-empty Principal
// asserting who sent it. A notification with no principal is rejected.
func Notify(e Event) error {
	if strings.TrimSpace(e.Principal) == "" {
		return fmt.Errorf("notify: principal is required (REPORT/AUTHOR invariant)")
	}
	if e.Type == "" {
		e.Type = EventNotify
	}
	return Emit(e)
}
