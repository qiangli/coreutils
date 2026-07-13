// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package steward

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// opts is the state every subcommand shares: which store, and whether the caller
// wants prose or JSON.
type opts struct {
	dir    string
	asJSON bool
}

func (o *opts) store() (*Store, error) { return Open(o.dir) }

// NewStewardCmd builds `bashy steward`: the host's single seat of authority and
// its permanent record.
//
// Mounted by the host (bashy) rather than exported as a tool, because a steward is
// not a userland utility — it is the thing the human talks TO about everything the
// agents on this machine did.
func NewStewardCmd() *cobra.Command {
	o := &opts{}
	cmd := &cobra.Command{
		Use:   "steward",
		Short: "the host's one seat of authority, and the journal that outlives whoever holds it",
		Long: `steward is the ONE agent per host/user that answers for what happened here.

Not one per repo, not one per terminal — one per machine-and-account, held under a
heartbeat lease, and recoverable by the next agent WITHOUT the last one's
cooperation. That last part is the whole design: a steward that crashed, was
rate-limited, or simply vanished leaves no goodbye note, and continuity must
survive it anyway.

  THE JOURNAL IS THE ONLY TRUTH.
  Board, status, log, conversation, history and checkpoints are read-only
  PROJECTIONS of it. None of them is a second place where state lives, so none of
  them can drift from it or quietly become the real one.

  EVIDENCE, OR IT DID NOT HAPPEN.
  An entry claiming success with nothing to point at projects as UNKNOWN, not as
  success. An agent writes confident prose about work it did not do; the only
  defense that scales is to refuse to promote an unevidenced claim into a fact.

  A STALE HEARTBEAT PROVES ONLY A LAPSE.
  It does not prove the incumbent is dead — it may be mid-thought, throttled, or
  on a bad network, and it may come back. So a successor's claim BUMPS A FENCING
  EPOCH: the returning incumbent, still holding the old epoch, is rejected
  loudly instead of silently interleaving its writes with the new steward's.

steward is NOT handoff. ` + "`bashy handoff`" + ` moves WORK — a diff, a working tree, a
task. steward moves a MANDATE. Claiming the seat touches no repository, restores
no working tree, and captures no diff.`,
		Example: `  bashy steward status                     # who holds the seat, and is the record sound?
  bashy steward claim --intent "on call"   # take a vacant or lapsed seat
  bashy steward record --workstream api -m "migrated the schema" \
        --outcome success -e "command:go test ./..." -e "commit:de6485c"
  bashy steward decide --workstream api -m "drop the v1 endpoint" \
        --rationale "no callers left in 90d of logs"
  bashy steward board                      # what is in flight, and what is unproven
  bashy steward log --degraded             # what do we NOT actually know?
  bashy steward reconcile                  # the verb a successor runs FIRST
  bashy steward takeover --authorized-by qiangli --reason "incumbent wedged"`,
		SilenceUsage: true,
	}
	cmd.PersistentFlags().StringVar(&o.dir, "dir", "",
		"steward store (default $BASHY_STEWARD_DIR, else ~/.bashy/steward)")
	cmd.PersistentFlags().BoolVar(&o.asJSON, "json", false, "emit machine-readable JSON")

	cmd.AddCommand(
		newStatusCmd(o),
		newBoardCmd(o),
		newLogCmd(o),
		newConversationCmd(o),
		newHistoryCmd(o),
		newCheckpointCmd(o),
		newReconcileCmd(o),
		newClaimCmd(o),
		newTakeoverCmd(o),
		newReleaseCmd(o),
		newHeartbeatCmd(o),
		newRecordCmd(o),
		newDecideCmd(o),
		newTranscriptCmd(o),
		newWorkstreamCmd(o),
	)
	return cmd
}

// ─── seat lifecycle ───────────────────────────────────────────────────────────

