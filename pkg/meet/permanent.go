package meet

import (
	"bytes"
	"context"
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
	Name          string   `json:"name"`
	Topic         string   `json:"topic,omitempty"`
	Agenda        []string `json:"agenda,omitempty"`
	Agent         string   `json:"agent,omitempty"`
	Band          int      `json:"band,omitempty"`
	AutoStart     *bool    `json:"auto_start,omitempty"`
	Secretary     string   `json:"secretary,omitempty"`
	SecretaryBand int      `json:"secretary_band,omitempty"`
}

// PermanentRoleStartRequest is the host-neutral request Meet emits when a
// human addresses an unoccupied permanent role. The embedding host owns agent
// process lifecycle; Meet owns only the durable room and routing alias.
type PermanentRoleStartRequest struct {
	Room  string
	Role  string
	Agent string
	Band  int
}

// StartPermanentRole is supplied by bashy, the host that can select and launch
// an agent. A bare coreutils embedding leaves it nil and fails clearly instead
// of pretending a role was started.
var StartPermanentRole func(context.Context, PermanentRoleStartRequest) error

type permanentRoomsFile struct {
	Schema string                `json:"schema"`
	Rooms  []PermanentRoomConfig `json:"rooms"`
}

func defaultPermanentRooms() []PermanentRoomConfig {
	auto := true
	return []PermanentRoomConfig{{
		Name: "steward", Topic: "Steward",
		Agenda: []string{"what is running on this host", "contention", "handoff"},
		Band:   4, AutoStart: &auto, SecretaryBand: 2,
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
		if raw.Band < 0 || raw.Band > 4 {
			return nil, fmt.Errorf("meet: %s: permanent room %q band must be 1-4", p, name)
		}
		if raw.SecretaryBand < 0 || raw.SecretaryBand > 4 {
			return nil, fmt.Errorf("meet: %s: permanent room %q secretary_band must be 1-4", p, name)
		}
		// An override may change only the heading or agenda. Preserve the
		// built-in steward's lazy-start policy unless explicitly disabled.
		if prior, ok := byName[name]; ok {
			if raw.AutoStart == nil {
				raw.AutoStart = prior.AutoStart
			}
			if raw.Band == 0 {
				raw.Band = prior.Band
			}
			if raw.SecretaryBand == 0 {
				raw.SecretaryBand = prior.SecretaryBand
			}
		}
		byName[name] = raw
	}
	return sortedRoomConfigs(byName), nil
}

func configuredPermanentRoom(name string) (PermanentRoomConfig, bool, error) {
	configs, err := ConfiguredPermanentRooms()
	if err != nil {
		return PermanentRoomConfig{}, false, err
	}
	for _, config := range configs {
		if config.Name == name {
			return config, true, nil
		}
	}
	return PermanentRoomConfig{}, false, nil
}

// ensurePermanentRoleStarted serializes lazy starts across browser requests,
// invokes the embedding host, then verifies positive evidence: the durable
// alias must name the agent that actually took the role.
func ensurePermanentRoleStarted(ctx context.Context, st *State, roleName string) (string, error) {
	base, err := baseDir()
	if err != nil {
		return "", err
	}
	l, err := lockfile.AcquireWithin(filepath.Join(base, "role-start.lock"), 30*time.Second,
		lockfile.Holder{Intent: "start permanent meet role"})
	if err != nil {
		return "", fmt.Errorf("meet: lock permanent role start: %w", err)
	}
	defer l.Release()

	fresh, err := loadState(st.ID)
	if err != nil {
		return "", err
	}
	if holder := strings.TrimSpace(fresh.RoleHolders[roleName]); holder != "" {
		return holder, nil
	}
	config, ok, err := configuredPermanentRoom(fresh.Name)
	if err != nil {
		return "", err
	}
	if !ok || config.AutoStart == nil || !*config.AutoStart {
		return "", fmt.Errorf("meet: @%s has no current holder and automatic start is disabled", roleName)
	}
	if StartPermanentRole == nil {
		return "", fmt.Errorf("meet: @%s has no current holder and this host cannot start agents", roleName)
	}
	band := config.Band
	if band == 0 {
		band = 4
	}
	if err := StartPermanentRole(ctx, PermanentRoleStartRequest{
		Room: fresh.Name, Role: roleName, Agent: strings.TrimSpace(config.Agent), Band: band,
	}); err != nil {
		return "", fmt.Errorf("meet: start @%s: %w", roleName, err)
	}
	fresh, err = loadState(st.ID)
	if err != nil {
		return "", err
	}
	holder := strings.TrimSpace(fresh.RoleHolders[roleName])
	if holder == "" {
		return "", fmt.Errorf("meet: start @%s returned without assigning the role", roleName)
	}
	return holder, nil
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
			Secretary: c.Secretary, SecretaryBand: c.SecretaryBand,
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
	return ensurePermanentRoom(name, opts, nil)
}

// EnsurePermanentRoleRoom is EnsurePermanentRoom plus the current holder of a
// stable role alias. It is the role-lifecycle seam used by the steward: the
// authoritative seat record remains in pkg/steward, while Meet gets the one
// routing fact it needs for @steward and roster authorization.
func EnsurePermanentRoleRoom(name, roleName, holder string, opts CreateOptions) (*State, error) {
	roleName, err := permanentRoomName(roleName)
	if err != nil {
		return nil, err
	}
	holder = canonAgent(holder)
	if holder == "" {
		return nil, fmt.Errorf("meet: permanent role %q has no holder", roleName)
	}
	return ensurePermanentRoom(name, opts, map[string]string{roleName: holder})
}

// ClearPermanentRoleHolder removes a role alias only when holder still owns
// it. The compare prevents a late predecessor shutdown from erasing the
// successor that already took over the permanent room.
func ClearPermanentRoleHolder(ref, roleName, holder string) error {
	roleName, err := permanentRoomName(roleName)
	if err != nil {
		return err
	}
	base, err := baseDir()
	if err != nil {
		return err
	}
	l, err := lockfile.AcquireWithin(filepath.Join(base, "rooms.lock"), 5*time.Second,
		lockfile.Holder{Intent: "release permanent meet role"})
	if err != nil {
		return fmt.Errorf("meet: lock permanent rooms: %w", err)
	}
	defer l.Release()

	id, err := resolveMeeting(ref)
	if err != nil {
		return err
	}
	st, err := loadState(id)
	if err != nil {
		return err
	}
	if !st.Permanent {
		return fmt.Errorf("meet: %s is not a permanent room", ref)
	}
	current := st.RoleHolders[roleName]
	if current == "" || !strings.EqualFold(canonAgent(current), canonAgent(holder)) {
		return nil
	}
	delete(st.RoleHolders, roleName)
	return st.save()
}

func ensurePermanentRoom(name string, opts CreateOptions, roles map[string]string) (*State, error) {
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
		if secretary := strings.TrimSpace(opts.Secretary); secretary != "" {
			if err := routableSeat(secretary); err != nil {
				return nil, err
			}
			found.Secretary = canonAgent(secretary)
			found.SecretaryPending = false
		} else if found.Secretary == "" && StartRoomSecretary != nil {
			found.SecretaryPending = true
			found.SecretaryBand = opts.SecretaryBand
			if found.SecretaryBand == 0 {
				found.SecretaryBand = 2
			}
		}
		for _, participant := range opts.Participants {
			participant = canonAgent(participant)
			if participant != "" && !containsFold(found.Participants, participant) {
				found.Participants = append(found.Participants, participant)
			}
		}
		if found.RoleHolders == nil && len(roles) > 0 {
			found.RoleHolders = map[string]string{}
		}
		for roleName, holder := range roles {
			found.RoleHolders[roleName] = holder
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
	if len(roles) > 0 {
		st.RoleHolders = make(map[string]string, len(roles))
		for roleName, holder := range roles {
			st.RoleHolders[roleName] = holder
		}
	}
	if err := st.Validate(); err != nil {
		return nil, err
	}
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
