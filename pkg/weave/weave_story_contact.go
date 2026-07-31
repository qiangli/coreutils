package weave

// REACHING THE CONDUCTOR — a room to talk in, and a bus to make someone notice.
//
// A sprint says who is accountable. It did not say how to reach them, and the
// gap matters as soon as more than one thing is running: the answer to "who
// owns this repo" is useless if the next question is "…and how do I ask them".
//
// Two mechanisms, because they answer different questions and neither
// substitutes for the other:
//
//	meet room   a PLACE to have the conversation, with a durable transcript
//	bus         the INTERRUPT that makes someone look at it
//
// The room is opened by `sprint start`, not by a conductor remembering to. An
// optional room is empty exactly when it is needed — at the moment somebody
// urgent arrives and the conductor is mid-turn on something else.
//
// # Why a file was rejected as the primary
//
// "A file the conductor checks periodically" assumes a liveness the lease
// already tells us is often false: conductors die by SIGKILL and token
// exhaustion, and a file nobody polls is indistinguishable from a file with
// nothing in it. The bus solves the half that is actually hard — its sidecar
// holds a subscription off the agent's critical path and leaves a pre-resolved
// buffer to read at a turn boundary, because an agent mid-turn cannot decide to
// go and check somewhere. Its demote-never-drop rule then guarantees an
// undeliverable interrupt becomes a queued notification with a recorded reason
// rather than silence.
//
// A file is fine BEHIND that as a durable drop-box. It is not a contact method.
//
// # The address is stored, not the mechanism
//
// Contact carries a Kind and a Ref rather than a room number, so the mechanism
// can change without a schema change. And the Ref is meet's room ID, never its
// short room NUMBER: that number is a pointer, released and REUSED when a room
// closes, so a sprint holding one would eventually name somebody else's meeting
// while looking perfectly valid.

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/weavecli"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/meet"
)

// sprintContact is how to reach the conductor responsible for a sprint.
type sprintContact struct {
	// Kind is the mechanism ("meet"), so a later one can be added without
	// reinterpreting Ref.
	Kind string `json:"kind"`
	// Ref is meet's room ID — the identity, never the reusable room number.
	Ref string `json:"ref"`
	// Room is the short number, carried only for display. It is a POINTER and
	// may be stale; nothing resolves against it.
	Room int `json:"room,omitempty"`
	// Topic is the bus topic that pings this sprint's conductor.
	Topic string `json:"topic,omitempty"`
}

// Ref returns the room identity, empty when there is no contact.
func (c *sprintContact) RefID() string {
	if c == nil {
		return ""
	}
	return c.Ref
}

// String renders the contact for a human choosing how to reach someone.
func (c *sprintContact) String() string {
	if c == nil || c.Ref == "" {
		return ""
	}
	if c.Room > 0 {
		return fmt.Sprintf("meet #%d · bus %s", c.Room, c.Topic)
	}
	return fmt.Sprintf("meet %s · bus %s", c.Ref, c.Topic)
}

// sprintTopic is the bus topic for one sprint. Derived rather than stored so it
// cannot drift from the sprint it names.
func sprintTopic(id int64) string { return fmt.Sprintf("sprint.%d", id) }

// openSprintRoom convenes the room a running sprint is reachable in.
//
// Failure is NOT fatal to starting the sprint. A conductor who cannot open a
// room still has a box to run, and refusing to start work because the intercom
// is down would be the wrong trade — the sprint simply records no contact, and
// the surfaces that show contact say so rather than implying one exists.
func openSprintRoom(s *weaveStory, conductor string) (*sprintContact, error) {
	st, err := meet.Create(meet.CreateOptions{
		Topic: fmt.Sprintf("sprint #%d: %s", s.ID, s.Title),
		// The conductor is a PARTICIPANT, not the chair. A chair is required to
		// have someone to call on, and this room has no roster at open time —
		// the whole point is that it exists BEFORE anyone needs it. Whoever
		// arrives joins; the conductor is simply the one guaranteed to be there.
		Participants: []string{conductor},
		Initiator:    conductor,
		Agenda:       []string{"delivery of this sprint", "blockers", "handoff"},
	})
	if err != nil {
		return nil, err
	}
	if st == nil || st.ID == "" {
		return nil, fmt.Errorf("meet returned no room")
	}
	return &sprintContact{Kind: "meet", Ref: st.ID, Room: st.Room, Topic: sprintTopic(s.ID)}, nil
}