func newClaimCmd(o *opts) *cobra.Command {
	var intent string
	cmd := &cobra.Command{
		Use:     "claim",
		Aliases: []string{"take"},
		Short:   "acquire a vacant or lapsed seat (atomically; never asks the incumbent)",
		Long: `claim takes the seat when nobody holds it, or when the holder's heartbeat has
lapsed. It is the ORDINARY path.

It never negotiates with the incumbent and never needs a handoff note: it reads
the journal, decides, and writes — all under one lock, so two agents racing for an
empty seat cannot both win.

Taking over a LIVE seat is a different act with a different name (` + "`steward takeover`" + `),
because seizing authority from a working agent is a human's call, not an agent's.

Re-claiming a seat you already hold and are live in is just a heartbeat: no new
epoch, no new journal entry.

Claiming captures NO repository state. It is a mandate, not a checkout.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			v, err := s.Claim(Self(), intent, time.Now())
			if err != nil {
				return err
			}
			if o.asJSON {
				return emitJSON(cmd.OutOrStdout(), v)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "claimed the steward seat: %s at epoch %d\n",
				holderName(v.Authority.Holder), v.Authority.Epoch)
			if v.Authority.TakenOverFrom != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  the lapsed seat was held by %s — they are now FENCED at the old epoch\n",
					holderName(*v.Authority.TakenOverFrom))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  store: %s\n", s.Dir())
			fmt.Fprintln(cmd.OutOrStdout(), "\nRun `steward reconcile` before acting: it reports what the record can and cannot establish.")
			return nil
		},
	}
	cmd.Flags().StringVar(&intent, "intent", "", "what you hold the seat to do")
	return cmd
}

func newTakeoverCmd(o *opts) *cobra.Command {
	var by, reason string
	cmd := &cobra.Command{
		Use:   "takeover",
		Short: "seize a LIVE seat under explicit human authorization, fencing the prior holder",
		Long: `takeover is the RECOVERY path, and it is deliberately the loud one.

It seizes the seat whether or not the incumbent is live, bumps the fencing epoch,
and records who authorized it, from whom it was taken, and why.

It requires a named human (--authorized-by). That is not ceremony: an agent that
could decide on its own to take over would eventually decide to do it to a healthy
steward. Seizing authority is a human's call.

It never asks the incumbent — an incumbent that could be asked would not need to be
taken over. From the instant the epoch bumps, the prior holder's writes are
REJECTED rather than interleaved, so a steward that comes back from a network
partition mid-sentence cannot corrupt the record.`,
		Example: `  bashy steward takeover --authorized-by qiangli --reason "incumbent wedged on a rate limit"`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			v, err := s.Takeover(Self(), Authorization{By: by, Reason: reason}, time.Now())
			if err != nil {
				return err
			}
			if o.asJSON {
				return emitJSON(cmd.OutOrStdout(), v)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "TOOK OVER the steward seat: %s at epoch %d (authorized by %s)\n",
				holderName(v.Authority.Holder), v.Authority.Epoch, by)
			if v.Authority.TakenOverFrom != nil {
				fmt.Fprintf(out, "  fenced: %s — any write it attempts at its old epoch is now rejected\n",
					holderName(*v.Authority.TakenOverFrom))
			}
			if reason != "" {
				fmt.Fprintf(out, "  reason: %s\n", reason)
			}
			fmt.Fprintln(out, "\nRun `steward reconcile` now: the prior steward may have left claims nobody has verified.")
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "authorized-by", "", "the human authorizing this seizure (required)")
	cmd.Flags().StringVar(&reason, "reason", "", "why recovery is necessary")
	return cmd
}

func newReleaseCmd(o *opts) *cobra.Command {
	var note string
	var epoch uint64
	cmd := &cobra.Command{
		Use:   "release",
		Short: "vacate the seat cleanly (captures no repository state)",
		Long: `release vacates the seat so the next steward can claim it without waiting for the
lease to lapse.

It is a COURTESY, not a correctness requirement — an unreleased seat still expires,
and the epoch still fences whoever comes back. The system is designed for the
steward that never gets to say goodbye; releasing merely saves the next one a wait.

It captures NO repository state: no diff, no branch, no working tree. A steward
hands over a MANDATE. Work in flight travels by ` + "`bashy handoff`" + `, which is a
different verb because it is a different thing — and conflating them is what made
"hand off your work" ambiguous in the first place.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			if err := s.Release(Self(), epoch, note, time.Now()); err != nil {
				return err
			}
			if o.asJSON {
				v, err := s.Status(time.Now())
				if err != nil {
					return err
				}
				return emitJSON(cmd.OutOrStdout(), v)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "released the steward seat. The journal keeps everything; only the seat is empty.")
			fmt.Fprintln(cmd.OutOrStdout(), "No repository state was captured — `bashy handoff` is the verb that moves WORK.")
			return nil
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "why you are standing down")
	cmd.Flags().Uint64Var(&epoch, "epoch", 0, "the epoch you believe you hold (0 = whatever is current)")
	return cmd
}

func newHeartbeatCmd(o *opts) *cobra.Command {
	return &cobra.Command{
		Use:   "heartbeat",
		Short: "refresh the holder's liveness (writes no journal entry)",
		Long: `heartbeat refreshes the seat's liveness so it does not lapse.

It writes NO journal entry. A heartbeat is a pulse, not history, and a journal that
recorded every pulse would bury the events that matter underneath them.

Recording to the journal already heartbeats — a steward that is actively writing is
self-evidently alive — so this is only needed by a steward that is thinking for a
long time without producing any record.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			if err := s.Heartbeat(Self(), time.Now()); err != nil {
				return err
			}
			if o.asJSON {
				v, err := s.Status(time.Now())
				if err != nil {
					return err
				}
				return emitJSON(cmd.OutOrStdout(), v)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "heartbeat recorded.")
			return nil
		},
	}
}

// ─── status ───────────────────────────────────────────────────────────────────

// statusEnvelope is the stable --json shape for `steward status`: the seat, the
// board, and the journal's coordinates in one object.
type statusEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Seat          View   `json:"seat"`
	Board         Board  `json:"board"`
	Journal       struct {
		Entries int    `json:"entries"`
		Head    string `json:"head"`
		Intact  bool   `json:"intact"`
		Corrupt string `json:"corrupt,omitempty"`
	} `json:"journal"`
	Dir string `json:"dir"`
}

func newStatusCmd(o *opts) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "who holds the seat, are they alive, and what does the board say",
		Long: `status answers the two questions a successor asks first, and keeps them SEPARATE:

  AUTHORITY — who holds the seat, at which epoch. Replayed from the journal, so it
              survives losing everything else. Delete the heartbeat file entirely
              and the holder and epoch are still known.
  LIVENESS  — is that holder still breathing. Read from the heartbeat, and honest
              about its limits: a lapse is a LAPSE, not a death.

Then the board: what is in flight, and — the part that matters — what is claimed but
not established.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			now := time.Now()
			rep, err := s.Replay()
			if err != nil {
				return err
			}
			v, err := s.viewFrom(rep, now)
			if err != nil {
				return err
			}
			board := ProjectBoard(rep.Entries)

			if o.asJSON {
				env := statusEnvelope{SchemaVersion: SchemaVersion, Seat: v, Board: board, Dir: s.Dir()}
				env.Journal.Entries = len(rep.Entries)
				env.Journal.Head = rep.Head
				env.Journal.Intact = rep.Intact()
				if rep.Corrupt {
					env.Journal.Corrupt = fmt.Sprintf("line %d: %s", rep.CorruptLine, rep.CorruptReason)
				}
				return emitJSON(cmd.OutOrStdout(), env)
			}

			out := cmd.OutOrStdout()
			switch v.Liveness {
			case LivenessVacant:
				fmt.Fprintf(out, "seat: VACANT — no steward on this host/user\n")
				if v.Authority.Epoch > 0 {
					fmt.Fprintf(out, "  epoch: %d (the ladder never resets; the next claim takes %d)\n",
						v.Authority.Epoch, v.Authority.Epoch+1)
				}
				fmt.Fprintln(out, "  claim it: `steward claim`")
			default:
				fmt.Fprintf(out, "seat: %s (epoch %d) — %s\n", holderName(v.Authority.Holder), v.Authority.Epoch, v.Liveness)
				if !v.Since.IsZero() {
					fmt.Fprintf(out, "  since:     %s\n", v.Since.Format(time.RFC3339))
				}
				switch v.Liveness {
				case LivenessLive:
					fmt.Fprintf(out, "  heartbeat: %s (%s ago)\n", v.Heartbeat.Format(time.RFC3339), short(now.UTC().Sub(v.Heartbeat)))
				case LivenessLapsed:
					fmt.Fprintf(out, "  heartbeat: %s (%s ago — LAPSED, which proves a lapse and nothing more:\n"+
						"             they may be mid-thought, throttled, or coming back. Claiming FENCES them, safely.)\n",
						v.Heartbeat.Format(time.RFC3339), short(now.UTC().Sub(v.Heartbeat)))
				case LivenessUnknown:
					fmt.Fprintln(out, "  heartbeat: UNKNOWN — no liveness record. Authority above still replayed from the journal.")
				}
				if v.Intent != "" {
					fmt.Fprintf(out, "  intent:    %s\n", v.Intent)
				}
				if v.Authority.AuthorizedBy != "" {
					fmt.Fprintf(out, "  took over: authorized by %s", v.Authority.AuthorizedBy)
					if v.Authority.TakenOverFrom != nil {
						fmt.Fprintf(out, ", from %s", holderName(*v.Authority.TakenOverFrom))
					}
					fmt.Fprintln(out)
				}
			}

			fmt.Fprintf(out, "  journal:   %d entries, ", len(rep.Entries))
			if rep.Intact() {
				fmt.Fprintln(out, "intact")
			} else {
				fmt.Fprintf(out, "CORRUPT TAIL at line %d (%s)\n", rep.CorruptLine, rep.CorruptReason)
				fmt.Fprintf(out, "             the %d entries above are valid and unaffected; `steward reconcile --repair-tail` truncates only the unreadable bytes\n", len(rep.Entries))
			}
			fmt.Fprintf(out, "  store:     %s\n", s.Dir())

			fmt.Fprintln(out)
			writeBoard(out, board)
			return nil
		},
	}
}

