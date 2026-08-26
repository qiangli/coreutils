package bus

// `bashy inbox` is the receive-side front door for 1:1 bus notifications.
//
// It deliberately owns no store. Notifications remain room.EventNotify records
// in the bus timeline, and the read position remains the bus's per-subscriber
// drain cursor. This command only fixes the filter to the reader's own address
// and gives that common operation the small flag surface shared by the other
// communication commands.

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/room"
)

// NewInboxCmd returns the top-level `inbox` command.
func NewInboxCmd() *cobra.Command {
	var as string
	var wait time.Duration
	var peek, jsonOut bool
	var limit int

	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "read 1:1 notifications waiting for you",
		Long: `inbox reads notifications addressed to you from the existing bus.

  bashy inbox
  bashy inbox --wait 15m
  bashy inbox --peek
  bashy inbox --limit 10 --json

Reading advances your existing per-subscriber bus cursor. --peek leaves it in
place, and --wait is bounded: a timeout is an empty successful read.`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if wait < 0 {
				return fmt.Errorf("inbox: --wait must not be negative")
			}
			if limit < 0 {
				return fmt.Errorf("inbox: --limit must not be negative")
			}
			who, err := BoardIdentity(as)
			if err != nil {
				return err
			}
			return runInbox(cmd, inboxOptions{
				reader: who, wait: wait, peek: peek, limit: limit, jsonOut: jsonOut,
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&as, "as", "", "read as this identity (default: resolved from your principal)")
	f.DurationVar(&wait, "wait", 0, "wait up to this duration for a new notification")
	f.BoolVar(&peek, "peek", false, "read without advancing your cursor")
	f.IntVarP(&limit, "limit", "n", 0, "print at most this many notifications (0 = no cap)")
	f.BoolVar(&jsonOut, "json", false, "emit one "+SchemaVersion+" JSON object per line (NDJSON)")
	return cmd
}

type inboxOptions struct {
	reader  string
	wait    time.Duration
	peek    bool
	limit   int
	jsonOut bool
}

func runInbox(cmd *cobra.Command, opt inboxOptions) error {
	from, err := readCursor(opt.reader)
	if err != nil {
		return err
	}
	filter := eventFilter{to: opt.reader}
	if opt.wait > 0 {
		// This is the bus watch bounded-drain primitive, including its stat-before-
		// parse loop and empty-success timeout contract. inbox adds no poller.
		if err := waitForDrain(cmd.Context(), filter, from, opt.wait); err != nil {
			return err
		}
	}

	events, err := watchTimeline(0)
	if err != nil {
		return err
	}
	matched := make([]room.Event, 0)
	high := from
	for _, e := range events {
		if e.Seq <= from {
			continue
		}
		if e.Seq > high {
			high = e.Seq
		}
		if filter.match(e) {
			matched = append(matched, e)
			if opt.limit > 0 && len(matched) == opt.limit {
				// Stop at the last item shown. Advancing beyond it would silently
				// consume a notification omitted only because of --limit.
				high = e.Seq
				break
			}
		}
	}

	for _, e := range matched {
		if err := writeEvent(cmd, e, opt.jsonOut); err != nil {
			return err
		}
	}
	if !opt.peek && high > from {
		if err := writeCursor(opt.reader, high); err != nil {
			return err
		}
	}
	if len(matched) == 0 && !opt.jsonOut {
		fmt.Fprintln(cmd.ErrOrStderr(), "nothing new")
	}
	return nil
}
