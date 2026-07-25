package bus

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/room"
)

// defaultPoll is how often follow mode re-reads the timeline. The log is a local
// append-only file, so polling is both cheap and reliable — a watcher that was
// not running when an event landed still sees it on its next read, which a
// push-only subscription could not offer.
const defaultPoll = time.Second

func newWatchCmd() *cobra.Command {
	var topic, to, roomID, as string
	var drain, jsonOut, all bool
	var poll time.Duration
	var since int64

	cmd := &cobra.Command{
		Use:     "watch [flags]",
		Aliases: []string{"sub", "subscribe"},
		Short:   "watch the bus for notifications (follow, or drain what you missed)",
		Long: `watch reads notifications off the host room's timeline.

Two modes, and the difference matters:

  --drain    print everything published since YOU last drained, then exit.
             This is the mode an orchestrator wants: it is not "what is on the
             bus", it is "what have I not seen", so nothing is missed just
             because nobody was watching at the time.

  (default)  follow: print matching notifications as they arrive, until
             interrupted.

Filters narrow what counts as yours: --topic, --to, --room. With no filter,
watch shows every notification (--all makes that explicit, and is required in
drain mode so a cursor is never advanced over messages you did not mean to
claim).

Drain position is per-subscriber, named by --as (default: your principal), so two
agents draining the same topic each get their own copy.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			filter := eventFilter{topic: topic, to: to, room: roomID}

			if !filter.any() && !all {
				return fmt.Errorf("watch: no filter given — pass --topic/--to/--room, or --all to watch everything")
			}

			if drain {
				return runDrain(cmd, filter, drainOptions{
					as: resolveSubscriber(as), since: since, jsonOut: jsonOut,
				})
			}
			return runFollow(cmd, filter, followOptions{
				since: since, jsonOut: jsonOut, poll: poll,
			})
		},
	}

	f := cmd.Flags()
	f.StringVar(&topic, "topic", "", "only notifications on this topic")
	f.StringVar(&to, "to", "", "only notifications addressed to this session or role")
	f.StringVar(&roomID, "room", "", "only notifications in this room")
	f.BoolVar(&all, "all", false, "watch every notification (required when no filter is given)")
	f.BoolVar(&drain, "drain", false, "print what you have not seen since your last drain, then exit")
	f.StringVar(&as, "as", "", "subscriber name for the drain cursor (default: your principal)")
	f.Int64Var(&since, "since", 0, "start after this sequence number (overrides the saved cursor)")
	f.DurationVar(&poll, "poll", defaultPoll, "how often follow mode re-reads the timeline")
	f.BoolVar(&jsonOut, "json", false, "emit one "+SchemaVersion+" JSON object per line (NDJSON)")
	return cmd
}

// eventFilter decides which notifications belong to this subscriber.
type eventFilter struct{ topic, to, room string }

func (f eventFilter) any() bool { return f.topic != "" || f.to != "" || f.room != "" }

// match reports whether an event is one this subscriber asked for.
//
// Only notify events are ever matched. The timeline also carries join/leave/
// steer/status entries — session bookkeeping that is not addressed to anyone and
// would be noise on a bus subscription.
func (f eventFilter) match(e room.Event) bool {
	if e.Type != room.EventNotify {
		return false
	}
	if f.topic != "" && e.Topic != f.topic {
		return false
	}
	if f.to != "" && e.To != f.to {
		return false
	}
	if f.room != "" && e.Room != f.room {
		return false
	}
	return true
}

type drainOptions struct {
	as      string
	since   int64
	jsonOut bool
}

// runDrain prints what this subscriber has not seen, then advances its cursor.
//
// The cursor is advanced only AFTER the events have been written out. A drain
// that updated the cursor first and then failed to print would silently consume
// the messages it was supposed to deliver — the reader would never learn they
// existed, and there is no way to ask for them again.
func runDrain(cmd *cobra.Command, f eventFilter, opt drainOptions) error {
	from := opt.since
	if from == 0 {
		c, err := readCursor(opt.as)
		if err != nil {
			return err
		}
		from = c
	}

	events, err := room.Timeline(0)
	if err != nil {
		return err
	}

	var matched []room.Event
	var high int64
	for _, e := range events {
		if e.Seq > high {
			high = e.Seq
		}
		if e.Seq > from && f.match(e) {
			matched = append(matched, e)
		}
	}

	for _, e := range matched {
		if err := writeEvent(cmd, e, opt.jsonOut); err != nil {
			return err
		}
	}

	// Advance past everything READ, not merely everything matched: a cursor that
	// only moved over matching events would re-scan the whole log every drain, and
	// would rewind the moment a subscriber narrowed its filter.
	if high > from {
		if err := writeCursor(opt.as, high); err != nil {
			return err
		}
	}
	if len(matched) == 0 && !opt.jsonOut {
		fmt.Fprintln(cmd.ErrOrStderr(), "nothing new")
	}
	return nil
}

type followOptions struct {
	since   int64
	jsonOut bool
	poll    time.Duration
}

// runFollow streams matching notifications until interrupted.
//
// It starts from the CURRENT end of the log rather than replaying history,
// because "follow" means "tell me what happens from now on" — a subscriber that
// wanted the backlog would have asked to drain it. --since overrides that.
func runFollow(cmd *cobra.Command, f eventFilter, opt followOptions) error {
	last := opt.since
	if last == 0 {
		events, err := room.Timeline(0)
		if err != nil {
			return err
		}
		for _, e := range events {
			if e.Seq > last {
				last = e.Seq
			}
		}
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()

	if opt.poll <= 0 {
		opt.poll = defaultPoll
	}
	t := time.NewTicker(opt.poll)
	defer t.Stop()

	for {
		events, err := room.Timeline(0)
		if err != nil {
			return err
		}
		for _, e := range events {
			if e.Seq <= last {
				continue
			}
			last = e.Seq
			if f.match(e) {
				if werr := writeEvent(cmd, e, opt.jsonOut); werr != nil {
					return werr
				}
			}
		}
		select {
		case <-ctx.Done():
			// Interrupted is the NORMAL way to end a follow, not a failure.
			return nil
		case <-t.C:
		}
	}
}

// WatchEvent is one notification as `watch --json` emits it (NDJSON, one per
// line — a stream, so a reader can act on each as it arrives rather than waiting
// for a closing bracket that follow mode would never write).
type WatchEvent struct {
	SchemaVersion string `json:"schema_version"`
	Seq           int64  `json:"seq"`
	TS            string `json:"ts"`
	Principal     string `json:"principal,omitempty"`
	Topic         string `json:"topic,omitempty"`
	To            string `json:"to,omitempty"`
	Room          string `json:"room,omitempty"`
	Body          string `json:"body,omitempty"`
}

func writeEvent(cmd *cobra.Command, e room.Event, jsonOut bool) error {
	w := cmd.OutOrStdout()
	if jsonOut {
		b, err := json.Marshal(WatchEvent{
			SchemaVersion: SchemaVersion,
			Seq:           e.Seq, TS: e.TS,
			Principal: e.Principal, Topic: e.Topic, To: e.To, Room: e.Room,
			Body: e.Body,
		})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(b))
		return err
	}

	line := fmt.Sprintf("%s  <%s>", e.TS, e.Principal)
	var addr []string
	if e.Topic != "" {
		addr = append(addr, "#"+e.Topic)
	}
	if e.To != "" {
		addr = append(addr, "→"+e.To)
	}
	if e.Room != "" {
		addr = append(addr, "@"+e.Room)
	}
	if len(addr) > 0 {
		line += "  " + strings.Join(addr, " ")
	}
	if e.Body != "" {
		line += "  " + e.Body
	}
	_, err := fmt.Fprintln(w, line)
	return err
}