// ─── board ────────────────────────────────────────────────────────────────────

func newBoardCmd(o *opts) *cobra.Command {
	return &cobra.Command{
		Use:   "board [workstream]",
		Short: "the workstreams, and which of their outcomes are actually established",
		Long: `board is a read-only PROJECTION of the journal — never a place where state lives.

It cannot drift from the journal, because it has no state of its own to drift with.
Replay the journal and you get this board, byte for byte, on any host.

Two columns matter and they are deliberately separate:

  STATE       open or closed — where the work is in its lifecycle.
  CONFIDENCE  verified, unknown, or degraded — whether we can actually BELIEVE the
              outcome it reports.

A workstream closed with a claim of success and no evidence is closed AND unknown.
Collapsing those into one green row is exactly how a status board starts reporting
wishes as facts.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			board, _, err := s.Board()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			if len(args) == 1 {
				for _, ws := range board.Workstreams {
					if ws.Name != args[0] {
						continue
					}
					if o.asJSON {
						return emitJSON(out, ws)
					}
					fmt.Fprintf(out, "workstream: %s\n", ws.Name)
					if ws.Title != "" {
						fmt.Fprintf(out, "  title:      %s\n", ws.Title)
					}
					fmt.Fprintf(out, "  state:      %s\n", ws.State)
					fmt.Fprintf(out, "  outcome:    %s (confidence: %s)\n", orDash(string(ws.Outcome)), ws.Confidence)
					fmt.Fprintf(out, "  entries:    %d (%d evidence, %d decisions)\n", ws.Entries, ws.EvidenceCount, ws.Decisions)
					if !ws.OpenedAt.IsZero() {
						fmt.Fprintf(out, "  opened:     %s\n", ws.OpenedAt.Format(time.RFC3339))
					}
					if ws.LastSummary != "" {
						fmt.Fprintf(out, "  last:       %s\n", ws.LastSummary)
					}
					for _, d := range ws.Degraded {
						fmt.Fprintf(out, "  UNPROVEN:   %s\n", d)
					}
					return nil
				}
				return fmt.Errorf("steward: no workstream %q on the board", args[0])
			}

			if o.asJSON {
				return emitJSON(out, board)
			}
			writeBoard(out, board)
			return nil
		},
	}
}

// writeBoard renders the board, leading with the honest headline.
func writeBoard(out io.Writer, b Board) {
	if len(b.Workstreams) == 0 {
		fmt.Fprintln(out, "board: empty — no workstreams recorded")
		return
	}
	degraded := 0
	for _, ws := range b.Workstreams {
		if ws.Outcome == OutcomeUnknown || ws.Outcome == OutcomeDegraded {
			degraded++
		}
	}
	fmt.Fprintf(out, "board: %d workstream(s)", len(b.Workstreams))
	if degraded > 0 {
		fmt.Fprintf(out, ", %d with an outcome NOBODY ESTABLISHED", degraded)
	}
	fmt.Fprintf(out, "  (watermark %d)\n", b.Watermark)

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  WORKSTREAM\tSTATE\tOUTCOME\tCONFIDENCE\tLAST")
	for _, ws := range b.Workstreams {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n",
			ws.Name, ws.State, orDash(string(ws.Outcome)), ws.Confidence, truncate(ws.LastSummary, 44))
	}
	tw.Flush()

	if degraded > 0 {
		fmt.Fprintln(out, "\nUnproven — a claim nobody can check is not a fact:")
		for _, ws := range b.Workstreams {
			if ws.Outcome != OutcomeUnknown && ws.Outcome != OutcomeDegraded {
				continue
			}
			for _, d := range ws.Degraded {
				fmt.Fprintf(out, "  %s: %s\n", ws.Name, d)
			}
		}
	}
}

// ─── log / conversation / history ─────────────────────────────────────────────

func newLogCmd(o *opts) *cobra.Command {
	var (
		f        Filter
		kinds    []string
		since    string
		follow   bool
		interval time.Duration
	)
	cmd := &cobra.Command{
		Use:   "log",
		Short: "the journal, chronologically — the authoritative record itself",
		Long: `log prints the journal: the one authoritative record, in the order it was written.

Chronological because the journal is append-only — entry order IS time order, and a
log that reordered them would be inventing a history the hash chain does not attest
to.

  --degraded   the query a successor needs FIRST: only the entries whose claims were
               never established. "What do I not actually know?"
  --follow     stream new entries as they land (polling; a corrupt tail does not stop
               the stream, it just does not advance past it)`,
		Example: `  bashy steward log --limit 20
  bashy steward log --degraded          # what is claimed but unproven
  bashy steward log --kind decision --workstream api
  bashy steward log --follow`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			for _, k := range kinds {
				f.Kinds = append(f.Kinds, Kind(k))
			}
			if since != "" {
				t, err := parseSince(since)
				if err != nil {
					return err
				}
				f.Since = t
			}

			entries, rep, err := s.Log(f)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			if o.asJSON {
				if err := emitJSON(out, entries); err != nil {
					return err
				}
			} else {
				if len(entries) == 0 {
					fmt.Fprintln(out, "log: no entries match")
				}
				for _, e := range entries {
					writeEntry(out, e)
				}
				warnCorrupt(out, rep)
			}
			if !follow {
				return nil
			}

			// Follow until the caller interrupts. Ctrl-C is how this ENDS, not how it
			// fails, so a cancelled follow exits 0.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			return s.Follow(ctx, f, interval, func(e Entry) error {
				if o.asJSON {
					return emitJSONLine(out, e)
				}
				writeEntry(out, e)
				return nil
			})
		},
	}
	cmd.Flags().StringSliceVar(&kinds, "kind", nil, "only these kinds (effect, observation, decision, transcript, reconcile, checkpoint, seat.*, workstream.*)")
	cmd.Flags().StringVar(&f.Workstream, "workstream", "", "only this workstream")
	cmd.Flags().StringVar(&f.Actor, "actor", "", "only this actor")
	cmd.Flags().BoolVar(&f.DegradedOnly, "degraded", false, "only entries whose claim was never established — the 'what do I not know?' query")
	cmd.Flags().IntVar(&f.Limit, "limit", 0, "keep only the last N matches (0 = all)")
	cmd.Flags().StringVar(&since, "since", "", "only entries after this time (RFC3339, or a duration like 2h)")
	cmd.Flags().BoolVar(&follow, "follow", false, "stream new entries as they are appended")
	cmd.Flags().DurationVar(&interval, "interval", time.Second, "poll interval for --follow")
	return cmd
}

func newConversationCmd(o *opts) *cobra.Command {
	var f Filter
	cmd := &cobra.Command{
		Use:     "conversation",
		Aliases: []string{"conv"},
		Short:   "the decisions — and, where a transcript survives, how the room got there",
		Long: `conversation shows what was DECIDED and why.

Decisions are AUTHORITATIVE: an explicit, durable record of intent with a rationale.
They are what a successor reads to learn not just what happened on this host, but
what the previous steward had concluded and was steering toward — which no amount of
replaying effects would ever recover.

Transcripts are NOT authoritative and are shown only as a courtesy. They are
hash-linked artifacts; nothing derives from them; deleting every one of them changes
no board, no status, and no checkpoint. The decision record is what binds — a
transcript merely lets a human see how the room got there.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			entries, rep, err := s.Conversation(f)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if o.asJSON {
				return emitJSON(out, entries)
			}
			if len(entries) == 0 {
				fmt.Fprintln(out, "conversation: nothing decided yet")
				return nil
			}
			for _, e := range entries {
				switch e.Kind {
				case KindDecision:
					fmt.Fprintf(out, "── DECISION  seq %d  %s  %s\n", e.Seq, e.Time, holderName(e.Actor))
					if e.Workstream != "" {
						fmt.Fprintf(out, "   workstream: %s\n", e.Workstream)
					}
					fmt.Fprintf(out, "   %s\n", e.Summary)
					if e.Rationale != "" {
						fmt.Fprintf(out, "   because: %s\n", e.Rationale)
					}
					for _, ev := range e.Evidence {
						fmt.Fprintf(out, "   evidence: %s:%s\n", ev.Kind, ev.Ref)
					}
				case KindTranscript:
					fmt.Fprintf(out, "── transcript (NON-AUTHORITATIVE)  seq %d  %s\n", e.Seq, e.Time)
					fmt.Fprintf(out, "   %s\n", e.Summary)
					if e.Artifact != nil {
						fmt.Fprintf(out, "   artifact: %s (%s)\n", e.Artifact.Path, e.Artifact.Digest)
					}
				}
				fmt.Fprintln(out)
			}
			warnCorrupt(out, rep)
			return nil
		},
	}
	cmd.Flags().StringVar(&f.Workstream, "workstream", "", "only this workstream")
	cmd.Flags().IntVar(&f.Limit, "limit", 0, "keep only the last N (0 = all)")
	return cmd
}