// pingSprintConductor publishes an interrupt to whoever is conducting.
//
// It addresses the sprint's TOPIC rather than the conductor's name on purpose:
// the holder changes across a handoff or a take, and a message addressed to a
// person who has since handed off would be delivered to somebody with no
// context — or to nobody. The topic follows the responsibility.
func pingSprintConductor(s *weaveStory, from, body, priority string) error {
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("nothing to send")
	}
	holder, _, free := weaveStoryLeaseState(s)
	if free {
		return fmt.Errorf("sprint #%d has no conductor to ping — `sprint take %d` to become one", s.ID, s.ID)
	}
	n := bus.Notification{
		Principal: from,
		Topic:     sprintTopic(s.ID),
		To:        holder,
		Body:      body,
		Priority:  priority,
	}
	if s.Contact != nil {
		n.Room = s.Contact.Ref
	}
	return bus.Publish(n)
}

// currentActor is who is running the command, for authorship on a ping.
func currentActor() string {
	if u := strings.TrimSpace(os.Getenv("WEAVE_CONDUCTOR")); u != "" {
		return u
	}
	return weaveConductorName("")
}

// newSprintPingCmd interrupts the conductor responsible for a sprint.
func newSprintPingCmd() *cobra.Command {
	var flags weaveOutputFlags
	var body, priority string
	cmd := &cobra.Command{
		Use:   "ping <sprint>",
		Short: "Interrupt the conductor responsible for a sprint",
		Long: "ping reaches the conductor who owns a sprint's delivery.\n\n" +
			"It goes to the sprint's TOPIC, not to a person's name. The holder changes\n" +
			"across a handoff or a take, and a message addressed to whoever held it an\n" +
			"hour ago would land on somebody with no context — or on nobody. The topic\n" +
			"follows the responsibility.\n\n" +
			"Delivery is the bus's problem, and it is the half that is actually hard: an\n" +
			"agent mid-turn cannot decide to go and look somewhere, so the sidecar holds\n" +
			"the subscription off its critical path and leaves a buffer to read at a turn\n" +
			"boundary. An interrupt that cannot be delivered is DEMOTED to a queued\n" +
			"notification with a reason, never dropped.\n\n" +
			"The conversation itself belongs in the sprint's meet room, which `sprint\n" +
			"status` prints beside the conductor's name.",
		Example: "  bashy sprint ping 3 --body \"stopping you for the incident — park at a clean gate\"\n" +
			"  bashy sprint ping 3 --body \"need coreutils free\" --priority high",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			mode := flags.mode()
			dir, derr := weaveStoryDir(cmd, mode, "sprint ping")
			if derr != nil {
				return derr
			}
			q, lerr := loadWeaveQueue(dir)
			if lerr != nil {
				return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "sprint ping", weavecli.ExitGenericFail, lerr))
			}
			s := findWeaveStory(q, id)
			if s == nil {
				return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "sprint ping", weavecli.ExitInvalidArg,
					fmt.Errorf("sprint #%d not found", id)))
			}
			if err := pingSprintConductor(s, currentActor(), body, priority); err != nil {
				return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "sprint ping", weavecli.ExitGenericFail, err))
			}
			holder, stale, _ := weaveStoryLeaseState(s)
			where := ""
			if c := s.Contact.String(); c != "" {
				where = " — reply in " + c
			}
			line := fmt.Sprintf("pinged %s on %s%s", holder, sprintTopic(id), where)
			if stale {
				// Sent, but say so: a stale lease means the conductor stopped
				// heartbeating, so the ping may be addressed to nobody. The bus
				// will hold it, and the sender deserves to know it may wait.
				line += "\n  note: that conductor's lease is STALE — the ping is queued, but may go unread until someone takes the sprint"
			}
			if mode != weavecli.OutputJSON {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return ec(emitOK(cmd.OutOrStdout(), mode, "sprint ping", map[string]any{
				"topic": sprintTopic(id), "to": holder, "lease_stale": stale, "room": s.Contact.RefID(),
			}))
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "what to say")
	cmd.Flags().StringVar(&priority, "priority", "", "urgency hint for the sidecar's governance rules")
	flags.attach(cmd)
	return cmd
}
