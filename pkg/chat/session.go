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

	// events is the tool's structured channel, when it has one. Its presence is
	// the difference between KNOWING a turn ended and guessing from silence.
	events *eventTail

	mu         sync.Mutex
	buf        strings.Builder
	mark       int       // end of the last Turn
	reported   string    // the answer the tool ASSERTED on its event channel
	eventsOnce sync.Once // the silent-degradation warning fires once per session, not per turn
	lastWrite  time.Time
	done       chan struct{}
	exit       int
	err        error
	killed     string
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

	// If the tool can TELL us when a turn ends, let it. See pkg/chat/events.go:
	// everything else here infers a turn boundary from 25 seconds of silence,
	// which is wrong for an agent that pauses to think and wrong for one that
	// renders a spinner.
	var tail *eventTail
	if tl, ok := newCatalog().Tool(l.ToolName); ok && tl.ReportsTurnEnd() {
		evPath, err := sessionEventsPath(l.Binding())
		if err == nil {
			if extra := tl.EventsArgv(evPath); len(extra) > 0 {
				argv = append(argv, extra...)
				tail = &eventTail{path: evPath}
			}
		}
	}

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
		events:  tail,
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
		if err := s.openConversation(ctx, opt.Prompt); err != nil {
			return s, err
		}
	}
	return s, nil
}

// openConversation sends the first message and CONFIRMS it arrived.
//
// It does not send and hope. A TUI that has not finished starting silently
// swallows whatever you type at it, and the resulting session sits at an empty
// prompt forever — which looks, from every angle, like a model that had nothing to
// say. Observed exactly that: an opencode conductor idled at its splash screen with
// `calls=0`, and the obvious conclusion ("deepseek did nothing") would have been
// wrong, and would have been recorded as a fact about the model.
//
// So: send, then look for the agent to actually start writing. If it does not,
// send once more. Confirmation is cheap; a false verdict about a model is not.
func (s *Session) openConversation(ctx context.Context, prompt string) error {
	for attempt := 1; attempt <= 2; attempt++ {
		s.mu.Lock()
		before := s.buf.Len()
		s.mu.Unlock()

		if err := s.Say(prompt); err != nil {
			return fmt.Errorf("chat: could not open the conversation with %s: %w", s.Nick, err)
		}

		// Did it take? An agent that received a prompt starts producing output.
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-s.done:
				return fmt.Errorf("chat: %s exited before it could be asked anything", s.Nick)
			case <-time.After(500 * time.Millisecond):
			}
			s.mu.Lock()
			grew := s.buf.Len() - before
			s.mu.Unlock()
			// A TUI redraws its own input box as the text is typed, so a few bytes
			// prove nothing. Real work moves considerably more than that.
			if grew > openAckBytes {
				return nil
			}
		}
	}
	return fmt.Errorf("chat: %s never acknowledged the opening prompt — it accepted the keystrokes "+
		"and produced nothing. Its TUI was most likely still starting up. This is a HARNESS failure, "+
		"not a model failure: do not record it as one", s.Nick)
}

