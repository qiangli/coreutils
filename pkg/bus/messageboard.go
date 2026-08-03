package bus

// `bashy mb` — the MESSAGE BOARD: wall + write, with selectable subsets.
//
// The shortest accurate description is the classic one. `mb post` is wall,
// `mb send <agent>` is write, and the addition is that the audience between
// "one" and "everyone" is selectable — by band, harness, provider, model family
// or version — which is the grouping a fleet actually needs and neither classic
// tool had.
//
// What it does NOT inherit from those two is the terminal. wall and write push
// to a tty: ephemeral, logged-in-only, and `write` to a logged-out user is an
// error. The board is durable and pull-read, which is the only shape that works
// when the party you need to tell something is not running.
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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// NewMessageBoardCmd returns the top-level `mb` verb.
//
// Bare `bashy mb` READS, because reading is what an agent does at the start of
// every turn and the common case should cost the fewest words.
func NewMessageBoardCmd() *cobra.Command {
	var jsonOut, peek, all bool
	var as string
	var limit int
	cmd := &cobra.Command{
		Use:     "mb",
		Aliases: []string{"messages"},
		Short:   "the host message board: read what was posted, post to others",
		Long: `mb is the host's message board — one shared, append-only board every agent
and human on this machine posts to and reads from.

  bashy mb                      what is new for you (marks it read)
  bashy mb post "..."           post to EVERYONE
  bashy mb send <agent> "..."   post to one agent
  bashy mb --all                the WHOLE board, everyone's posts
  bashy mb --peek               read without marking anything

PUBLIC BY CONSTRUCTION. Every post is visible to every reader; addressing is a
hint about who should act, never a permission. Nothing is deleted — reading only
advances your own cursor — so --all always answers "what was said, and when".

No setup: there is nothing to subscribe to. A post to an agent that is not
running waits on the board and is there when it next looks.`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			who := BoardIdentity(as)
			var posts []Post
			var older int
			var err error
			if all {
				posts, err = Posts()
			} else {
				var directed, other []Post
				directed, other, older, err = Unseen(who, limit)
				// Directed first: those carry an obligation, and a reader that
				// stops after the first screen must have seen them.
				posts = append(directed, other...)
			}
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if jsonOut {
				enc := json.NewEncoder(w)
				for _, p := range posts {
					if eerr := enc.Encode(p); eerr != nil {
						return eerr
					}
				}
			} else if len(posts) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "nothing new on the board for %s\n", who)
			} else {
				fmt.Fprintf(w, "## Board — %d post(s) for %s\n\n", len(posts), who)
				for _, p := range posts {
					to := p.Audiences()
					if p.Directed(who) {
						to = "you"
					}
					fmt.Fprintf(w, "- [%d] **%s** from `%s` → %s\n  %s\n\n", p.Seq, p.Topic, p.From, to, p.Body)
				}
				if older > 0 {
					// Say what was hidden. A cap that stays quiet is a silent
					// drop, and a reader cannot tell "nothing else" from
					// "twelve more" unless it is told.
					fmt.Fprintf(w, "_+%d older, not addressed to you — `bashy mb --all` for the whole board._\n", older)
				}
			}
			// Advance the cursor only AFTER the posts have been written out,
			// and never on --peek or --all: a read that fails halfway must not
			// consume what it did not show.
			if peek || all || len(posts) == 0 {
				return nil
			}
			return MarkSeen(who, posts[len(posts)-1].Seq)
		},
	}
	f := cmd.Flags()
	f.StringVar(&as, "as", "", "read as this identity (default: resolved from your principal)")
	f.BoolVar(&jsonOut, "json", false, "one JSON object per line")
	f.BoolVar(&peek, "peek", false, "read without marking anything read")
	f.BoolVar(&all, "all", false, "the whole board — every post by everyone, read or not")
	f.IntVarP(&limit, "limit", "n", DefaultBoardLimit,
		"cap posts NOT addressed to you by name (0 = no cap); directed posts are never capped")
	cmd.AddCommand(newMBSendCmd(), newMBPostCmd())
	cmd.CompletionOptions.DisableDefaultCmd = true
	return cmd
}

