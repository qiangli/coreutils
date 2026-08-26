package bus

// `bashy notify` is the send-side front door for one subject-only bus event.
// It adds no message schema and no transport: the subject is Notification.Body,
// the addressee is Notification.To, and Publish remains the enforcement point.

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// MaxNotifySubjectBytes keeps a notification a doorbell rather than a message
// body. The command refuses overlong subjects; it never silently changes one.
const MaxNotifySubjectBytes = 256

// NotifyReceipt is the machine-readable result of one notify attempt.
type NotifyReceipt struct {
	SchemaVersion string `json:"schema_version"`
	State         string `json:"state"`
	Principal     string `json:"principal,omitempty"`
	To            string `json:"to,omitempty"`
	Subject       string `json:"subject,omitempty"`
	Error         string `json:"error,omitempty"`
}

// NewNotifyCmd returns the top-level `notify` command.
func NewNotifyCmd() *cobra.Command {
	var as, to string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "notify [--to <agent>] [<agent>] <subject>",
		Short: "send one subject-only notification to an agent or role",
		Long: `notify sends one subject-only notification through the existing bus.

  bashy notify meridian "meet 3 — your turn"
  bashy notify --to steward "nightly backup failed"

A notification initiates a conversation; it never hosts one. Put the place to
continue in the subject text. Bodies, attachments and a second pointer field are
deliberately absent.`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, subject, err := notifyOperands(to, args)
			if err != nil {
				return notifyFailure(cmd, jsonOut, "", target, subject, err)
			}
			if err := validateNotifySubject(subject); err != nil {
				return notifyFailure(cmd, jsonOut, "", target, subject, err)
			}
			principal, err := BoardIdentity(as)
			if err != nil {
				return notifyFailure(cmd, jsonOut, "", target, subject, err)
			}
			if strings.TrimSpace(principal) == "" || principal == "anonymous" {
				return notifyFailure(cmd, jsonOut, "", target, subject,
					fmt.Errorf("notify: sender identity is required; pass --as or set BASHY_PRINCIPAL"))
			}
			addr, kind, ok := resolveNotifyTarget(target)
			if !ok {
				return notifyFailure(cmd, jsonOut, principal, target, subject, unresolvedTargetError(target))
			}

			before := timelineHigh()
			if err := Publish(Notification{Principal: principal, To: addr, Body: subject}); err != nil {
				return notifyFailure(cmd, jsonOut, principal, target, subject, err)
			}
			state := StateAccepted
			if kind != TargetRole {
				if seq := findNotificationSeq(before, principal, addr, subject); seq > 0 {
					state = inboxDeliveryState(addr, seq)
				}
			}
			receipt := NotifyReceipt{
				SchemaVersion: SchemaVersion, State: state, Principal: principal,
				To: RoleLabelFor(addr), Subject: subject,
			}
			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(receipt)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", state, receipt.To)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&as, "as", "", "send as this identity (default: resolved from your principal)")
	f.StringVar(&to, "to", "", "recipient agent or role (explicit form of the first operand)")
	f.BoolVar(&jsonOut, "json", false, "emit a "+SchemaVersion+" delivery receipt")
	return cmd
}

func notifyOperands(to string, args []string) (target, subject string, err error) {
	to = strings.TrimSpace(to)
	switch {
	case to == "" && len(args) == 2:
		return strings.TrimSpace(args[0]), args[1], nil
	case to != "" && len(args) == 1:
		return to, args[0], nil
	case to != "" && len(args) == 2:
		return to, args[1], fmt.Errorf("notify: name the addressee once, either as <agent> or with --to")
	case to == "":
		return "", "", fmt.Errorf("notify: requires exactly <agent> and one quoted subject")
	default:
		return to, "", fmt.Errorf("notify: --to requires exactly one quoted subject")
	}
}

func validateNotifySubject(subject string) error {
	if strings.TrimSpace(subject) == "" {
		return fmt.Errorf("notify: subject is required")
	}
	if strings.ContainsAny(subject, "\r\n") {
		return fmt.Errorf("notify: subject must be one line; bodies are not supported")
	}
	if len(subject) > MaxNotifySubjectBytes {
		return fmt.Errorf("notify: subject is %d bytes; maximum is %d (refused, not truncated)", len(subject), MaxNotifySubjectBytes)
	}
	return nil
}

func notifyFailure(cmd *cobra.Command, jsonOut bool, principal, to, subject string, cause error) error {
	if !strings.HasPrefix(strings.ToLower(cause.Error()), StateFailed+":") {
		cause = fmt.Errorf("%s: %w", StateFailed, cause)
	}
	if !jsonOut {
		return cause
	}
	_ = json.NewEncoder(cmd.ErrOrStderr()).Encode(NotifyReceipt{
		SchemaVersion: SchemaVersion, State: StateFailed, Principal: principal,
		To: to, Subject: subject, Error: cause.Error(),
	})
	return reported("%s", cause)
}

// timelineHigh and findNotificationSeq recover the sequence assigned by the
// append-only room store without introducing a second write API. A failure to
// inspect the receipt after a successful Publish leaves the honest lower claim
// `accepted`; it never turns a successful durable write into a reported failure.
func timelineHigh() int64 {
	events, err := watchTimeline(0)
	if err != nil || len(events) == 0 {
		return 0
	}
	return events[len(events)-1].Seq
}

func findNotificationSeq(after int64, principal, to, subject string) int64 {
	events, err := watchTimeline(0)
	if err != nil {
		return 0
	}
	var seq int64
	for _, e := range events {
		if e.Seq > after && e.Type == "notify" && e.Principal == principal && e.To == to && e.Body == subject {
			seq = e.Seq
		}
	}
	return seq
}

// resolveNotifyTarget starts with the communication family's shared resolver,
// then accepts a name proven by this private channel's own cursor. That final
// case matters for a bus-only subscriber that has never joined the public board:
// its existing inbox is evidence, not a guessed identity.
func resolveNotifyTarget(target string) (addr, kind string, ok bool) {
	if addr, kind, ok := ResolveSendTarget(target); ok {
		return addr, kind, true
	}
	target = strings.TrimSpace(target)
	if _, ok := inboxCursorSeq(target); ok {
		return target, TargetReader, true
	}
	return "", "", false
}

// inboxDeliveryState applies the canonical Delivery vocabulary to the bus
// cursor (not the public board cursor). A missing or malformed cursor proves no
// reader has drained successfully, so the only honest state is unverified.
func inboxDeliveryState(reader string, seq int64) string {
	cur, ok := inboxCursorSeq(reader)
	if !ok {
		return StateUnverified
	}
	if cur >= seq {
		return StateRead
	}
	return StateQueued
}

func inboxCursorSeq(reader string) (int64, bool) {
	path, err := cursorPath(reader)
	if err != nil {
		return 0, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	cur, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0, false
	}
	return cur, true
}
