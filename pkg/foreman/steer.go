package foreman

import (
	"context"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/chat"
)

// What `foreman tell` used to do
//
// It composed the goal + the whole session history + the new message into one
// prompt, spawned a BRAND NEW agent process, waited for it to exit, and appended
// its output to the history. Every message. That has three problems, and the third
// is the one that matters:
//
//  1. It re-pays the entire conversation in tokens on every turn.
//  2. The agent has no continuity — no open files, no train of thought, nothing it
//     learned last turn except what got scraped from its stdout.
//  3. IT CANNOT INTERRUPT ANYTHING. The whole point of a foreman is to lean over
//     and say "not that file — the other one" WHILE the agent is working. Queuing
//     a message for the next spawn is not steering; it is leaving a note.
//
// And it was indistinguishable, from the outside, from a foreman that did steer.
// The operator typed `tell`, the state went to `working`, an answer came back. It
// looked right. It was a conversation with an agent that had already left.
//
// So: hold the agent OPEN. `tell` becomes a keystroke into a live session, which
// is what the word always meant.
//
// # When we cannot
//
// A tool with no interactive launch cannot be steered — there is nothing running
// to steer. For those the replay path is not a bug, it is the only thing that
// works, and the state says so out loud (`steering: false` + why). A silent
// downgrade here would recreate the exact failure this change exists to fix.

// quietPeriod is how long an agent must be silent before its turn is considered
// over. See chat.Session.WaitIdle: silence is the only turn boundary a terminal
// offers, and this is sized for a slow tool call, not for a human's patience.
const quietPeriod = 25 * time.Second

// steerable reports whether this session can hold its agent open.
//
// An injected Runner always wins: it is the test seam and the programmatic entry
// point, and a caller that supplied its own runner asked for exactly that runner.
func (s *Session) steerable() (bool, string) {
	if s.runner != nil {
		return false, "a runner was supplied by the caller"
	}
	agent := strings.TrimSpace(s.state.Agent)
	if agent == "" {
		// A role resolves to an agent, but the agent it resolves to may change
		// between turns — which is a fine property for one-shot dispatch and a
		// nonsensical one for a held-open session. Steer a named agent.
		return false, "no agent named (a role picks a fresh agent per turn; a session holds one open)"
	}
	return chat.CanSteer(agent)
}

// attach opens the live session, using the first message as its opening prompt.
func (s *Session) attach(ctx context.Context, msg string) error {
	live, err := chat.Start(ctx, s.state.Agent, chat.SessionOptions{
		Prompt:   s.composePrompt(msg),
		Cwd:      s.state.Cwd,
		ReadOnly: false, // a foreman's agent is here to DO the work
	})
	if err != nil {
		return err
	}
	s.live = live
	s.state.Steering = true
	s.state.SteerWhyNot = ""
	s.state.CtlSock = live.CtlSock
	s.state.Binding = live.Agent
	return nil
}

// steer sends one message to the live agent and records what it said back.
//
// The opening message carries the goal and any host-kb preamble. Every message
// after it is sent RAW — the agent already has the conversation; replaying it
// would be both wasteful and confusing, since the agent would see its own words
// quoted back at it as if they were new.
func (s *Session) steer(ctx context.Context, msg string) error {
	if s.live == nil || !s.live.Live() {
		if err := s.attach(ctx, msg); err != nil {
			return err
		}
	} else if err := s.live.Say(msg); err != nil {
		return err
	}

	// Silence means the agent has stopped talking. It does NOT mean the agent has
	// finished thinking, and it certainly does not mean the agent did what it was
	// asked — for THAT, read the artifacts, not the terminal.
	_ = s.live.WaitIdle(ctx, quietPeriod)

	if out := strings.TrimSpace(chat.SanitizeTurn(s.live.Turn())); out != "" {
		s.history = append(s.history, "agent: "+out)
	}
	if !s.live.Live() {
		// It left. Say so rather than letting the next tell silently start a fresh
		// agent that knows nothing about this conversation.
		if _, err := s.live.Wait(); err != nil {
			s.live = nil
			s.state.Steering = false
			s.state.SteerWhyNot = "the agent exited: " + err.Error()
			return nil
		}
		s.live = nil
		s.state.Steering = false
		s.state.SteerWhyNot = "the agent exited"
	}
	return nil
}

// Close ends the live agent, if there is one. A foreman that is done with a
// session must not leave an agent sitting at a prompt forever.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLive()
}

// closeLive ends the live agent. The caller must hold s.mu — Apply already does.
func (s *Session) closeLive() {
	if s.live == nil {
		return
	}
	s.live.Close()
	s.live = nil
	s.state.Steering = false
}
