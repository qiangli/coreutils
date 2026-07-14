package chat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/qiangli/coreutils/pkg/agentctl"
	"github.com/qiangli/coreutils/pkg/agentpty"
)

// A Session is a live agent you can talk to.
//
// It exists because the fleet had grown THREE different ideas of what "steering"
// means, and only one of them steered anything:
//
//   - `weave say`   wrote to a run's pty control socket. Real mid-turn steering.
//   - `meet say`    wrote to the same kind of socket — but meet's turns were
//     headless one-shots, so there was usually nothing on the
//     other end to hear it.
//   - `foreman tell` queued a message and spawned a WHOLE NEW chat.Invoke. That is
//     conversation, not steering: it cannot interrupt anything,
//     because by the time the message lands the agent has exited.
//
// One primitive, so a chair, a conductor, a foreman and an operator all mean the
// same thing by "tell it to stop doing that".
//
// # Session vs Invoke
//
// Invoke is a QUESTION: one prompt, one answer, a clean captured turn (stdout and
// stderr stay apart on a pipe). Use it when you want a reply.
//
// Session is a CONVERSATION: the agent's interactive launch (steer_exec) under a
// pty, held open, with a control socket. Use it when you need to interrupt.
//
// The trade is real and the registry refuses to hide it: a pty merges stdout and
// stderr, so the tool's own chrome lands in the captured text. You pay that to be
// able to say "stop, you're off the agenda" without killing the turn and losing
// everything it had already said.
type Session struct {
	Agent   string // the canonical binding, as recorded
	Nick    string // the name the caller used
	CtlSock string // where a steer lands

	mu        sync.Mutex
	buf       strings.Builder
	mark      int // end of the last Turn
	lastWrite time.Time
	done      chan struct{}
	exit      int
	err       error
	killed    string
}

// SessionOptions configures a live agent session.
type SessionOptions struct {
	// Prompt opens the conversation. A tool whose steerable launch takes a prompt
	// on the command line (agy -i) gets it there; one that opens an empty session
	// (codex, opencode) is SENT it over the control channel once it is up, which
	// is the same thing from the caller's side and not from the tool's.
	Prompt string

	Cwd     string
	Timeout time.Duration

	// Stream receives the agent's output as it is written — the same tee Invoke
	// offers, so an observer sees a session exactly as it sees a turn.
	Stream io.Writer

	// ReadOnly strips the approval-gate kill-switches. A session that only has to
	// TALK needs no write authority, and read-only passes the launch guard by
	// construction on an ordinary host.
	ReadOnly bool
}

// Start launches an agent's interactive session.
//
// It fails loudly when the tool has no steerable launch, rather than quietly
// starting a one-shot that will exit before the first steer arrives. A caller
// that asked for a conversation must not be handed a monologue.
func Start(ctx context.Context, agent string, opt SessionOptions) (*Session, error) {
	if !agentpty.Supported() {
		return nil, fmt.Errorf("chat: a steerable session needs a pty, which this platform has no support for")
	}

	name, err := ResolveAgent(agent, "")
	if err != nil {
		return nil, err
	}
	// Deliberately the SAME resolver Invoke uses. A steerable session differs from
	// a turn in exactly one respect — which launch template it renders — and
	// everything else (the containerized guard, the read-only argv stripping, the
	// canonical binding) must not be able to drift.
	l, err := resolveLaunch(name, Options{
		Cwd:      opt.Cwd,
		ReadOnly: opt.ReadOnly,
		Steer:    true, // the interactive launch, never the one-shot
	})
	if err != nil {
		return nil, err
	}

	argv := append([]string{}, l.Args...)
	if l.TakesPrompt && strings.TrimSpace(opt.Prompt) != "" {
		argv = append(argv, opt.Prompt)
	}

	cwd := opt.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	// The socket is short and hashed, NOT under a long workspace path: a unix
	// socket address caps at ~104 bytes, and blowing that degrades steering to a
	// polling file channel without saying so.
	sock, err := sessionSock(l.Binding(), cwd)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, l.Tool, argv...)
	cmd.Dir = cwd
	// agentChildEnv is the whole reason Session lives in this package and not in
	// agentpty: it is what scrubs the operator's vault secrets out of the child,
	// grants back only the one API key this model declared, forces the agent's
	// shell to be bashy, and stamps its principal identity. A session launcher
	// built anywhere else would silently drop all four, and nothing would fail
	// loudly enough for anyone to notice.
	cmd.Env = agentChildEnv(withLaunch(ctx, l))

	// Prevention before cure: seed the tool's own config so its trust prompt never
	// appears. agentpty's gate classifier is the backstop for the ones it misses.
	if p, ok := agentctl.ProfileFor(l.ToolName); ok && p.Preseed != "" {
		_ = agentctl.ApplyTrustPreseed(cmd.Dir, p.Preseed)
	}

	s := &Session{
		Agent:   l.Binding(),
		Nick:    l.Nick,
		CtlSock: sock,
		done:    make(chan struct{}),
		// Seed the idle clock at launch. An agent that says NOTHING must still go
		// idle — otherwise WaitIdle would hang forever on the one failure it most
		// needs to report, which is an agent that never spoke.
		lastWrite: time.Now(),
	}

	sink := io.Writer(&sessionWriter{s: s})
	if opt.Stream != nil {
		sink = io.MultiWriter(sink, opt.Stream)
	}

	go func() {
		defer close(s.done)
		exit, killed, err := agentpty.Run(cmd, sink, agentpty.Options{
			CtlSock:    sock,
			Capture:    true, // the caller records this; the human watches via observe
			MaxRuntime: opt.Timeout,
		})
		s.mu.Lock()
		s.exit, s.killed, s.err = exit, killed, err
		s.mu.Unlock()
	}()

	// A tool that opens an EMPTY session takes its first message over the wire.
	if !l.TakesPrompt && strings.TrimSpace(opt.Prompt) != "" {
		if err := s.waitReady(ctx); err != nil {
			return s, err
		}
		if err := s.Say(opt.Prompt); err != nil {
			return s, fmt.Errorf("chat: could not open the conversation: %w", err)
		}
	}
	return s, nil
}