// historyEnvelope is the stable --json shape for `steward history`.
type historyEnvelope struct {
	SchemaVersion string          `json:"schema_version"`
	Seat          []StateChange   `json:"seat"`
	Checkpoints   []CheckpointRef `json:"checkpoints"`
}

func newHistoryCmd(o *opts) *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "how the seat changed hands, and the checkpoints taken along the way",
		Long: `history is the seat's authority ladder, reconstructed ENTIRELY by replay.

Every claim, every takeover, every release — who, when, at which epoch, and (for a
takeover) which human authorized seizing it. There is nowhere else this is stored:
delete the heartbeat file, delete every checkpoint, and this history is unchanged,
because it lives in the journal like everything else that matters.

The checkpoints listed are the ones the JOURNAL remembers being taken. That is a
different question from which checkpoint FILES still exist — a file is a cache and
can be deleted or re-derived; the fact that a checkpoint was taken at a watermark is
history.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			changes, cks, rep, err := s.History()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if o.asJSON {
				return emitJSON(out, historyEnvelope{SchemaVersion: SchemaVersion, Seat: changes, Checkpoints: cks})
			}

			if len(changes) == 0 {
				fmt.Fprintln(out, "history: the seat has never been held")
			} else {
				fmt.Fprintln(out, "seat history:")
				tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "  SEQ\tWHEN\tEVENT\tEPOCH\tACTOR\tAUTHORIZED BY")
				for _, c := range changes {
					fmt.Fprintf(tw, "  %d\t%s\t%s\t%d\t%s\t%s\n",
						c.Seq, c.At.Format(time.RFC3339), c.Kind, c.Epoch, c.Actor, orDash(c.AuthorizedBy))
				}
				tw.Flush()
			}

			if len(cks) > 0 {
				fmt.Fprintln(out, "\ncheckpoints (as the journal remembers them — the files are only a cache):")
				for _, c := range cks {
					fmt.Fprintf(out, "  seq %-4d %s  %s\n", c.Seq, c.At.Format(time.RFC3339), c.ID)
				}
			}
			warnCorrupt(out, rep)
			return nil
		},
	}
}

// ─── checkpoint ───────────────────────────────────────────────────────────────

func newCheckpointCmd(o *opts) *cobra.Command {
	var (
		note   string
		list   bool
		verify string
	)
	cmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "materialize a verified, reproducible projection of the journal",
		Long: `checkpoint materializes the board at the journal's current head, and records that