// newMBSendCmd posts to one agent, or to everyone a selector matches.
//
// No authorization, deliberately. Any agent may post to any other and read any
// view: the board is public, addressing is a hint about who should act, and
// reading only advances your own cursor. Nothing a reader can do destroys
// content, which is what keeps this simple enough to be used — the moment a
// read could destroy history it would need a permission model, and a permission
// model is how a messaging feature stops being one.
func newMBSendCmd() *cobra.Command {
	var topic, as, tool, provider, family, version string
	var band int
	cmd := &cobra.Command{
		Use:   "send [<agent>] <message>...",
		Short: "post to one agent, or to everyone matching a selector",
		Long: `send posts to a named agent, or to every agent a selector matches.

  bashy mb send codex-gpt5.6-sol "gate is red on main"
  bashy mb send --band 4 "need an L4 to review the converge gate"
  bashy mb send --tool ycode "ycode rebuilt — re-probe your bindings"
  bashy mb send --provider anthropic "anthropic keys rotated"
  bashy mb send --family opus "opus family: cost_micro was corrected"
  bashy mb send --family gemini-flash --version 3.6 "3.6 flash is now bound"

'bashy agents list' is the address book: a bare name is its NAME column, and the
selectors read the same catalog, so who is "L4" here and there can never drift.

Selectors are ANDed, not unioned. A union would make the wider blast radius the
easier thing to type, and on a shared board the wide one is what turns messages
into noise nobody reads. For genuinely everyone: 'bashy mb post'.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			aud := Audience{
				Band: band, Tool: strings.TrimSpace(tool),
				Provider: strings.TrimSpace(provider),
				Family:   strings.TrimSpace(family), Version: strings.TrimSpace(version),
			}
			if !aud.Empty() {
				// ONE post carrying the selector — not one per member. See
				// Post.Audience: expanding made the board grow with the size of
				// the audience.
				if err := PostMessage(Post{
					From: BoardIdentity(as), Audience: &aud, Topic: topic, Body: strings.Join(args, " "),
				}); err != nil {
					return err
				}
				reach := "no agent currently matches — the post stands and will reach whoever does"
				if FleetSelect != nil {
					if names, ferr := FleetSelect(aud); ferr == nil && len(names) > 0 {
						reach = fmt.Sprintf("%d agent(s) match now: %s", len(names), strings.Join(names, ", "))
					}
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "posted to %s — %s\n", aud.describe(), reach)
				return nil
			}
			if len(args) < 2 {
				return fmt.Errorf("mb send: name an agent, or pass a selector (--band/--tool/--provider/--family/--version)")
			}
			to := strings.TrimSpace(args[0])
			if err := PostMessage(Post{
				From: BoardIdentity(as), To: to, Topic: topic, Body: strings.Join(args[1:], " "),
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "posted to %s — they will see it with `bashy mb`\n", to)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&topic, "topic", "mb", "topic label for the post")
	f.StringVar(&as, "as", "", "sender identity (default: resolved from your principal)")
	f.IntVar(&band, "band", 0, "post to every agent at this band (1-4)")
	f.StringVar(&tool, "tool", "", "post to every agent on this harness (claude, ycode, agy, codex, opencode)")
	f.StringVar(&provider, "provider", "", "post to every agent whose model has this provider")
	f.StringVar(&family, "family", "", "post to every agent in this model family (opus, sonnet, gemini-flash, ...)")
	f.StringVar(&version, "version", "", "post to every agent on this model version (5, 4.8, 3.6, ...)")
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
			if err := PostMessage(Post{
				From: BoardIdentity(as), Topic: topic, Body: strings.Join(args, " "),
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
