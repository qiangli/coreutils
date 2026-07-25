package notify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/qiangli/coreutils/pkg/room"
	"github.com/spf13/cobra"
)

const SchemaVersion = "notify-v1"

// Envelope is the structured response in --json mode.
type Envelope struct {
	SchemaVersion string `json:"schema_version"`
	Status        string `json:"status"`
	Principal     string `json:"principal,omitempty"`
	Topic         string `json:"topic,omitempty"`
	Room          string `json:"room,omitempty"`
	To            string `json:"to,omitempty"`
	Message       string `json:"message,omitempty"`
	Error         string `json:"error,omitempty"`
}

// NewCommand builds `bashy notify`: publish a notification to the host room's
// timeline, which `bashy chat timeline` reads.
//
// NOT `bashy watch` — that name is taken by the classic watch(1) (run a command
// periodically), so the subscribe/drain half of the bus still needs a name of its
// own. See dhnt/docs/agent-notification-bus-design.md.
func NewCommand() *cobra.Command {
	var topic, to, roomID, principal string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "notify --topic <topic> [flags] <message>",
		Short: "Publish a notification to the host room timeline (the agent bus)",
		Long: `Publish a notification to the host room's timeline — the push half of
the agent bus, where 'bashy kb' is the durable pull half.

Read notifications back with:

  bashy chat timeline

Addressing — at least one of these is REQUIRED, because a notification
nobody is addressed by cannot be delivered to anyone:
  --topic <t>    broadcast to a named topic
  --to <id>      1:1 delivery to a session or role
  --room <id>    room-scoped publish

Every publish must carry a principal (who sent it). Supply --principal,
set $BASHY_PRINCIPAL, or $USER is used as the default. A publish with
no principal is rejected — this is the REPORT/AUTHOR invariant.

There is no subscribe/drain verb yet: publishing appends to the timeline,
and readers poll it. Note that 'bashy watch' is NOT that subscriber — it is
the classic watch(1), which runs a command periodically.`,
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			msg := strings.Join(args, " ")
			who := resolvePrincipal(principal)
			if who == "" {
				return wrapErr("principal is required (REPORT/AUTHOR invariant): set --principal or $BASHY_PRINCIPAL", topic, roomID, to, msg, jsonOut, cmd)
			}
			// Addressing was DOCUMENTED as required and never enforced, so an
			// unaddressed publish succeeded and exited 0. That is a notification
			// that cannot be delivered to anyone: no topic to match, no recipient
			// to route to, no room to scope it. It lands on the timeline carrying
			// no more information than silence, while reporting success — the same
			// success-by-absence-of-evidence shape the fleet invariant forbids.
			// Refusing costs the caller one flag and makes the exit code mean
			// something.
			if topic == "" && to == "" && roomID == "" {
				return wrapErr("addressing is required: pass at least one of --topic, --to, or --room (an unaddressed notification reaches nobody)", topic, roomID, to, msg, jsonOut, cmd)
			}

			ev := room.Event{
				Principal: who,
				Topic:     topic,
				Room:      roomID,
				To:        to,
				Body:      msg,
			}

			if err := room.Notify(ev); err != nil {
				return wrapErr(err.Error(), topic, roomID, to, msg, jsonOut, cmd)
			}

			if jsonOut {
				return emitOK(cmd, who, topic, roomID, to, msg)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&topic, "topic", "", "topic to broadcast to")
	cmd.Flags().StringVar(&to, "to", "", "recipient session or role (1:1)")
	cmd.Flags().StringVar(&roomID, "room", "", "room ID for room-scoped publish")
	cmd.Flags().StringVar(&principal, "principal", "", "sender principal (who is publishing)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit notify-v1 JSON envelope")

	return cmd
}

func resolvePrincipal(flag string) string {
	if strings.TrimSpace(flag) != "" {
		return strings.TrimSpace(flag)
	}
	if p := strings.TrimSpace(os.Getenv("BASHY_PRINCIPAL")); p != "" {
		return p
	}
	return strings.TrimSpace(os.Getenv("USER"))
}

// ErrReported marks a failure the command has ALREADY written to stderr, in
// whichever form the caller asked for. A front end wraps it with ExitCode and
// prints nothing more.
//
// Without it the two output modes cannot both be correct: --json writes a
// `"status":"error"` envelope to stderr, and a front end that also prints the
// returned error appends a plain line to what is supposed to be a parseable
// stream. One reporting path, one place that decides the wording.
var ErrReported = errors.New("notify: already reported")

// ExitCode maps a RunE error to a process exit status, mirroring the
// skills.ExitCode / dag.ExitCodeOf convention used by the other front-door verbs.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	return 1
}

// Reported reports whether the command has already written this failure to
// stderr, so a front end knows not to print it again.
func Reported(err error) bool { return errors.Is(err, ErrReported) }

func wrapErr(errMsg, topic, roomID, to, msg string, jsonOut bool, cmd *cobra.Command) error {
	if jsonOut {
		return emitError(cmd, errMsg, topic, roomID, to, msg)
	}
	return fmt.Errorf("%s", errMsg)
}

func emitOK(cmd *cobra.Command, principal, topic, roomID, to, msg string) error {
	env := Envelope{
		SchemaVersion: SchemaVersion,
		Status:        "ok",
		Principal:     principal,
		Topic:         topic,
		Room:          roomID,
		To:            to,
		Message:       msg,
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(env)
}

// emitError writes the error envelope AND returns a real error.
//
// It used to return `enc.Encode(env)`, which is nil whenever the encode
// succeeds — so in --json mode every refusal printed a `"status":"error"`
// envelope on stderr and then EXITED 0. The two channels contradicted each
// other, and the exit code is the one an agent checks: `bashy notify --json …
// && echo sent` reported success for a notification that was never published.
//
// The encode error is deliberately discarded in favour of the original failure:
// if stderr is broken too there is nothing useful to say, and the caller needs
// to know why the publish was refused, not why the report about it failed.
func emitError(cmd *cobra.Command, errMsg, topic, roomID, to, msg string) error {
	env := Envelope{
		SchemaVersion: SchemaVersion,
		Status:        "error",
		Topic:         topic,
		Room:          roomID,
		To:            to,
		Message:       msg,
		Error:         errMsg,
	}
	enc := json.NewEncoder(cmd.ErrOrStderr())
	enc.SetIndent("", "  ")
	_ = enc.Encode(env)
	// Wrap ErrReported: the envelope above IS the report, so a front end must not
	// print a second, plain-text copy into the JSON stream it just wrote.
	return fmt.Errorf("%s: %w", errMsg, ErrReported)
}