it did so IN the journal.

A checkpoint is a CACHE WITH A RECEIPT, never a competing truth. It carries the
watermark it projects and the chain digest at that watermark, so it can be VERIFIED
rather than trusted: re-project the journal at the same watermark and compare. Same
entries, same board, always — no clock, no randomness, no ambient state leaks into
the projection.

Delete every checkpoint on the host and you have lost nothing but the time it takes
to recompute them.

The tempting design — a checkpoint you can EDIT, that accumulates state the journal
never saw — produces an artifact that is faster to read and impossible to trust: the
first time it disagrees with the journal, nobody can say which one is wrong. This
package structurally cannot do that.

  --verify <id>   re-derive a stored checkpoint and report whether it still holds.
                  A mismatch means the journal beneath it changed, which — given the
                  hash chain — means someone rewrote history.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			switch {
			case verify != "":
				ck, err := s.LoadCheckpoint(verify)
				if err != nil {
					return err
				}
				v, err := s.VerifyCheckpoint(ck)
				if err != nil {
					return err
				}
				if o.asJSON {
					return emitJSON(out, v)
				}
				if v.Reproducible {
					fmt.Fprintf(out, "%s: REPRODUCIBLE — re-derived from the journal at watermark %d, digests match\n", v.ID, v.Watermark)
					return nil
				}
				fmt.Fprintf(out, "%s: NOT REPRODUCIBLE — %s\n", v.ID, v.Reason)
				fmt.Fprintf(out, "  stored board:   %s\n  derived board:  %s\n", v.StoredDigest, v.DerivedDigest)
				fmt.Fprintf(out, "  stored journal: %s\n  derived journal:%s\n", v.StoredHead, v.DerivedHead)
				return fmt.Errorf("checkpoint %s no longer re-derives from the journal — the history beneath it changed", v.ID)

			case list:
				cks, err := s.ListCheckpoints()
				if err != nil {
					return err
				}
				if o.asJSON {
					return emitJSON(out, cks)
				}
				if len(cks) == 0 {
					fmt.Fprintln(out, "no checkpoints stored (the journal may still remember ones whose files are gone — see `steward history`)")
					return nil
				}
				tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "  ID\tCREATED\tWATERMARK\tWORKSTREAMS\tDEGRADED")
				for _, ck := range cks {
					fmt.Fprintf(tw, "  %s\t%s\t%d\t%d\t%d\n",
						ck.ID, ck.CreatedAt.Format(time.RFC3339), ck.Watermark, len(ck.Board.Workstreams), len(ck.Degraded))
				}
				tw.Flush()
				return nil
			}

			ck, err := s.Checkpoint(Self(), note, time.Now())
			if err != nil {
				return err
			}
			if o.asJSON {
				return emitJSON(out, ck)
			}
			fmt.Fprintf(out, "checkpoint %s\n", ck.ID)
			fmt.Fprintf(out, "  watermark:      %d\n", ck.Watermark)
			fmt.Fprintf(out, "  journal digest: %s\n", ck.JournalDigest)
			fmt.Fprintf(out, "  board digest:   %s\n", ck.Board.Digest)
			fmt.Fprintf(out, "  workstreams:    %d\n", len(ck.Board.Workstreams))
			if len(ck.Degraded) > 0 {
				fmt.Fprintf(out, "  CARRIED FORWARD — %d unresolved claim(s); a checkpoint that dropped its unknowns\n"+
					"                    would look like a clean bill of health:\n", len(ck.Degraded))
				for _, d := range ck.Degraded {
					fmt.Fprintf(out, "    %s\n", d)
				}
			}
			fmt.Fprintf(out, "\nVerify it any time: `steward checkpoint --verify %s`\n", ck.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "why this checkpoint is being taken")
	cmd.Flags().BoolVar(&list, "list", false, "list stored checkpoints")
	cmd.Flags().StringVar(&verify, "verify", "", "re-derive a stored checkpoint and report whether it still holds")
	return cmd
}

// ─── reconcile ────────────────────────────────────────────────────────────────

