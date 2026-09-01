package weave

// The lifecycle surface gives the common conductor transition one verb per
// intent while preserving the older, lower-level primitives:
//
//   pause  = write continuity and release the seat; workers keep running
//   resume = claim the seat and print continuity in the same operation
//   end    = strict stop + done column + released seat (implemented beside stop)
//
// pause/resume intentionally do not touch weave workers. A conductor switch
// is not an execution-state change; linked runs survive independently. `end`
// is the operation that drains and audits them.

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newSprintPauseCmd() *cobra.Command {
	var flags weaveOutputFlags
	var message string
	cmd := &cobra.Command{
		Use:   "pause <sprint>",
		Short: "Checkpoint continuity and release the conductor; workers continue",
		Long: `pause is the routine conductor transition. It requires a resume brief,
records it on the sprint, closes the current conductor room, and releases the
lease. Linked weave workers are deliberately untouched: changing who conducts
the work must not kill, suspend, or restart the work itself.

Use end when the sprint itself is finished and all linked work must be drained
and audited.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			message = strings.TrimSpace(message)
			if message == "" {
				return fmt.Errorf("-m <resume brief> required")
			}
			return runWeaveStoryMutate(cmd, id, "sprint pause", &flags, func(s *weaveStory) (string, error) {
				prev, stale, free := weaveStoryLeaseState(s)
				if free {
					return "", fmt.Errorf("sprint #%d lease is unclaimed — there is no conductor to pause", id)
				}
				if stale {
					return "", fmt.Errorf("sprint #%d lease is STALE (was %s) — take it explicitly before pausing", id, prev)
				}
				// A conductor identity is stored on the lease, not in a durable
				// process environment: resume and pause are normally separate CLI
				// invocations. Act as the current fresh holder rather than falling
				// back to the generic "conductor" identity on the second command.
				who := prev
				// The successor must inherit the TRUTH about the tree, not just
				// the plan. Anything unclean, unreachable or unplanned is folded
				// into the brief `sprint resume` prints, so nobody starts by
				// rediscovering a dirty repo and blaming their own first command.
				s.Continuity = message + sprintStateAddendum(s)
				// THE ROOM BELONGS TO THE SPRINT, NOT THE CONDUCTOR. Pausing
				// releases the lease; it does not end the sprint. Closing here
				// left an OPEN card with a running box and nowhere to reach it
				// during exactly the window somebody needs to ask a question.
				roomNote := "; room stays open"
				if !sprintRoomRetained(s) {
					_ = closeSprintRoom(s, who)
					roomNote = ""
				}
				weaveStoryAppend(s, who, "system", "paused conductor session — continuity recorded; linked workers left running"+roomNote)
				s.Lease = nil
				return fmt.Sprintf("sprint #%d paused — lease released; linked workers unchanged", id), nil
			})
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "required resume brief for the next conductor")
	flags.attach(cmd)
	return cmd
}

func newSprintResumeCmd() *cobra.Command {
	var flags weaveOutputFlags
	var as string
	var force bool
	cmd := &cobra.Command{
		Use:   "resume <sprint>",
		Short: "Claim the conductor lease and display the continuity brief",
		Long: `resume is the pickup half of sprint pause. It claims a free or stale
conductor lease, opens the sprint room when the sprint is on the clock, and
prints the saved continuity brief in the same operation. Linked weave workers
are not relaunched because pause never stopped them.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			return runWeaveStoryMutate(cmd, id, "sprint resume", &flags, func(s *weaveStory) (string, error) {
				who := sprintTakeoverIdentity(s, as)
				prev, stale, free := weaveStoryLeaseState(s)
				if !free && !stale && prev != who && !force {
					return "", fmt.Errorf("sprint #%d lease is held by %s (fresh) — coordinate, or --force to resume", id, prev)
				}
				// AN OWNER MUST BE AN ADDRESS. Refuse a name that resolves to
				// no agent: every coordination surface (mb, chat, inbox, ping)
				// keys on this string, and one that names nobody is worse than
				// none — it is printed as a contact and silently never answers.
				if err := validateSprintOwner(who); err != nil {
					return "", err
				}
				s.Lease = &weaveStoryLease{Holder: who, At: time.Now().UTC()}
				s.Owner = who
				// Reuse the room the sprint already has; open one only when it
				// has none. The transcript across a pause IS the handover.
				if s.currentBox().Running() || sprintColumnOpen(s.Column) {
					ensureSprintRoom(s, who)
				}
				weaveStoryAppend(s, who, "system", "resumed conductor session; continuity loaded")
				brief := strings.TrimSpace(s.Continuity)
				if brief == "" {
					brief = "(no continuity brief recorded)"
				}
				return fmt.Sprintf("sprint #%d resumed by %s\ncoordination: use %s for mb/Meet/chat/ping; %s\ncontinuity: %s", id, who, who, sprintReadyLine(who), brief), nil
			})
		},
	}
	cmd.Flags().StringVar(&as, "as", "", "conductor name (default $WEAVE_CONDUCTOR/$WEAVE_AGENT)")
	cmd.Flags().BoolVar(&force, "force", false, "take over a fresh lease")
	flags.attach(cmd)
	return cmd
}
