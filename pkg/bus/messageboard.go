package bus

// `bashy mb` — the MESSAGE BOARD.
//
// This shipped first as `im` + `inbox`, and both names were wrong. What the
// mechanism actually is, structurally, is a bulletin board:
//
//	ONE SHARED SPOOL      every message to every recipient lives in one
//	                      append-only timeline.jsonl, not in per-recipient
//	                      mailboxes
//	PER-READER CURSORS    each subscriber tracks what it has seen — this is a
//	                      .newsrc, not a delivery receipt
//	TOPICS                the Topic field groups messages the way a newsgroup does
//	NOTHING IS PRIVATE    any caller may read any subscriber's view (--as), and
//	                      the spool itself is readable by anything on the host
//	NOTHING IS DELETED    reading marks; history is retained forever
//	PULL, NOT PUSH        a message arrives when the reader looks, unless the
//	                      session happens to have been launched by bashy
//
// That is Usenet, near enough exactly. It is NOT email — there is no private
// mailbox — and it is NOT instant messaging — there is no delivery to a party
// who is not looking. Calling it `inbox` and `im` promised both, and a name that
// promises what the mechanism does not do is the same class of defect as an exit
// code reporting a success nothing verified.
//
// So the honest name is the board, and `im` / `inbox` are DELIBERATELY LEFT
// UNREGISTERED. When there is a real per-recipient mailbox with delivery
// semantics, `inbox` should be free to mean it; when there is genuine push to a
// live party, `im` should be free to mean that. Spending those words on
// something that is neither would cost them permanently.

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// NewMessageBoardCmd returns the top-level `mb` verb.
//
// Bare `bashy mb` READS, because reading is what an agent does at the start of
// every turn and the common case should cost the fewest words.
func NewMessageBoardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "mb",
		Aliases: []string{"messages"},
		Short:   "the host message board: read what was posted to you, post to others",
		Long: `mb is the host's message board — a shared, append-only board every agent
on this machine can post to and read from.

  bashy mb                      read what is new for you (marks it read)
  bashy mb post "..."           post to EVERYONE on the board
  bashy mb send <agent> "..."   post to one agent
  bashy mb --all                every message you have received, read or not
  bashy mb --peek               read without marking anything

It is a BOARD, not a mailbox and not a chat. Everything lives in one shared
append-only spool: nothing is private, nothing is ever deleted, and a message
arrives when the reader looks rather than being pushed at them. Reading marks
what you saw, so --all can still answer "what was I told, and when" long after
the fact.

The recipient does not have to be running. A message to an agent that is down
waits and is delivered the next time it looks.

Posts arrive QUEUED — read at the recipient's next turn boundary, never forced
into whatever it is doing. Breaking into a running turn is a separate, governed
act (see 'bashy bus subscribe --interrupt-from').`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.CompletionOptions.DisableDefaultCmd = true

	read := newPendingCmd()
	read.Use = "read [flags]"
	read.Short = "read what was posted to you"
	cmd.AddCommand(read, newMBSendCmd(), newMBPostCmd())

	// Bare `bashy mb` reads. The read flags are mirrored onto the parent so
	// `mb --all` works without having to learn a subcommand first.
	cmd.Flags().AddFlagSet(read.Flags())
	cmd.RunE = read.RunE
	cmd.Args = cobra.NoArgs
	return cmd
}

// newMBSendCmd is the post half.
//
// `bus publish --to X --topic t "msg"` was already the mechanism and was spelled
// for the TRANSPORT: three flags and a noun a reader has to understand before
// they can say anything. Posting to a colleague should cost one line.
//
// No authorization, deliberately. Any agent on the host may post to any other
// and read any board view, and reading marks rather than deletes — so the worst
// a bad actor can do is mark something read, which changes a STATUS and destroys
// nothing. The moment a read could destroy history it would need a permission
// model, and a permission model is how a messaging feature stops being one.
func newMBSendCmd() *cobra.Command {
	var topic, as string
	cmd := &cobra.Command{
		Use:   "send <agent> <message>...",
		Short: "post a message to another agent on this host",
		Long: `send posts a message to another agent.

  bashy mb send codex-gpt5.6-sol "gate is red on main"
  bashy agents list                  # who you can post to (the NAME column)`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			to := strings.TrimSpace(args[0])
			body := strings.Join(args[1:], " ")
			// NO PRIOR SETUP. Open the recipient's inbox first if it has none:
			// Matches needs a stored subscription, so without this a post to a
			// name outside the fleet catalog reports "posted" and reaches
			// nobody — the exit-0 lie this package exists to remove.
			//
			// BEFORE Publish, and the order is the whole point: a new inbox
			// opens at the current timeline head, so it must be created before
			// the post is appended or the post lands at or below its own
			// cursor and is never delivered.
			if _, eerr := EnsureSubscription(to); eerr != nil {
				return eerr
			}
			// The principal is REQUIRED — Publish refuses without one (the
			// report/author invariant) — so a post's origin is never a guess
			// and never blank.
			if err := Publish(Notification{
				Principal: resolvePrincipal(as), To: to, Topic: topic, Body: body,
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "posted to %s — they will see it with `bashy mb`\n", to)
			return nil
		},
	}
	cmd.Flags().StringVar(&topic, "topic", "mb", "topic label for the post")
	cmd.Flags().StringVar(&as, "as", "", "sender identity (default: your principal)")
	return cmd
}

// newMBPostCmd is the BROADCAST half — a public forum post.
//
// It carries no recipient at all. Delivery works because every default
// subscription joins the well-known BoardRoom, and Subscription.Matches accepts
// on Room as readily as on To. Without that membership a recipient-less post
// would match nothing and reach nobody while reporting success — the failure
// this whole line of work exists to remove.
func newMBPostCmd() *cobra.Command {
	var topic, as string
	cmd := &cobra.Command{
		Use:   "post <message>...",
		Short: "post to EVERYONE on the board",
		Long: `post broadcasts to every agent on this host. No recipient, public by
construction — anyone can read it and anyone can respond.

  bashy mb post "cert campaign: sh/ is frozen until P0-1 lands"
  bashy mb                  # what others have posted to you or to everyone

Use 'mb send <agent>' when exactly one agent needs to act. A broadcast everybody
must read is how a board becomes noise nobody reads.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := Publish(Notification{
				Principal: resolvePrincipal(as),
				Room:      BoardRoom,
				Topic:     topic,
				Body:      strings.Join(args, " "),
			}); err != nil {
				return err
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "posted to the board — everyone will see it with `bashy mb`")
			return nil
		},
	}
	cmd.Flags().StringVar(&topic, "topic", "mb", "topic label for the post")
	cmd.Flags().StringVar(&as, "as", "", "sender identity (default: your principal)")
	return cmd
}
