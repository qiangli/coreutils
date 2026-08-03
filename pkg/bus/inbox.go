package bus

// `bashy inbox` — the one word an agent has to know.
//
// Everything needed to read a message already existed as `bashy bus pending`,
// and that is the problem: it is two words, it lives under a noun describing
// the TRANSPORT rather than the act, and an agent has to already understand the
// sidecar model to guess it. A message system whose read command has to be
// explained is one nobody reads.
//
// So this is a top-level alias, not a new mechanism. Same buffer, same
// resolve-on-read, same clear-after-print semantics — reachable by the word a
// person would actually try.
//
// # Reading marks, it does not delete
//
// An inbox that erases what it shows you cannot answer "what was I told, and
// when". The append-only room timeline already keeps every message forever, so
// dropping the per-agent view of them just made the two disagree. Reading now
// stamps read_at and keeps the record; --all shows the whole history.
//
// Anyone may read anyone's inbox (--as). That is deliberate and the trade is
// understood: the point is that a message is never lost, and a reader who
// marks somebody else's mail read has changed a STATUS, not destroyed content.
//
// It stays an ALIAS rather than a move. `bus pending` is the transport-level
// spelling and other tooling already uses it; breaking that to gain a nicer
// name would trade a working integration for an ergonomic one.

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// NewInboxCmd returns the top-level `inbox` verb.
func NewInboxCmd() *cobra.Command {
	cmd := newPendingCmd()
	cmd.Use = "inbox [flags]"
	cmd.Aliases = []string{"messages", "msgs"}
	cmd.Short = "read the messages other agents and humans sent you"
	cmd.Long = `inbox prints the messages addressed to you, and clears them.

  bashy inbox            read your new messages (marks them read)
  bashy inbox --peek     read without marking anything
  bashy inbox --all      every message ever received, read or not
  bashy inbox --json     one JSON object per line

Reading MARKS, it does not delete. A message stays in your history with a
read_at stamp, so --all can still answer "what was I told, and when" long after
the fact — the question that actually comes up when a fleet run goes wrong.

It resolves on read, so it works with NO sidecar and no daemon running: a
message sent while you were busy — or not running at all — is delivered the next
time you look. Anyone may send to you; only principals you have named may
interrupt a running turn (see 'bashy bus subscribe --interrupt-from'), so what
arrives here is queued rather than urgent.

This is the same buffer as 'bashy bus pending', under the word you would guess.`
	return cmd
}

// NewIMCmd returns the top-level `im` verb — the send half of the pair.
//
// `bus publish --to X --topic t "msg"` was already the mechanism, and like
// `bus pending` it was spelled for the transport rather than the act: three
// flags and a noun an agent has to understand before it can say hello. Sending
// a message to a colleague should cost one word.
//
//	bashy im codex-gpt5.6-sol "gate is red on main"
//	bashy inbox
//
// No authorization, deliberately. Any agent on the host may message any other,
// and reading marks rather than deletes — so the worst a bad actor can do is
// mark somebody's mail read, which changes a STATUS and destroys nothing. That
// is what keeps this simple enough to be used: the moment delivery could
// destroy history, it would need a permission model, and a permission model is
// how a messaging feature stops being one.
//
// Interrupting a running turn is the part that IS governed, and it stays that
// way: `bus subscribe --interrupt-from` names who may break in. Everything sent
// here is queued and read at the recipient's convenience.
func NewIMCmd() *cobra.Command {
	var topic, as string
	cmd := &cobra.Command{
		Use:     "im <agent> <message>...",
		Aliases: []string{"msg", "tell"},
		Short:   "send a message to another agent on this host",
		Long: `im sends a message to another agent. One word, one recipient, one line.

  bashy im codex-gpt5.6-sol "gate is red on main"
  bashy agents list                  # who you can write to
  bashy inbox                        # read what was sent to you

The recipient does not have to be running. A message to an agent that is down
waits in its inbox and is delivered the next time it looks, so "is it up right
now" is never a question the sender has to answer.

It arrives QUEUED — read at the recipient's next turn boundary, not forced into
whatever it is doing. Breaking into a running turn is a separate, governed act
(see 'bashy bus subscribe --interrupt-from').`,
		Args:          cobra.MinimumNArgs(2),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			to := strings.TrimSpace(args[0])
			body := strings.Join(args[1:], " ")
			// The principal is REQUIRED — Publish refuses without one (the
			// report/author invariant). It is who SENT this, resolved the same
			// way `bus publish` resolves it, so a message's origin is never a
			// guess and never blank.
			if err := Publish(Notification{
				Principal: resolvePrincipal(as), To: to, Topic: topic, Body: body,
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "sent to %s — they will see it with `bashy inbox`\n", to)
			return nil
		},
	}
	cmd.Flags().StringVar(&topic, "topic", "im", "topic label for the message")
	cmd.Flags().StringVar(&as, "as", "", "sender identity (default: your principal)")
	return cmd
}
