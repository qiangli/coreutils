package meet

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/lockfile"
)

const permanentRoomsSchema = "bashy-meet-permanent-rooms-v1"

// ErrPermanentRoom marks an attempt to close a configured room through the
// ordinary meeting lifecycle. Permanent rooms are host addresses: they stay
// open across steward handoffs and service restarts.
var ErrPermanentRoom = errors.New("meet: permanent rooms cannot be closed")

// PermanentRoomConfig is one desired host-local room. The built-in steward
// room is always present unless a config entry explicitly disables it.
type PermanentRoomConfig struct {
	Name   string   `json:"name"`
	Topic  string   `json:"topic,omitempty"`
	Agenda []string `json:"agenda,omitempty"`
}

type permanentRoomsFile struct {
	Schema string                `json:"schema"`
	Rooms  []PermanentRoomConfig `json:"rooms"`
}

func defaultPermanentRooms() []PermanentRoomConfig {
	return []PermanentRoomConfig{{
		Name: "steward", Topic: "Steward",
		Agenda: []string{"what is running on this host", "contention", "handoff"},
	}}
}

func permanentRoomsConfigPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("BASHY_MEET_ROOMS_FILE")); p != "" {
		return p, nil
	}
	base, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "rooms.json"), nil
}

// ConfiguredPermanentRooms returns the built-in rooms merged with rooms.json.
// Entries in the file override a built-in by name. Additional entries add
// permanent rooms without requiring a code change.
func ConfiguredPermanentRooms() ([]PermanentRoomConfig, error) {
	byName := map[string]PermanentRoomConfig{}
	for _, c := range defaultPermanentRooms() {
		byName[c.Name] = c
	}
	p, err := permanentRoomsConfigPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sortedRoomConfigs(byName), nil
		}
		return nil, fmt.Errorf("meet: read permanent room config: %w", err)
	}
	var f permanentRoomsFile
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("meet: parse %s: %w", p, err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("meet: parse %s: trailing JSON", p)
	}
	if f.Schema != permanentRoomsSchema {
		return nil, fmt.Errorf("meet: %s has schema %q, want %q", p, f.Schema, permanentRoomsSchema)
	}
	seen := map[string]bool{}
	for _, raw := range f.Rooms {
		name, err := permanentRoomName(raw.Name)
		if err != nil {
			return nil, fmt.Errorf("meet: %s: %w", p, err)
		}
		if seen[name] {
			return nil, fmt.Errorf("meet: %s defines permanent room %q more than once", p, name)
		}
		seen[name] = true
		raw.Name = name
		if strings.TrimSpace(raw.Topic) == "" {
			raw.Topic = permanentRoomTitle(name)
		}
		byName[name] = raw
	}
	return sortedRoomConfigs(byName), nil
}

func sortedRoomConfigs(m map[string]PermanentRoomConfig) []PermanentRoomConfig {
	out := make([]PermanentRoomConfig, 0, len(m))
	for _, c := range m {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func permanentRoomName(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(raw, "@")))
	if name == "" || slugify(name) != name {
		return "", fmt.Errorf("invalid permanent room name %q (use lowercase letters, digits, and dashes)", raw)
	}
	return name, nil
}

// EnsureConfiguredPermanentRooms materializes the desired host rooms. It is
// called before meet serve starts; role owners may also ensure one directly.
func EnsureConfiguredPermanentRooms() ([]*State, error) {
	configs, err := ConfiguredPermanentRooms()
	if err != nil {
		return nil, err
	}
	out := make([]*State, 0, len(configs))
	for _, c := range configs {
		st, err := EnsurePermanentRoom(c.Name, CreateOptions{
			Topic: c.Topic, Agenda: c.Agenda, Out: OutStore,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

// EnsurePermanentRoom returns the one durable room with name, creating or
// reopening it when needed. A kernel-held store lock makes simultaneous service
// and steward starts converge on one identity rather than create twins.
func EnsurePermanentRoom(name string, opts CreateOptions) (*State, error) {
	name, err := permanentRoomName(name)
	if err != nil {
		return nil, err
	}
	base, err := baseDir()
	if err != nil {
		return nil, err
	}
	l, err := lockfile.AcquireWithin(filepath.Join(base, "rooms.lock"), 5*time.Second,
		lockfile.Holder{Intent: "ensure permanent meet room"})
	if err != nil {
		return nil, fmt.Errorf("meet: lock permanent rooms: %w", err)
	}
	defer l.Release()

	sessions, err := listSessions()
	if err != nil {
		return nil, err
	}
	var found *State
	for _, st := range sessions {
		if st.Permanent && st.Name == name {
			if found != nil {
				return nil, fmt.Errorf("meet: permanent room %q has duplicate identities %s and %s", name, found.ID, st.ID)
			}
			found = st
		}
	}
	if found != nil {
		if topic := strings.TrimSpace(opts.Topic); topic != "" {
			found.Topic = topic
		}
		if opts.Agenda != nil {
			found.Agenda = append([]string(nil), opts.Agenda...)
		}
		for _, participant := range opts.Participants {
			participant = canonAgent(participant)
			if participant != "" && !containsFold(found.Participants, participant) {
				found.Participants = append(found.Participants, participant)
			}
		}
		if found.Status != "open" || found.Room < 1 || roomHeldByAnother(sessions, found) {
			found.Status = "open"
			found.Room = 0
			found.Room = lowestFreeRoom(sessions)
		}
		if err := found.Validate(); err != nil {
			return nil, err
		}
		if err := found.save(); err != nil {
			return nil, err
		}
		return found, nil
	}

	if strings.TrimSpace(opts.Topic) == "" {
		opts.Topic = permanentRoomTitle(name)
	}
	st, err := Create(opts)
	if err != nil {
		return nil, err
	}
	st.Name, st.Permanent = name, true
	if err := st.save(); err != nil {
		return nil, err
	}
	return st, nil
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func permanentRoomTitle(name string) string {
	words := strings.Split(name, "-")
	for i, word := range words {
		if word != "" {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

func roomHeldByAnother(sessions []*State, target *State) bool {
	for _, st := range sessions {
		if st.ID != target.ID && st.Status == "open" && st.Room == target.Room {
			return true
		}
	}
	return false
}
