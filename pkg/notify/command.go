package notify

import (
	"encoding/json"
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

// NewCommand builds `bashy notify`: publish a notification to the local bus
// store that `bashy watch` reads.
func NewCommand() *cobra.Command {
	var topic, to, roomID, principal string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "notify --topic <topic> [flags] <message>",
		Short: "Publish a notification to the local bus store",
		Long: `Publish a notification to the local bus store (the room timeline)
that 'bashy watch' subscribes to.

Addressing (at least one of --topic, --to, --room):
  --topic <t>    broadcast to a named topic
  --to <id>      1:1 delivery to a session or role
  --room <id>    room-scoped publish

Every publish must carry a principal (who sent it). Supply --principal,
set $BASHY_PRINCIPAL, or $USER is used as the default. A publish with
no principal is rejected — this is the REPORT/AUTHOR invariant.`,
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			msg := strings.Join(args, " ")
			who := resolvePrincipal(principal)
			if who == "" {
				return wrapErr("principal is required (REPORT/AUTHOR invariant): set --principal or $BASHY_PRINCIPAL", topic, roomID, to, msg, jsonOut, cmd)
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
	return enc.Encode(env)
}