func newReconcileCmd(o *opts) *cobra.Command {
	var (
		record     bool
		repairTail bool
	)
	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "what can and cannot be established — the verb a successor runs FIRST",
		Long: `reconcile compares the journal against reality and reports what it can and cannot
establish.

It is the verb a successor runs BEFORE touching anything: who holds the seat, whether
the journal is intact, which claims are unproven, and which artifacts have gone
missing. That is the difference between inheriting a SYSTEM and inheriting a STORY
about a system.

The result is allowed — required, even — to say "I don't know". A reconciliation that
always produced a clean verdict would be worthless; the only useful thing it can do is
tell you precisely where the record runs out.

  ok        the journal is intact and every claim in it is established
  degraded  the record is readable, but something in it could not be established
  unknown   the record ITSELF is damaged. What survives is still valid; what came
            after it cannot be spoken for.

There is deliberately no "failed". This subsystem never reports success in the face of
missing evidence — and it never invents a failure it cannot prove either.

  --record       append the reconciliation to the journal, so "we checked, and here is
                 what we could not establish" becomes permanent rather than printed
                 once and lost. Its outcome mirrors the verdict, so a reconciliation
                 that found damage can never be replayed later as a success.
  --repair-tail  truncate an unreadable journal tail. It cuts ONLY the bytes after the
                 last entry that verified — a valid entry can never be removed — and
                 records the repair. Explicit and human-invoked on purpose: a log that
                 silently healed itself would be worthless, because "it repaired
                 itself" and "someone tampered with it" would look identical.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			now := time.Now()

			if repairTail {
				discarded, err := s.Repair(Self(), now)
				if err != nil {
					return err
				}
				if discarded == 0 {
					fmt.Fprintln(out, "journal tail is intact — nothing to repair")
				} else {
					fmt.Fprintf(out, "repaired: discarded %d unreadable trailing byte(s) after the last valid entry.\n", discarded)
					fmt.Fprintln(out, "No valid entry was removed — the cut is at the last byte that verified.")
				}
			}

			r, err := s.Reconcile(now)
			if err != nil {
				return err
			}
			if record {
				if _, err := s.RecordReconciliation(Self(), r, now); err != nil {
					// Recording needs the seat. Not being the holder must not swallow the
					// REPORT — the reader still gets the truth, and is told why it was
					// not written down.
					fmt.Fprintf(cmd.ErrOrStderr(), "note: the report below was NOT written to the journal: %v\n\n", err)
				} else {
					// Re-derive: the reconcile entry we just wrote is itself history now.
					if r2, err := s.Reconcile(now); err == nil {
						r = r2
					}
				}
			}

			if o.asJSON {
				return emitJSON(out, r)
			}

			fmt.Fprintf(out, "reconciliation: %s\n\n", strings.ToUpper(string(r.Health)))

			switch r.Seat.Liveness {
			case LivenessVacant:
				fmt.Fprintln(out, "seat:     VACANT")
			default:
				fmt.Fprintf(out, "seat:     %s (epoch %d) — %s\n",
					holderName(r.Seat.Authority.Holder), r.Seat.Authority.Epoch, r.Seat.Liveness)
			}

			fmt.Fprintf(out, "journal:  %d entries, ", r.JournalEntries)
			if r.JournalIntact {
				fmt.Fprintf(out, "intact (head %s)\n", short8(r.JournalHead))
			} else {
				fmt.Fprintf(out, "DAMAGED — %s\n", r.CorruptTail)
				fmt.Fprintln(out, "          `steward reconcile --repair-tail` truncates the unreadable bytes and nothing else")
			}
			fmt.Fprintf(out, "board:    %d workstream(s)\n", len(r.Board.Workstreams))

			if len(r.Unproven) > 0 {
				fmt.Fprintf(out, "\nUNPROVEN — %d claim(s) you must not take on faith:\n", len(r.Unproven))
				for _, u := range r.Unproven {
					fmt.Fprintf(out, "  seq %-4d [%s] claimed %s, effective %s\n", u.Seq, orDash(u.Workstream), u.Claimed, u.Effective)
					fmt.Fprintf(out, "           %s\n           why: %s\n", u.Summary, u.Why)
				}
			}
			if len(r.MissingArtifacts) > 0 {
				fmt.Fprintf(out, "\nmissing artifacts (%d) — OPTIONAL by contract; no projection depends on them,\n"+
					"and the board is bit-identical without them:\n", len(r.MissingArtifacts))
				for _, m := range r.MissingArtifacts {
					fmt.Fprintf(out, "  %s\n", m)
				}
			}
			if len(r.TamperedArtifacts) > 0 {
				fmt.Fprintf(out, "\nTAMPERED ARTIFACTS (%d) — an absent artifact is a gap; an ALTERED one is a lie:\n", len(r.TamperedArtifacts))
				for _, t := range r.TamperedArtifacts {
					fmt.Fprintf(out, "  %s\n", t)
				}
			}
			if len(r.CheckpointsVerified) > 0 {
				fmt.Fprintln(out, "\ncheckpoints:")
				for _, v := range r.CheckpointsVerified {
					if v.Reproducible {
						fmt.Fprintf(out, "  %s: reproducible\n", v.ID)
					} else {
						fmt.Fprintf(out, "  %s: NOT REPRODUCIBLE — %s\n", v.ID, v.Reason)
					}
				}
			}

			switch r.Health {
			case HealthOK:
				fmt.Fprintln(out, "\nEverything in the record is established. Safe to proceed.")
			case HealthDegraded:
				fmt.Fprintln(out, "\nThe record is readable but INCOMPLETE. Treat the unproven claims above as open questions —")
				fmt.Fprintln(out, "they are exactly the things a previous agent said were done and could not show for.")
			case HealthUnknown:
				fmt.Fprintln(out, "\nThe RECORD ITSELF is damaged. What survives above is valid; what came after it cannot be")
				fmt.Fprintln(out, "spoken for. Do not treat the absence of an entry as proof that nothing happened.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&record, "record", false, "append the reconciliation to the journal (requires holding the seat)")
	cmd.Flags().BoolVar(&repairTail, "repair-tail", false, "truncate an unreadable journal tail — only the bytes after the last valid entry")
	return cmd
}

// ─── writes ───────────────────────────────────────────────────────────────────

func newRecordCmd(o *opts) *cobra.Command {
	var (
		workstream, ref, summary, rationale, outcome string
		evidence                                     []string
		observation                                  bool
		epoch                                        uint64
	)
	cmd := &cobra.Command{
		Use:   "record",
		Short: "append an evidence-bearing effect or observation to the journal",
		Long: `record appends an AUTHORITATIVE entry: something happened in the world.

Bring evidence. An entry claiming --outcome success with no -e/--evidence does not
project as success — it projects as UNKNOWN, on every board, in every checkpoint,
forever. The claim is still recorded faithfully (it is an honest record of what was
asserted), but no view will ever promote it into a fact.

That rule is the point of the whole subsystem. An agent writes fluent, confident
prose about work it did not do; the only defense that scales is to refuse to launder
an unevidenced claim into a fact.

Evidence is anything a skeptic could go and check:

  command:go test ./...        file:/tmp/build.log#sha256:abc…
  commit:de6485c               test:TestFencing
  url:https://…                note:asked the human, they confirmed

Only the holder of the seat may write, and only at the CURRENT epoch: a steward that
lapsed and was taken over is fenced, and its writes are rejected loudly.

A long-running steward should capture its epoch at claim time and pass it with
--epoch. That is the whole point of holding a fencing token: if the seat moved on
while you were away, the write is REJECTED rather than silently interleaved with the
new steward's — and you are told so, instead of quietly corrupting the record.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			if strings.TrimSpace(summary) == "" {
				return fmt.Errorf("steward: a record needs a summary (-m)")
			}
			evs, err := parseEvidenceList(evidence)
			if err != nil {
				return err
			}
			kind := KindEffect
			if observation {
				kind = KindObservation
			}
			e, err := s.Record(Entry{
				Actor:      Self(),
				Kind:       kind,
				Workstream: workstream,
				Ref:        ref,
				Summary:    summary,
				Rationale:  rationale,
				Outcome:    Outcome(outcome),
				Evidence:   evs,
			}, epoch, time.Now())
			if err != nil {
				return err
			}
			if o.asJSON {
				return emitJSON(cmd.OutOrStdout(), e)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "recorded seq %d (%s, epoch %d)\n", e.Seq, e.Kind, e.Epoch)
			if eff := e.EffectiveOutcome(); eff != e.Outcome {
				fmt.Fprintf(out, "\nNOTE: you claimed %q with NO EVIDENCE, so the board will show %q.\n", e.Outcome, eff)
				fmt.Fprintln(out, "The claim is recorded exactly as you made it — but nothing will project it as a fact.")
				fmt.Fprintln(out, "Add something checkable: -e \"command:…\" -e \"commit:…\" -e \"test:…\"")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&summary, "message", "m", "", "what happened (required)")
	cmd.Flags().StringVar(&workstream, "workstream", "", "the strand of work this belongs to")
	cmd.Flags().StringVar(&ref, "ref", "", "a free pointer (issue, PR, host, service)")
	cmd.Flags().StringVar(&outcome, "outcome", "", "success | failed | degraded | unknown")
	cmd.Flags().StringVar(&rationale, "rationale", "", "why")
	cmd.Flags().StringSliceVarP(&evidence, "evidence", "e", nil, "checkable reference: kind:ref[#sha256:…] (repeatable)")
	cmd.Flags().BoolVar(&observation, "observation", false, "you OBSERVED this rather than caused it")
	cmd.Flags().Uint64Var(&epoch, "epoch", 0,
		"present the fencing token you captured at claim time; the write is REJECTED if the seat has moved on (0 = whatever you currently hold)")
	return cmd
}

