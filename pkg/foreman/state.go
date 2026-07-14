package foreman

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	StatusIdle    = "idle"
	StatusWorking = "working"
	StatusBlocked = "blocked"
	StatusDone    = "done"
)

const (
	CommandTell   = "tell"
	CommandPause  = "pause"
	CommandResume = "resume"
	CommandSkip   = "skip"
	CommandPrio   = "prio"
	CommandStop   = "stop"
)

type State struct {
	ID          string    `json:"id"`
	Goal        string    `json:"goal"`
	Status      string    `json:"status"`
	CurrentStep string    `json:"current_step,omitempty"`
	DriveLease  string    `json:"drive_lease,omitempty"`
	CtlSock     string    `json:"ctl_sock,omitempty"`
	Agent       string    `json:"agent,omitempty"`
	Role        string    `json:"role,omitempty"`
	Cwd         string    `json:"cwd,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Stopped     bool      `json:"stopped,omitempty"`
	Paused      bool      `json:"paused,omitempty"`

	// Binding is the canonical tool:model this session is actually talking to.
	// Agent may be an alias or a nickname; a record must never store one of those.
	Binding string `json:"binding,omitempty"`

	// Steering says whether `tell` reaches a LIVE agent (a keystroke into an open
	// session) or merely queues a message for the next fresh spawn.
	//
	// The two look identical from outside — the operator types tell, the status
	// goes to working, an answer comes back — and they are not remotely the same
	// thing. So the state says which one happened, and SteerWhyNot says why when it
	// is the lesser one. An operator who thinks they interrupted an agent, and did
	// not, has been lied to by silence.
	Steering    bool   `json:"steering"`
	SteerWhyNot string `json:"steer_why_not,omitempty"`
}

type Command struct {
	Seq      int64     `json:"seq,omitempty"`
	Verb     string    `json:"verb"`
	Message  string    `json:"message,omitempty"`
	Target   string    `json:"target,omitempty"`
	Priority string    `json:"priority,omitempty"`
	At       time.Time `json:"at"`
}

type Store struct {
	Root string
	ID   string
}

func DefaultRoot() string {
	if v := strings.TrimSpace(os.Getenv("BASHY_FOREMAN_DIR")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("BASHY_HOME")); v != "" {
		return filepath.Join(v, "foreman")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "bashy", "foreman")
	}
	return filepath.Join(home, ".bashy", "foreman")
}

func NewStore(root, id string) Store {
	if root == "" {
		root = DefaultRoot()
	}
	return Store{Root: root, ID: id}
}

func (s Store) Dir() string {
	return filepath.Join(s.Root, s.ID)
}

func (s Store) StatePath() string {
	return filepath.Join(s.Dir(), "state.json")
}

func (s Store) CommandsPath() string {
	return filepath.Join(s.Dir(), "commands")
}

func (s Store) CtlSockPath() string {
	p := filepath.Join(s.Dir(), "ctl.sock")
	if len(p) <= 100 {
		return p
	}
	return filepath.Join(os.TempDir(), "bashy-foreman-"+s.ID+".sock")
}

func (s Store) Ensure() error {
	return os.MkdirAll(s.Dir(), 0o700)
}

func (s Store) LoadState() (State, error) {
	data, err := os.ReadFile(s.StatePath())
	if err != nil {
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, err
	}
	return st, nil
}

func (s Store) SaveState(st State) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	st.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.StatePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.StatePath())
}

func (s Store) AppendCommand(cmd Command) error {
	if strings.TrimSpace(cmd.Verb) == "" {
		return errors.New("foreman: command verb required")
	}
	if err := s.Ensure(); err != nil {
		return err
	}
	cmd.At = time.Now().UTC()
	f, err := os.OpenFile(s.CommandsPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(cmd)
}

func (s Store) LoadCommands() ([]Command, error) {
	f, err := os.Open(s.CommandsPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Command
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var c Command
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, fmt.Errorf("foreman: read command: %w", err)
		}
		out = append(out, c)
	}
	return out, sc.Err()
}

func (s Store) TruncateCommands() error {
	if err := s.Ensure(); err != nil {
		return err
	}
	return os.WriteFile(s.CommandsPath(), nil, 0o600)
}

func List(root string) ([]State, error) {
	if root == "" {
		root = DefaultRoot()
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []State
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		st, err := NewStore(root, e.Name()).LoadState()
		if err == nil {
			out = append(out, st)
		}
	}
	return out, nil
}
