//go:build !windows

package chat

import (
	"os/exec"
	"syscall"
)

// Killing a wedged agent means killing a TREE, not a process.
//
// The pipe runner used exec.CommandContext, whose cancellation kills exactly one
// pid: the agent CLI. But every agent CLI bashy drives spawns children — a shell
// shim, an MCP server, a language server — and those children INHERIT the write
// end of the stdout/stderr pipes. So the direct child dies, the grandchildren
// live, and two things go wrong at once: Wait blocks until the last pipe writer
// closes (WaitDelay bounds that, but only by abandoning the pipes), and the
// grandchildren keep running forever as orphans. The 2026-07-18 meeting artifact
// shows both — a turn stranded past its 20m budget, and descendants surviving a
// cancellation.
//
// The fix is to give the child its own process group at launch and signal the
// GROUP. agentpty already does the equivalent for the terminal path (see
// agentpty.Run's killTree); this is the pipe path's half, deliberately kept to
// process-group signalling so it stays pure-Go syscall with no `ps` snapshot.

// setProcessGroup puts the child in a new process group whose id equals its pid,
// so a later kill(-pid) reaches every descendant that has not deliberately
// setpgid'd itself away.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessTree signals the child's whole process group.
//
// It sends SIGKILL rather than SIGTERM: this is only ever reached after the
// per-turn budget expired or the caller cancelled, both of which already mean
// "this agent has had its chance". A graceful window here would just be more
// wall-clock spent on a process that is by definition not responding.
//
// Falls back to killing the bare pid when the group is unavailable (the child
// raced us and exited, or Setpgid did not take), so the caller never ends up
// with no kill at all.
func killProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pid := cmd.Process.Pid
	if err := syscall.Kill(-pid, syscall.SIGKILL); err == nil {
		return nil
	}
	return cmd.Process.Kill()
}