func newDecideCmd(o *opts) *cobra.Command {
	var (
		workstream, summary, rationale string
		evidence                       []string
	)
	cmd := &cobra.Command{
		Use:   "decide",
		Short: "record an explicit, durable decision — what was decided, and WHY",
		Long: `decide records a DECISION: an explicit, durable statement of intent with a rationale.

A decision is authoritative on its own terms. It asserts INTENT, not effect, so it
needs a rationale rather than evidence — nothing about the world is being claimed.

This is the entry a successor reads to understand not just what happened on this host,
but what the previous steward had CONCLUDED and was steering toward. No amount of
replaying effects would ever recover that: effects tell you what was done, and only a
decision record tells you what it was for.`,
		Example: `  bashy steward decide --workstream api -m "drop the v1 endpoint" \
        --rationale "no callers in 90 days of logs; keeping it forces the auth shim to stay"`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			if strings.TrimSpace(summary) == "" {
				return fmt.Errorf("steward: a decision needs a summary (-m)")
			}
			if strings.TrimSpace(rationale) == "" {
				// A decision with no WHY is the one thing a successor cannot reconstruct.
				return fmt.Errorf("steward: a decision needs a rationale (--rationale): " +
					"a successor can replay every effect on this host and still never recover WHY you chose this")
			}
			evs, err := parseEvidenceList(evidence)
			if err != nil {
				return err
			}
			e, err := s.Decide(Self(), workstream, summary, rationale, evs, time.Now())
			if err != nil {
				return err
			}
			if o.asJSON {
				return emitJSON(cmd.OutOrStdout(), e)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "decision recorded: seq %d (epoch %d)\n", e.Seq, e.Epoch)
			return nil
		},
	}
	cmd.Flags().StringVarP(&summary, "message", "m", "", "what was decided (required)")
	cmd.Flags().StringVar(&rationale, "rationale", "", "why (required)")
	cmd.Flags().StringVar(&workstream, "workstream", "", "the strand of work this belongs to")
	cmd.Flags().StringSliceVarP(&evidence, "evidence", "e", nil, "supporting reference (repeatable)")
	return cmd
}

