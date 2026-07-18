package chat

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/agentctl"
	"github.com/qiangli/coreutils/pkg/agentpty"
	"github.com/qiangli/coreutils/pkg/room"
)

// readOpeningLine reads one line from f byte-by-byte (no read-ahead), so the raw
// pty passthrough that follows does not lose the operator's subsequent keystrokes.
func readOpeningLine(f *os.File) string {
	var b []byte
	buf := make([]byte, 1)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			if buf[0] != '\r' {
				b = append(b, buf[0])
			}
		}
		if err != nil {
			break
		}
	}
	return strings.TrimSpace(string(b))
}

// principalName is the human/agent this session is attributed to on its room card.
// Optional (best-effort from the environment) — the card omits it when unknown.
func principalName() string {
	for _, k := range []string{"BASHY_PRINCIPAL", "USER", "LOGNAME"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// InteractOptions configures a foreground, human-driven session.
type InteractOptions struct {
	// Prompt optionally opens the conversation. A tool whose steerable launch
	// takes a prompt on the command line gets it there; one that opens an empty
	// session is left at its prompt for the human to type into.
	Prompt   string
	Cwd      string
	Timeout  time.Duration
	ReadOnly bool
	// Unattended (--yolo) keeps the agent's approval gate OFF for a session
	// supervised remotely via steer — no one sits at the terminal to approve.
	Unattended bool
	// Status receives bashy's own one-line notices (which agent, the session id);
	// defaults to os.Stderr so they never contaminate the tool's stdout.
	Status io.Writer
}

// nativeHarnesses are first-party tools that already speak bashy's event channel
// and governance directly — wrapping them in a chat session adds nothing. Kept as
// a set so it is one obvious place to extend if another first-party harness lands.
var nativeHarnesses = map[string]bool{"ycode": true}

// Interact launches an agent's NATIVE interactive session in the foreground, under
// full bashy governance, and registers it so the fleet can reach it.
//
// It is the foreground twin of Start: the SAME resolveLaunch + agentChildEnv, so
// governance (vault-secret scrub, single declared API key, shell forced to bashy,
// principal identity) never drifts between a programmatic session and a human one.
// The only difference is that agentpty.Run is called in the FOREGROUND with
// Capture:false, so the human's terminal IS the session and the tool's own TUI is
// what they see — identical to running the tool directly, but with the selected
// model, governed, and addressable (steer / interrupt / attach / observe).
func Interact(ctx context.Context, agent string, opt InteractOptions) (int, error) {
	status := opt.Status
	if status == nil {
		status = os.Stderr
	}

	if !agentpty.Supported() {
		return 1, fmt.Errorf("chat: an interactive session needs a pty, which this platform has no support for")
	}
	name, err := ResolveAgent(agent, "")
	if err != nil {
		return 1, err
	}
	// Deliberately the SAME resolver and launch Start/Invoke use — a session
	// differs only in which launch template it renders, and governance must not
	// be able to drift. Attended: a human is driving, so the tool's own approval
	// gate stays ON (the auto-approve kill-switches are stripped) — safer than an
	// unattended fleet launch, and it passes the uncontained-host guard exactly as
	// running the tool by hand would. ReadOnly, when set, is stricter and wins.
	l, err := resolveLaunch(name, Options{
		Cwd:         opt.Cwd,
		ReadOnly:    opt.ReadOnly,
		Attended:    !opt.ReadOnly,
		AllowUnsafe: opt.Unattended,
		Steer:       true,
	})
	if err != nil {
		return 1, err
	}

	// ycode (and any first-party harness) applies its OWN governance, so bashy does
	// not scrub its env or force a single key — that is the only difference (native
	// env, below). It still routes through the SAME pty + control socket + host-room
	// membership as every other agent, because that is the whole point of
	// `bashy chat`: an instance launched this way is DISCOVERABLE and STEERABLE.
	// (An earlier version launched ycode with inherited stdio and no control channel
	// — a running session was then invisible on `chat sessions` and impossible to
	// steer. This closes that gap; the transparent pty passthrough keeps the UX
	// identical to `ycode --model x`.)
	native := nativeHarnesses[l.ToolName]

	argv := append([]string{}, l.Args...)
	if l.TakesPrompt {
		prompt := strings.TrimSpace(opt.Prompt)
		if prompt == "" {
			// This tool bakes the opening prompt into its interactive launch (agy's
			// `-i <prompt>`), so it CANNOT open an empty session — an empty prompt
			// dangles the flag ("flag needs an argument: -i"). Read the operator's
			// first line and seed the session with it; the tool continues from there.
			fmt.Fprintf(status, "chat: %s opens with an initial message — type it, then Enter:\n> ", l.Nick)
			prompt = readOpeningLine(os.Stdin)
			if prompt == "" {
				return 1, fmt.Errorf("chat: %s cannot open an empty session; give an opening message "+
					"(next time: `bashy chat --agent %s -m \"...\"`)", l.Nick, l.Nick)
			}
		}
		argv = append(argv, prompt)
	}

	cwd := opt.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	sock, err := sessionSock(l.Binding(), cwd)
	if err != nil {
		return 1, err
	}

	// A capture log ALONGSIDE the native TUI is what makes a live human session
	// observable/attachable. Best-effort: if it cannot be opened the session still
	// runs (native UX intact), it just is not followable.
	logPath, logSink, closeLog := sessionLog(l.Binding(), cwd)
	if closeLog != nil {
		defer closeLog()
	}

	cmd := exec.CommandContext(ctx, l.Tool, argv...)
	cmd.Dir = cwd
	if native {
		// Native env: a first-party harness uses the operator's real env + its own
		// key/config, so it is truly "identical to `ycode --model x`" for auth. bashy
		// adds only the control channel + membership, never a governance override.
		cmd.Env = os.Environ()
	} else {
		// agentChildEnv is the one choke point that scrubs the operator's vault
		// secrets, grants back only this model's one API key, forces the child's
		// shell to bashy, and stamps its principal identity. Built anywhere else, all
		// four silently drop.
		cmd.Env = agentChildEnv(withLaunch(ctx, l))
	}
	if p, ok := agentctl.ProfileFor(l.ToolName); ok && p.Preseed != "" {
		_ = agentctl.ApplyTrustPreseed(cmd.Dir, p.Preseed)
	}

	card := room.Card{
		ID:        sessionID(l),
		Principal: principalName(),
		Tool:      l.ToolName,
		Model:     l.ModelName,
		Binding:   l.Binding(),
		Nick:      l.Nick,
		Band:      bindingBand(name),
		Mode:      "interactive",
		CtlSock:   sock,
		LogPath:   logPath,
		PID:       os.Getpid(),
		Cwd:       cwd,
		Native:    native,
	}
	_ = room.Join(card)
	defer room.Leave(card.ID)

	posture := "governed + steerable"
	if native {
		posture = "native harness (self-governing) + steerable"
	}
	fmt.Fprintf(status, "chat: %s (%s) — %s (id %s). Ctrl-C ends it; "+
		"from another terminal: `bashy chat steer %s \"...\"` / `attach %s`.\n",
		card.Nick, card.Binding, posture, card.ID, card.ID, card.ID)

	// Foreground + parent-is-a-TTY + Capture:false → agentpty gives native raw-mode
	// passthrough (the tool's own TUI), teeing to logSink for observers.
	exit, killed, runErr := agentpty.Run(cmd, logSink, agentpty.Options{
		CtlSock:    sock,
		Capture:    false,
		MaxRuntime: opt.Timeout,
	})
	if killed != "" {
		fmt.Fprintf(status, "chat: session ended (%s)\n", killed)
	}
	return exit, runErr
}

// sessionLog opens a per-session capture file. On any error it degrades to no
// capture (nil sink) rather than failing the launch — an unobservable session is
// worse than none only if it also refuses to start.
func sessionLog(binding, cwd string) (path string, sink io.Writer, closeFn func()) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil, nil
	}
	dir := filepath.Join(home, ".bashy", "sessions", "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, nil
	}
	path = filepath.Join(dir, shortHash(binding+"\x00"+cwd+"\x00"+fmt.Sprint(os.Getpid()))+".log")
	f, err := os.Create(path)
	if err != nil {
		return "", nil, nil
	}
	return path, f, func() { _ = f.Close() }
}

// bindingBand is the capability band of a resolved agent name, or 0 when the name
// is a bare tool that pegs to no model. Best-effort decoration for `chat sessions`.
func bindingBand(name string) int {
	if _, _, m, err := newCatalog().Binding(name); err == nil {
		return m.Band
	}
	return 0
}
