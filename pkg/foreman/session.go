package foreman

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
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
	kbNote  *string    // cached host-kb preamble for the session goal
	mu      sync.Mutex // guards state/history; HELD FOR THE WHOLE TURN

	// live is guarded by its OWN lock, deliberately.
	//
	// Apply holds s.mu for the entire duration of a turn — that is correct, a turn
	// is one atomic thing. But a STEER must reach the agent *while that turn is
	// running*, which means it cannot wait on s.mu: it would block until the very
	// turn it was trying to interrupt had already finished. A steer that waits for
	// the turn to end is not a steer, it is a note left on the desk.
	liveMu  sync.Mutex
	live    *chat.Session
	steers  []string // mid-turn corrections, appended under liveMu
	logFile *os.File // the live agent's output, tee'd for `foreman log`
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
	s.mu.Lock()
	defer s.mu.Unlock()
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

// persistLocked writes state.json. The caller must ALREADY hold s.mu — it reads
// s.state directly rather than going through State(), which would re-take the
// lock and deadlock.
//
// It exists because status must be true DURING a turn, not merely after it.
// SaveState used to run only once Apply returned, so for the entire time an agent
// was working, `foreman status` reported "idle" and `steering: false` -- the file
// described a session that was doing nothing, while an agent burned tokens. An
// operator cannot supervise a run whose status only becomes true once there is
// nothing left to supervise.
func (s *Session) persistLocked() { _ = s.store.SaveState(s.state) }

func (s *Session) Apply(ctx context.Context, cmd Command) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
		s.persistLocked() // the turn has STARTED; say so now, not when it ends
		if s.runner == nil && strings.TrimSpace(s.state.Agent) == "" && strings.TrimSpace(s.state.Role) == "" {
			s.state.Status = StatusIdle
			return nil
		}

		// STEER a live agent when we can. `tell` means "lean over and say something
		// to the agent that is working right now" — and for most of this fleet's
		// tools that is now literally what it does.
		if ok, why := s.steerable(); ok {
			if err := s.steer(ctx, cmd.Message); err != nil {
				// Falling through to the replay path would be the wrong kindness: it
				// would produce a plausible answer from a fresh agent and hide the fact
				// that the live one is gone.
				s.state.Status = StatusBlocked
				s.state.Steering = false
				s.state.SteerWhyNot = err.Error()
				return err
			}
			s.state.Status = StatusIdle
			break
		} else {
			// Not steerable. The replay path below still works — it is just a
			// different thing, and the state must not pretend otherwise.
			s.state.Steering = false
			s.state.SteerWhyNot = why
		}

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
		if strings.TrimSpace(cmd.Target) != "" {
			s.state.CurrentStep = "skip:" + strings.TrimSpace(cmd.Target)
		} else {
			s.state.CurrentStep = ""
		}
		s.state.Status = StatusIdle
	case CommandPrio:
		if strings.TrimSpace(cmd.Target) != "" {
			s.state.DriveLease = strings.TrimSpace(cmd.Target) + ":" + strings.TrimSpace(cmd.Priority)
		} else {
			s.state.DriveLease = strings.TrimSpace(cmd.Priority)
		}
	case CommandStop:
		s.state.Stopped = true
		s.state.Status = StatusDone
		// Ask the live agent to leave. Stopping a foreman while its agent sits at a
		// prompt would strand a process holding an API session open indefinitely.
		s.closeLive()
	default:
		return fmt.Errorf("foreman: unknown command %q", cmd.Verb)
	}
	return nil
}

func (s *Session) composePrompt(msg string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Goal:\n%s\n\n", s.state.Goal)
	if note := s.kbPreamble(); note != "" {
		b.WriteString(note)
		b.WriteByte('\n')
	}
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
