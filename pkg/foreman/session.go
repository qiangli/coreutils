package foreman

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/chat"
)

type Options struct {
	ID     string
	Goal   string
	Agent  string
	Role   string
	Cwd    string
	Root   string
	Runner chat.Runner
}

type Session struct {
	store   Store
	state   State
	runner  chat.Runner
	history []string
}

func Start(ctx context.Context, opt Options) (*Session, error) {
	id := strings.TrimSpace(opt.ID)
	if id == "" {
		id = newID()
	}
	goal := strings.TrimSpace(opt.Goal)
	if goal == "" {
		return nil, errors.New("foreman: goal required")
	}
	store := NewStore(opt.Root, id)
	now := time.Now().UTC()
	st := State{
		ID:        id,
		Goal:      goal,
		Status:    StatusIdle,
		CtlSock:   store.CtlSockPath(),
		Agent:     opt.Agent,
		Role:      opt.Role,
		Cwd:       opt.Cwd,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.SaveState(st); err != nil {
		return nil, err
	}
	s := &Session{store: store, state: st, runner: opt.Runner}
	return s, nil
}

func Open(root, id string, runner chat.Runner) (*Session, error) {
	store := NewStore(root, id)
	st, err := store.LoadState()
	if err != nil {
		return nil, err
	}
	return &Session{store: store, state: st, runner: runner}, nil
}

func (s *Session) State() State {
	return s.state
}

func (s *Session) Store() Store {
	return s.store
}

func (s *Session) Enqueue(cmd Command) error {
	return s.store.AppendCommand(cmd)
}

func (s *Session) ProcessPending(ctx context.Context) error {
	cmds, err := s.store.LoadCommands()
	if err != nil {
		return err
	}
	if len(cmds) == 0 {
		return nil
	}
	if err := s.store.TruncateCommands(); err != nil {
		return err
	}
	for _, cmd := range cmds {
		if err := s.Apply(ctx, cmd); err != nil {
			return err
		}
	}
	return s.store.SaveState(s.state)
}

func (s *Session) Apply(ctx context.Context, cmd Command) error {
	switch strings.ToLower(strings.TrimSpace(cmd.Verb)) {
	case CommandTell:
		if strings.TrimSpace(cmd.Message) == "" {
			return errors.New("foreman: tell message required")
		}
		if s.state.Paused {
			s.state.Status = StatusBlocked
			s.history = append(s.history, "human: "+cmd.Message)
			return nil
		}
		s.state.Status = StatusWorking
		s.state.CurrentStep = cmd.Message
		s.history = append(s.history, "human: "+cmd.Message)
		res, err := chat.Invoke(ctx, chat.Options{
			Agent:       s.state.Agent,
			Role:        s.state.Role,
			Instruction: s.composePrompt(cmd.Message),
			Cwd:         s.state.Cwd,
		}, s.runner)
		if res.Output != "" {
			s.history = append(s.history, "agent: "+strings.TrimSpace(res.Output))
		}
		if err != nil || res.ExitCode != 0 {
			s.state.Status = StatusBlocked
			if err != nil {
				return err
			}
			return fmt.Errorf("foreman: runner exited %d", res.ExitCode)
		}
		s.state.Status = StatusIdle
	case CommandPause:
		s.state.Paused = true
		s.state.Status = StatusBlocked
	case CommandResume:
		s.state.Paused = false
		s.state.Status = StatusIdle
	case CommandSkip:
		s.state.CurrentStep = ""
		s.state.Status = StatusIdle
	case CommandPrio:
		s.state.DriveLease = strings.TrimSpace(cmd.Priority)
	case CommandStop:
		s.state.Stopped = true
		s.state.Status = StatusDone
	default:
		return fmt.Errorf("foreman: unknown command %q", cmd.Verb)
	}
	return nil
}

func (s *Session) composePrompt(msg string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Goal:\n%s\n\n", s.state.Goal)
	if len(s.history) > 0 {
		b.WriteString("Session history:\n")
		for _, h := range s.history {
			b.WriteString(h)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "Steering message:\n%s", msg)
	return b.String()
}

func newID() string {
	return time.Now().UTC().Format("20060102-150405.000000000")
}
