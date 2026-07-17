package chat

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/agentpty"
)

// newChatSessionsCmd lists the live governed sessions — the live-agent board.
func newChatSessionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sessions",
		Aliases: []string{"ls"},
		Short:   "list live governed agent sessions launched by `bashy chat`",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sessions, err := listSessions()
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if len(sessions) == 0 {
				fmt.Fprintln(w, "no live sessions — start one with `bashy chat --agent NICK` or `--band N`")
				return nil
			}
			fmt.Fprintf(w, "%-20s %-22s %-4s %-8s %s\n", "ID", "BINDING", "BAND", "PID", "STARTED")
			for _, s := range sessions {
				band := "-"
				if s.Band > 0 {
					band = "L" + fmt.Sprint(s.Band)
				}
				fmt.Fprintf(w, "%-20s %-22s %-4s %-8d %s\n", s.ID, s.Binding, band, s.PID, s.Started)
			}
			return nil
		},
	}
	return cmd
}

// newChatSteerCmd injects a line into a running session mid-turn.
func newChatSteerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "steer <id> <text>",
		Short: "inject a line into a running session (the one control surface: mid-turn steering)",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := findSession(args[0])
			if err != nil {
				return err
			}
			text := strings.Join(args[1:], " ")
			if err := agentpty.SendFrame(s.CtlSock, agentpty.TextFrame(text)); err != nil {
				return fmt.Errorf("chat: could not steer %s: %w", s.ID, err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "chat: steered %s\n", s.ID)
			return nil
		},
	}
	return cmd
}

// newChatInterruptCmd sends ESC — the only thing that breaks a tool loop mid-turn.
func newChatInterruptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "interrupt <id>",
		Short: "send ESC to a running session — breaks a tool loop a queued line cannot reach",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) > 0 {
				id = args[0]
			}
			s, err := findSession(id)
			if err != nil {
				return err
			}
			if err := agentpty.SendFrame(s.CtlSock, agentpty.VerbatimFrame([]byte{0x1b})); err != nil {
				return fmt.Errorf("chat: could not interrupt %s: %w", s.ID, err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "chat: sent ESC to %s\n", s.ID)
			return nil
		},
	}
	return cmd
}

// newChatAttachCmd follows a session's capture and forwards typed lines as steers —
// the `weave attach` pattern over any registered session, so a SECOND party can
// watch and instruct a session someone else (or a coach) is driving.
func newChatAttachCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach <id>",
		Short: "watch and steer a running session (type to instruct, /detach to leave)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) > 0 {
				id = args[0]
			}
			s, err := findSession(id)
			if err != nil {
				return err
			}
			if s.LogPath == "" {
				return fmt.Errorf("chat: session %s has no capture to follow (it may have launched without a log)", s.ID)
			}
			return attachSession(cmd, s)
		},
	}
	return cmd
}

func attachSession(cmd *cobra.Command, s LiveSession) error {
	f, err := os.Open(s.LogPath)
	if err != nil {
		return fmt.Errorf("chat: capture missing on disk: %s", s.LogPath)
	}
	defer f.Close()

	errOut := cmd.ErrOrStderr()
	fmt.Fprintf(errOut, "attached to %s (%s) — type to instruct, /detach to leave (the agent keeps running)\n",
		s.ID, s.Binding)

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Follower: dump what's there, then poll for growth. Raw ANSI, so a native-TUI
	// capture looks like a redraw stream — good enough to see activity and steer,
	// not a clean transcript (that is what invoke's headless capture is for).
	go func() {
		out := cmd.OutOrStdout()
		_, _ = io.Copy(out, f)
		buf := make([]byte, 4096)
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(200 * time.Millisecond):
			}
			if !pidAlive(s.PID) {
				fmt.Fprintln(errOut, "\nchat: session ended")
				cancel()
				return
			}
			for {
				n, rerr := f.Read(buf)
				if n > 0 {
					_, _ = out.Write(buf[:n])
				}
				if rerr != nil || n == 0 {
					break
				}
			}
		}
	}()

	scanner := bufio.NewScanner(cmd.InOrStdin())
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		line := scanner.Text()
		switch strings.TrimSpace(line) {
		case "/detach", "/quit":
			fmt.Fprintln(errOut, "detached (the agent keeps running)")
			return nil
		default:
			if err := agentpty.SendFrame(s.CtlSock, agentpty.TextFrame(line)); err != nil {
				return fmt.Errorf("chat: steer failed: %w", err)
			}
		}
	}
	return scanner.Err()
}
