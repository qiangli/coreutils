package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/room"
)

// newCoachAttachCmd is the `coach attach` subcommand: point a coach at a session
// that is ALREADY RUNNING, instead of only at one the coach launched itself.
//
// It resolves the target through the SAME resolver `chat steer` uses (findMember
// → room.Find: full id or unambiguous prefix/nick), so an operator does not learn
// a second resolution rule. Then it runs the shared attachCoach, which tails the
// member's output (pty mode) or structured events (event mode) and steers through
// the member's control socket using the ONE trip implementation a launched coach
// uses.
//
// `--list` prints the attachable members (those with a control socket) so an
// operator can discover the id before attaching.
func newCoachAttachCmd() *cobra.Command {
	var (
		list      bool
		repeat    int
		ratio     float64
		maxSteers int
		readOnly  bool
		logPath   string
		timeout   time.Duration
	)
	cmd := &cobra.Command{
		Use:   "attach <session>",
		Short: "coach a session that is ALREADY RUNNING (attach; never launches or kills it)",
		Long: "Coach attach points the LLM-free auto-coach at a session already in flight — " +
			"the member is resolved from the host room, watched through its capture (pty mode: " +
			"the generic loop-from-output signal, works for any tool) or its structured event " +
			"channel (event mode: precise tool.call tracking, when the member reports one), and " +
			"steered through its control socket. A report channel only: it presses ESC and says a " +
			"sentence, never writes. Detach (--timeout or Ctrl-C) leaves the coachee running.\n\n" +
			"Resolve the target the same way `chat steer` does: the full instance id or an " +
			"unambiguous prefix. `coach attach --list` shows the attachable members.",
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			if list {
				return listAttachable(cmd)
			}
			if len(args) < 1 {
				return fmt.Errorf("coach attach: name a session id — `coach attach --list` shows attachable members")
			}
			card, err := findMember(args[0])
			if err != nil {
				return err
			}
			pol := DefaultCoachPolicy()
			if repeat > 0 {
				pol.RepeatThreshold = repeat
			}
			if ratio > 0 {
				pol.RatioThreshold = ratio
			}
			if maxSteers > 0 {
				pol.MaxSteers = maxSteers
			}
			pol.LogPath = logPath

			// --read-only makes the coach a DETECT-ONLY report: it watches and
			// records trips but sends nothing to the coachee. A coach is already a
			// report channel, never an author; read-only drops even ESC+Say, for an
			// operator who wants to observe with zero chance of intervention.
			if readOnly {
				pol.Interrupt = false
			}

			base := cmd.Context()
			var ctx context.Context
			var cancel context.CancelFunc
			if timeout > 0 {
				ctx, cancel = context.WithTimeout(base, timeout)
			} else {
				ctx, cancel = context.WithCancel(base)
			}
			defer cancel()

			errOut := cmd.ErrOrStderr()
			fmt.Fprintf(errOut, "coach: attached to %s (%s) — Ctrl-C / --timeout detaches; the agent keeps running\n", card.ID, card.Binding)

			coach, err := attachCoach(ctx, card, pol, readOnly)
			if err != nil {
				return err
			}

			// Block until the watch ends. Waiting on the coach (not ctx.Done) means a
			// coachee that exits on its own — pid gone — ends the watch too, so an
			// attach with no --timeout returns when the agent does instead of hanging.
			// The watcher exits on --timeout, Ctrl-C (ctx cancelled), or the coachee's
			// pid going away; the coachee is never killed by this side either way.
			coach.Wait()
			rep := coach.Report()

			fmt.Fprintf(errOut, "\n── coach report (%s, %s) ──\n", card.ID, coach.Mode())
			if coach.Mode() == "events" {
				fmt.Fprintf(errOut, "tool calls: %d total / %d distinct (repeat %.2f)\n", rep.Total, rep.Distinct, rep.Repeat)
			} else {
				fmt.Fprintf(errOut, "output lines: %d total / %d distinct (repeat %.2f)\n", rep.Total, rep.Distinct, rep.Repeat)
			}
			fmt.Fprintf(errOut, "steers: %d\n", len(rep.Steers))
			for i, s := range rep.Steers {
				fmt.Fprintf(errOut, "  %d. [%s] at repeat=%.2f: %q\n", i+1, s.Reason, s.Repeat, s.Steer)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&list, "list", false, "list attachable sessions (room members with a control socket) and exit")
	f.IntVar(&repeat, "repeat", 0, "trip when one identical call/line is issued this many times (default 3)")
	f.Float64Var(&ratio, "ratio", 0, "trip when total/distinct reach this (default 3.0)")
	f.IntVar(&maxSteers, "max-steers", 0, "hard cap on interventions (default 3)")
	f.BoolVar(&readOnly, "read-only", false, "detect-only: watch and report, send nothing to the coachee")
	f.StringVar(&logPath, "log", "", "append one JSON line per steer here (the training record)")
	f.DurationVar(&timeout, "timeout", 0, "bound the watch; detaches when it elapses (the agent keeps running)")
	return cmd
}

// listAttachable prints the room members a coach can attach to — the live,
// steerable ones (a non-empty control socket is what makes a member attachable:
// it is the only surface a coach steers through).
func listAttachable(cmd *cobra.Command) error {
	members, err := room.Members()
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	var attachable []room.Card
	for _, c := range members {
		if c.CtlSock != "" {
			attachable = append(attachable, c)
		}
	}
	if len(attachable) == 0 {
		fmt.Fprintln(w, "no attachable sessions — a member needs a control socket to be coached")
		return nil
	}
	fmt.Fprintf(w, "%-24s %-22s %-8s %-7s %s\n", "ID", "BINDING", "EVENTS", "PID", "TASK")
	for _, c := range attachable {
		ev := "pty"
		if c.Events {
			ev = "events"
		}
		fmt.Fprintf(w, "%-24s %-22s %-8s %-7d %s\n", c.ID, c.Binding, ev, c.PID, c.Task)
	}
	return nil
}