func newTranscriptCmd(o *opts) *cobra.Command {
	var workstream, summary, file string
	cmd := &cobra.Command{
		Use:   "transcript",
		Short: "attach an OPTIONAL, non-authoritative conversation artifact",
		Long: `transcript stores a conversation dump and records a hash-linked pointer to it.

It is NON-AUTHORITATIVE, and that is a contract, not a caveat. Nothing derives from a
transcript: delete every transcript artifact on this host and the board, the status,
the history, and every checkpoint are BIT-IDENTICAL. A test pins this
(TestTranscriptDeletionDoesNotAffectProjections) precisely so it cannot quietly stop
being true.

So why store one at all? A decision record says what was decided; a transcript lets a
human go back and see how the room got there. Useful — and never load-bearing.

Reads from --file, or from stdin.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			var src io.Reader = cmd.InOrStdin()
			if file != "" {
				f, err := os.Open(file)
				if err != nil {
					return err
				}
				defer f.Close()
				src = f
			}
			e, err := s.Transcript(Self(), workstream, summary, src, time.Now())
			if err != nil {
				return err
			}
			if o.asJSON {
				return emitJSON(cmd.OutOrStdout(), e)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "transcript recorded: seq %d — %s (%d bytes)\n", e.Seq, e.Artifact.Digest, e.Artifact.Bytes)
			fmt.Fprintln(cmd.OutOrStdout(), "Non-authoritative: no board, status, history, or checkpoint depends on it.")
			return nil
		},
	}
	cmd.Flags().StringVarP(&summary, "message", "m", "", "what this transcript is")
	cmd.Flags().StringVar(&workstream, "workstream", "", "the strand of work this belongs to")
	cmd.Flags().StringVar(&file, "file", "", "read the transcript from this file (default: stdin)")
	return cmd
}

func newWorkstreamCmd(o *opts) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workstream",
		Aliases: []string{"ws"},
		Short:   "open or close a strand of work",
	}

	var title string
	open := &cobra.Command{
		Use:   "open <name>",
		Short: "open a strand of work",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			e, err := s.OpenWorkstream(Self(), args[0], title, time.Now())
			if err != nil {
				return err
			}
			if o.asJSON {
				return emitJSON(cmd.OutOrStdout(), e)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "opened workstream %q (seq %d)\n", args[0], e.Seq)
			return nil
		},
	}
	open.Flags().StringVar(&title, "title", "", "a human title for the strand")

	var (
		summary  string
		outcome  string
		evidence []string
	)
	closeCmd := &cobra.Command{
		Use:   "close <name>",
		Short: "close a strand of work, with its outcome",
		Long: `close records that a strand of work is finished.

It does NOT force the outcome to success. If the closing entry claims success with no
evidence, the board still projects UNKNOWN — so "closed" and "verified done" remain
different facts, which is the entire difference between a status board and a wish
list.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := o.store()
			if err != nil {
				return err
			}
			evs, err := parseEvidenceList(evidence)
			if err != nil {
				return err
			}
			e, err := s.CloseWorkstream(Self(), args[0], summary, Outcome(outcome), evs, time.Now())
			if err != nil {
				return err
			}
			if o.asJSON {
				return emitJSON(cmd.OutOrStdout(), e)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "closed workstream %q (seq %d)\n", args[0], e.Seq)
			if eff := e.EffectiveOutcome(); eff != e.Outcome {
				fmt.Fprintf(out, "\nNOTE: closed claiming %q with NO EVIDENCE — the board will show %q.\n", e.Outcome, eff)
				fmt.Fprintln(out, "Closed is not the same fact as verified done.")
			}
			return nil
		},
	}
	closeCmd.Flags().StringVarP(&summary, "message", "m", "", "how it ended")
	closeCmd.Flags().StringVar(&outcome, "outcome", "", "success | failed | degraded | unknown")
	closeCmd.Flags().StringSliceVarP(&evidence, "evidence", "e", nil, "checkable reference (repeatable)")

	cmd.AddCommand(open, closeCmd)
	return cmd
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func emitJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// emitJSONLine emits one compact object per line — the shape a `--follow --json`
// consumer can read incrementally without waiting for a closing bracket.
func emitJSONLine(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v)
}

func writeEntry(out io.Writer, e Entry) {
	fmt.Fprintf(out, "seq %-4d %s  %-18s %s", e.Seq, e.Time, e.Kind, holderName(e.Actor))
	fmt.Fprintf(out, "  (epoch %d)", e.Epoch)
	if e.Workstream != "" {
		fmt.Fprintf(out, " [%s]", e.Workstream)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "         %s\n", e.Summary)
	if e.Rationale != "" {
		fmt.Fprintf(out, "         because: %s\n", e.Rationale)
	}
	if e.Outcome != "" {
		eff := e.EffectiveOutcome()
		if eff != e.Outcome {
			fmt.Fprintf(out, "         outcome: %s → %s  (claimed success with NO EVIDENCE — not projected as a fact)\n", e.Outcome, eff)
		} else {
			fmt.Fprintf(out, "         outcome: %s\n", eff)
		}
	}
	for _, ev := range e.Evidence {
		fmt.Fprintf(out, "         evidence: %s:%s", ev.Kind, ev.Ref)
		if ev.Note != "" {
			fmt.Fprintf(out, " (%s)", ev.Note)
		}
		fmt.Fprintln(out)
	}
	if e.Artifact != nil {
		fmt.Fprintf(out, "         artifact: %s %s (non-authoritative)\n", e.Artifact.Path, short8(e.Artifact.Digest))
	}
}

// warnCorrupt tells the reader that what they just saw is a valid PREFIX, not
// necessarily the whole story. Printing the entries and staying silent about the
// damage would be the one dishonest thing this package could do.
func warnCorrupt(out io.Writer, rep *Replay) {
	if rep == nil || !rep.Corrupt {
		return
	}
	fmt.Fprintf(out, "\nWARNING: the journal's tail is unreadable from line %d (%s).\n", rep.CorruptLine, rep.CorruptReason)
	fmt.Fprintf(out, "The %d entries above ARE valid — a torn tail never hides the history before it.\n", len(rep.Entries))
	fmt.Fprintln(out, "Anything written after the tear cannot be spoken for. `steward reconcile --repair-tail` truncates only the unreadable bytes.")
}

func parseEvidenceList(in []string) ([]Evidence, error) {
	var out []Evidence
	for _, s := range in {
		ev, err := ParseEvidence(s)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}

// parseSince accepts an RFC3339 instant or a duration ago ("2h", "30m").
func parseSince(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().UTC().Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("steward: --since %q is neither an RFC3339 time nor a duration like 2h", s)
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > n {
		return s[:n-1] + "…"
	}
	return s
}

// short8 abbreviates a digest for human output; the full value is always in --json.
func short8(digest string) string {
	if i := strings.IndexByte(digest, ':'); i >= 0 && len(digest) > i+9 {
		return digest[:i+9] + "…"
	}
	return digest
}

func short(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
}