// openAckBytes is how much output proves the agent actually took the prompt,
// rather than merely echoing it into its input box.
const openAckBytes = 400

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

	// PREFER WHAT THE AGENT SAID IT SAID.
	//
	// The terminal scrape is a RECONSTRUCTION: a pty merges stdout and stderr, so
	// the captured text carries banners, spinners and cursor gymnastics, and
	// SanitizeTurn spends its life guessing which bytes were the answer. When the
	// tool reports the answer as DATA, the guessing stops -- the answer is known.
	if s.reported != "" {
		out := s.reported
		s.reported = ""
		s.mark = s.buf.Len()
		return out
	}

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

	// RACE THE REPORT AGAINST THE GUESS. Whichever arrives first is the answer.
	//
	// The first version ran them in sequence: wait for turn.end, and on failure
	// fall back to silence. That is wrong in a way a test caught immediately —
	// waiting for the report consumed the ENTIRE context budget, so the fallback
	// inherited an already-expired deadline and could never run. The degradation
	// path was unreachable, which meant a tool that declared an event channel and
	// did not deliver one would hang until the caller's timeout rather than quietly
	// doing the sensible thing.
	//
	// Racing is also just the truth of the situation: we do not know which will
	// happen. A tool that reports gives us a real boundary; a tool that does not
	// goes quiet. Wait for either.
	events := make(chan string, 1)
	if s.events != nil {
		go func() {
			text, ok, err := s.events.WaitTurnEnd(ctx)
			if err == nil && ok {
				events <- text
				return
			}
			close(events)
		}()
	}

	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-s.done:
			return nil // it exited; that IS a boundary

		case text, ok := <-events:
			if ok {
				// THE TOOL TOLD US. No guessing, no quiet period, and the answer
				// arrives as data rather than as bytes scraped off a terminal.
				s.mu.Lock()
				s.reported = text
				s.mu.Unlock()
				return nil
			}
			// The channel closed without a turn.end: we know NOTHING, which is not
			// the same as "the turn ended". Keep waiting on silence — but SAY SO. A
			// capability that quietly does not work is the exact failure this whole
			// line of work exists to stamp out.
			//
			// Known live gap: ycode emits events from its ONE-SHOT path (RunPrompt)
			// and not from its TUI (RunInteractive) — and the TUI is what steer_exec
			// launches, so a steerable ycode session lands here every time. The TUI's
			// events already flow through ycode's internal bus; what is missing is a
			// sink and ONE vocabulary (the bus says `turn.complete`, the one-shot path
			// says `turn.end` — two names for one thing, inside one binary). Until
			// that is fixed this warning fires, and it should: it is telling the truth
			// about a feature that is plumbed and not yet delivered.
			s.eventsOnce.Do(func() {
				fmt.Fprintf(os.Stderr,
					"chat: %s: declared an event channel but reported no turn.end — "+
						"falling back to the SILENCE heuristic (a turn is GUESSED, not reported)\n",
					s.Nick)
			})
			events = nil // stop selecting on a closed channel

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
// waitReady blocks until the agent can actually HEAR us.
//
// Two conditions, and the second one is the one that was missing:
//
//  1. The control socket is bound — otherwise the message goes to an address that
//     does not exist yet and simply vanishes.
//  2. The TUI has DRAWN and gone QUIET. A tool still starting up (opencode loads
//     MCP servers; codex draws a splash) swallows keystrokes without a trace. It
//     is not an error, it is not a rejection, it is nothing at all — the session
//     just sits at an empty prompt forever, looking for all the world like a model
//     with nothing to say.
//
// This used to be `time.Sleep(1500ms)`, which is not a readiness check; it is a
// guess. It happened to be long enough for the tools that take their prompt on
// argv (where it does not matter) and too short for the ones that do not (where it
// is the whole ballgame). Waiting for the output to settle asks the tool itself
// whether it is ready, instead of betting on a number.
func (s *Session) waitReady(ctx context.Context) error {
	// 1. The socket.
	deadline := time.Now().Add(20 * time.Second)
	bound := false
	for time.Now().Before(deadline) {
		if _, err := os.Stat(s.CtlSock); err == nil {
			bound = true
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			return fmt.Errorf("chat: %s exited before its session was ready", s.Nick)
		case <-time.After(200 * time.Millisecond):
		}
	}
	if !bound {
		return fmt.Errorf("chat: %s never opened a control channel", s.Nick)
	}

	// 2. The TUI: it has painted something, and then stopped painting.
	//
	// Bounded, because a tool that renders a spinner never goes quiet at all. When
	// the budget runs out we send anyway — a prompt that might be swallowed beats a
	// session that never starts, and openConversation confirms delivery regardless.
	settle := 1200 * time.Millisecond
	budget := time.Now().Add(25 * time.Second)
	for time.Now().Before(budget) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			return fmt.Errorf("chat: %s exited before its session was ready", s.Nick)
		case <-time.After(200 * time.Millisecond):
		}
		s.mu.Lock()
		drawn, quiet := s.buf.Len() > 0, time.Since(s.lastWrite) >= settle
		s.mu.Unlock()
		if drawn && quiet {
			return nil
		}
	}
	return nil
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

// sessionEventsPath is where a tool streams its structured events for this run.
func sessionEventsPath(binding string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".bashy", "events")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, shortHash(binding+"\x00"+fmt.Sprint(os.Getpid()))+".ndjson"), nil
}
