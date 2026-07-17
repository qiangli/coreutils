package weave

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"golang.org/x/term"

	"github.com/qiangli/coreutils/pkg/agentpty"
	"github.com/qiangli/coreutils/pkg/chat"
)

// The PTY runner used to live here. It moved to pkg/agentpty because `chat`
// needs it too — a meeting participant that hangs on a trust prompt is exactly
// as stuck as a weave worker — and weave already imports chat, so chat could not
// import weave without a cycle.
//
// What is left is the weave-shaped half: weave's guards, and weave's opinion
// about what a worker log should look like.

// runWeaveToolPTY launches a subagent under a PTY with weave's watchdogs and
// control socket. Kept as weave's name for it so the call sites — and the tests
// that gate this move — read exactly as they did before.
func runWeaveToolPTY(cmd *exec.Cmd, logSink io.Writer, guards weaveGuards) (int, string, error) {
	// The reflex coach (P2a): attach the LLM-free loop detector to every run by
	// default. It tees the run's DECODED prose (below) into a pty-novelty
	// detector and, when a run churns without progress, ESC+Says it off the loop
	// through the same control socket `weave attach` uses. Off with BASHY_NO_COACH;
	// a no-op when there is no control socket to steer through.
	sink := logSink
	var coach *chat.Coach
	if guards.ctlSock != "" && chat.ReflexEnabled() {
		coach = chat.NewLineCoach(chat.DefaultCoachPolicy(), chat.NewCtlSteerer(guards.ctlSock))
		sink = io.MultiWriter(logSink, coach)
	}

	code, reason, err := agentpty.Run(cmd, sink, agentpty.Options{
		IdleTimeout:   guards.idleTimeout,
		MaxRuntime:    guards.maxRuntime,
		MemLimitBytes: guards.memLimitBytes,
		CtlSock:       guards.ctlSock,

		// weave's worker log is read by humans and by `weave wait --broker`, so
		// an agent's stream-json is decoded into prose rather than dumped raw.
		// A meeting wants the opposite — the raw lines, exactly as written — and
		// that disagreement is why the filter is injected rather than baked in.
		Filter: func(w io.Writer) (io.Writer, func() error) {
			sj := newWeaveStreamJSONLogWriter(w)
			return sj, sj.Flush
		},
	})

	if coach != nil {
		if rep := coach.Report(); len(rep.Steers) > 0 {
			fmt.Fprintf(logSink, "\n[coach] steered this run %d time(s) off a suspected loop (%d output lines, %d distinct)\n",
				len(rep.Steers), rep.Total, rep.Distinct)
		}
	}
	return code, reason, err
}

// weaveStdinIsTTY reports whether the calling process's stdin is a real
// terminal. Used to gate the auto-setsid + auto-log-file paths.
func weaveStdinIsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
