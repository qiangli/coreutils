// Package meet implements `bashy meet` — a multi-participant deliberation
// session where agentic CLIs and a human take turns, and a dedicated
// notes-only secretary keeps the minutes and files them.
//
// P0 (this cut) is deliberately minimal: local sessions only, sequential
// round-robin turns, a notes-only secretary, and a docs/ filer. See
// dhnt/docs/bashy-meet.md for the full design and the deferred rungs.
package meet

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const schemaVersion = "bashy-meet-v1"

// nowFn is indirected so tests get deterministic timestamps.
var nowFn = time.Now

// Event is one append-only entry in a meeting transcript.
type Event struct {
	Round   int       `json:"round"`
	Speaker string    `json:"speaker"`
	Role    string    `json:"role,omitempty"`
	Kind    string    `json:"kind"` // agenda|human|turn|decision|action|summary
	Text    string    `json:"text"`
	TS      time.Time `json:"ts"`
}

// State is the durable meeting header, saved as state.json.
type State struct {
	Schema       string    `json:"schema"`
	ID           string    `json:"id"`
	Topic        string    `json:"topic"`
	Agenda       []string  `json:"agenda,omitempty"`
	Secretary    string    `json:"secretary"`
	Participants []string  `json:"participants"`
	Human        string    `json:"human"`
	Mode         string    `json:"mode"`
	Status       string    `json:"status"`
	Cwd          string    `json:"cwd"`
	Out          string    `json:"out,omitempty"`
	Created      time.Time `json:"created"`
	Round        int       `json:"round"`
}

// baseDir is the root of the local session store. Overridable via
// BASHY_MEET_DIR (used by tests and by operators who want a custom location).
func baseDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv("BASHY_MEET_DIR")); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".bashy", "meet"), nil
}

func storeDir(id string) (string, error) {
	base, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, id), nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 48 {
		out = strings.Trim(out[:48], "-")
	}
	if out == "" {
		out = "meeting"
	}
	return out
}

// newID derives a stable session id from the topic + timestamp.
func newID(topic string, now time.Time) string {
	sum := sha256.Sum256([]byte(topic + now.Format(time.RFC3339Nano)))
	short := hex.EncodeToString(sum[:])[:4]
	return fmt.Sprintf("%s-%s-%s", now.Format("2006-01-02"), slugify(topic), short)
}

func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *State) save() error {
	dir, err := storeDir(s.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	s.Schema = schemaVersion
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, "state.json"), b)
}

func loadState(id string) (*State, error) {
	dir, err := storeDir(id)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func appendEvent(id string, e Event) error {
	dir, err := storeDir(id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// O_APPEND makes concurrent turn-writes safe without a lock.
	f, err := os.OpenFile(filepath.Join(dir, "transcript.jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
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

func readTranscript(id string) ([]Event, error) {
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
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

func listSessions() ([]*State, error) {
	base, err := baseDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*State
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if s, err := loadState(e.Name()); err == nil {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}