// Say puts a line in front of the agent WHILE it is working.
//
// This is the one control surface. `meet say`, `weave say`, `foreman tell` and an
// operator at a keyboard all land here, so they cannot drift into meaning
// different things.
func (s *Session) Say(text string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("chat: nothing to say")
	}
	if s.CtlSock == "" {
		return fmt.Errorf("chat: %s has no control channel", s.Nick)
	}
	return agentctl.Say(s.CtlSock, text)
}

// Output is everything the agent has said so far. Safe to call while it is still
// talking.
func (s *Session) Output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// Turn marks the current end of the transcript and returns everything written
// since the last mark. Say + WaitIdle + Turn is how a caller that needs a REPLY
// (a foreman recording history, a chair writing minutes) gets one out of a stream
// that has no turn boundaries in it.
func (s *Session) Turn() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := s.buf.String()
	if s.mark > len(all) {
		s.mark = len(all)
	}
	out := all[s.mark:]
	s.mark = len(all)
	return out
}

// WaitIdle blocks until the agent has written nothing for `quiet`, or until it
// exits, or until ctx ends.
//
// # This is a heuristic and it is named like one
//
// A headless one-shot has a real turn boundary: the process exits. An interactive
// session does not — it just stops typing. Silence is the ONLY signal the terminal
// gives us, so silence is what we use, and callers should size `quiet` to the
// slowest thing the agent might legitimately pause for (a tool call, a model
// re-prompt) rather than to how long a human would wait.
//
// It is deliberately NOT a claim that the turn is complete: an agent that pauses
// long enough looks finished, and an agent that streams a spinner never does. A
// caller that needs certainty about what the agent DID must read the artifacts it
// left behind, not the shape of its output — which is the fleet-evidence rule,
// and the honest reason the first-party harness (structured events, not a scraped
// terminal) is worth building.
func (s *Session) WaitIdle(ctx context.Context, quiet time.Duration) error {
	if quiet <= 0 {
		quiet = 20 * time.Second
	}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			return nil // it exited; that IS a boundary
		case <-tick.C:
			s.mu.Lock()
			last := s.lastWrite
			s.mu.Unlock()
			if !last.IsZero() && time.Since(last) >= quiet {
				return nil
			}
		}
	}
}

// Live reports whether the agent is still running.
func (s *Session) Live() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

// Wait blocks until the agent exits, and reports how.
func (s *Session) Wait() (int, error) {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.exit, s.err
	}
	if s.killed != "" {
		return s.exit, fmt.Errorf("agent terminated: %s", s.killed)
	}
	return s.exit, nil
}

// Quit asks the agent to leave, rather than killing it.
//
// Asking is not the same as making it go: a tool that ignores the request is
// ended by the caller's timeout or by Close. But an agent given the chance to
// finish writing usually leaves a cleaner tree than one shot mid-edit.
func (s *Session) Quit() error { return s.Say("/quit") }

// Close ends the session now.
func (s *Session) Close() {
	select {
	case <-s.done:
	default:
		_ = s.Quit()
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
		}
	}
	_ = os.Remove(s.CtlSock)
}

// waitReady blocks until the control socket exists, so the first message is not
// sent into a socket agentpty has not bound yet — which would silently vanish.
func (s *Session) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(s.CtlSock); err == nil {
			// The socket exists; give the TUI a moment to draw its input box before
			// typing into it.
			time.Sleep(1500 * time.Millisecond)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			return fmt.Errorf("chat: %s exited before its session was ready", s.Nick)
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("chat: %s never opened a control channel", s.Nick)
}

type sessionWriter struct{ s *Session }

func (w *sessionWriter) Write(p []byte) (int, error) {
	w.s.mu.Lock()
	w.s.buf.Write(p)
	w.s.lastWrite = time.Now()
	w.s.mu.Unlock()
	return len(p), nil
}

// sessionSock is short by construction. A unix socket address caps at ~104 bytes,
// and a workspace path plus a binding blows straight past it — at which point
// agentpty degrades to a polling file channel and steering silently gets worse.
func sessionSock(binding, cwd string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".bashy", "ctl")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, shortHash(binding+"\x00"+cwd+"\x00"+fmt.Sprint(os.Getpid()))+".sock"), nil
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

// CanSteer reports whether an agent has an interactive launch, and if not, why.
//
// Callers use this to degrade LOUDLY. A conductor that silently falls back to
// replaying the conversation into a fresh one-shot — which is what `foreman tell`
// used to do — looks exactly like one that steered, and the operator has no way to
// tell that their mid-turn correction actually arrived after the turn was over.
func CanSteer(agent string) (bool, string) {
	if !agentpty.Supported() {
		return false, "this platform has no pty support"
	}
	name, err := ResolveAgent(agent, "")
	if err != nil {
		return false, err.Error()
	}
	if _, err := resolveLaunch(name, Options{Steer: true, ReadOnly: true}); err != nil {
		return false, err.Error()
	}
	return true, ""
}
