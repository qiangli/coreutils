package weave

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/weavecli"
)

// weaveDefaultFleet is the canonical agent CLI roster used when --fleet is
// not given. It matches the autopilot/orchestrator fleet documented in the
// weave skill.
var weaveDefaultFleet = []string{"claude", "codex", "opencode", "aider"}

func newWeaveFleetCmd() *cobra.Command {
	var flags weaveOutputFlags
	var fleetCSV string
	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Show each fleet tool's availability (throttle cooldowns)",
		Long: `fleet reports, for each configured agent CLI, whether it is available
now or cooling down until a parsed usage-limit reset. This is the surface
the orchestrator queries before assigning a tool: a tool that hit a
provider/subscription throttle is recorded on cooldown by 'weave start',
and fleet shows when it becomes assignable again so the loop can fail over
to an available tool and re-engage the throttled one automatically.

The roster defaults to ` + fmt.Sprintf("%v", weaveDefaultFleet) + `; override
with --fleet claude,codex,...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWeaveFleet(cmd, fleetCSV, &flags)
		},
	}
	flags.attach(cmd)
	cmd.Flags().StringVar(&fleetCSV, "fleet", "", "Comma-separated tool roster (default claude,codex,opencode,aider)")
	return cmd
}

func runWeaveFleet(cmd *cobra.Command, fleetCSV string, flags *weaveOutputFlags) error {
	mode := flags.mode()
	cwd, _ := os.Getwd()
	root, err := weaveRepoRoot(cwd)
	if err != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave fleet",
			weavecli.ExitPrecondFail, err))
	}
	dir, err := weaveQueueDir(root)
	if err != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave fleet",
			weavecli.ExitGenericFail, err))
	}

	fleet := parseWeaveAutopilotFleet(fleetCSV)
	if len(fleet) == 0 {
		fleet = append([]string(nil), weaveDefaultFleet...)
	}

	now := time.Now()
	type fleetRow struct {
		Tool        string `json:"tool"`
		Available   bool   `json:"available"`
		CoolingUnit string `json:"cooling_until,omitempty"` // RFC3339 local
	}
	rows := make([]fleetRow, 0, len(fleet))
	for _, tool := range fleet {
		row := fleetRow{Tool: tool, Available: true}
		if until, ok := toolAvailableAt(dir, tool); ok && until.After(now) {
			row.Available = false
			row.CoolingUnit = until.Local().Format(time.RFC3339)
		}
		rows = append(rows, row)
	}

	if mode == weavecli.OutputJSON {
		tools := make([]map[string]any, len(rows))
		for i, r := range rows {
			m := map[string]any{"tool": r.Tool, "available": r.Available}
			if r.CoolingUnit != "" {
				m["cooling_until"] = r.CoolingUnit
			}
			tools[i] = m
		}
		return ec(weavecli.EmitOK(cmd.OutOrStdout(), mode, "weave fleet", map[string]any{
			"tools": tools,
		}))
	}

	for _, r := range rows {
		if r.Available {
			fmt.Fprintf(cmd.OutOrStdout(), "%-12s available\n", r.Tool)
			continue
		}
		until, _ := toolAvailableAt(dir, r.Tool)
		fmt.Fprintf(cmd.OutOrStdout(), "%-12s cooling until %s\n", r.Tool, until.Local().Format("15:04"))
	}
	return ec(weavecli.EmitOK(cmd.OutOrStdout(), mode, "weave fleet", nil))
}
