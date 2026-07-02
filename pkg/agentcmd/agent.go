package agentcmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/qiangli/coreutils/pkg/sdlc"
	"github.com/spf13/cobra"
)

const schemaVersion = "bashy-agent-v1"

type WhoAmIResult struct {
	SchemaVersion string `json:"schema_version"`
	Agent         string `json:"agent"`
	Source        string `json:"source"`
}

func NewAgentCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "agent identity and local agent helpers"}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddCommand(newWhoamiCmd())
	return cmd
}

func newWhoamiCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "print the active operator agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			res := WhoAmI()
			if asJSON || os.Getenv("BASHY_AGENTIC") != "" {
				b, _ := json.Marshal(res)
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), res.Agent)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func WhoAmI() WhoAmIResult {
	return WhoAmIResult{
		SchemaVersion: schemaVersion,
		Agent:         sdlc.DefaultConductorAgent(),
		Source:        "environment",
	}
}
