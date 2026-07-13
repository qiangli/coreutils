package weave

import (
	"io"
	"os"
	"os/exec"

	"golang.org/x/term"

	"github.com/qiangli/coreutils/pkg/agentpty"
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
	return agentpty.Run(cmd, logSink, agentpty.Options{
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
}

// weaveStdinIsTTY reports whether the calling process's stdin is a real
// terminal. Used to gate the auto-setsid + auto-log-file paths.
func weaveStdinIsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
