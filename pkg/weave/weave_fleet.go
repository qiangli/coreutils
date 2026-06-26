package weave

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/weavecli"
)

// weaveDefaultFleet is the canonical agent CLI roster used when --fleet is
// not given. It matches the autopilot/orchestrator fleet documented in the
// weave skill.
var weaveDefaultFleet = []string{"claude", "codex", "opencode", "aider"}

// fleetProbeTTL bounds how long a cached --probe capability result is trusted
// before re-probing. Existence (PATH lookup) is cheap and never cached.
const fleetProbeTTL = time.Hour

func newWeaveFleetCmd() *cobra.Command {
	var flags weaveOutputFlags
	var fleetCSV string
	var probe bool
	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Show each fleet tool's availability (installed? on PATH? cooling down?)",
		Long: `fleet reports, for each configured agent CLI, whether it is assignable
right now — and why not if not. This is the surface an orchestrator queries
BEFORE assigning a tool, so it can skip tools it cannot launch and fail over
to ones it can:

  installed   the binary resolves on PATH (exec.LookPath) — a tool that is
              not installed/not on PATH is reported NOT FOUND, so the
              orchestrator never wastes a launch on it (the failure mode that
              otherwise surfaces only as a 0-second 'weave start' failure).
  cooldown    a tool that hit a provider/subscription throttle is recorded on
              cooldown by 'weave start'; fleet shows when it becomes
              assignable again.
  capability  with --probe, run '<tool> --version' once (3s timeout) and CACHE
              the result (` + fleetProbeTTL.String() + ` TTL) under the queue dir, so repeated
              preflights are instant.

A tool is "available" only if it is installed AND not cooling down.

The roster defaults to ` + fmt.Sprintf("%v", weaveDefaultFleet) + `; override
with --fleet claude,codex,...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWeaveFleet(cmd, fleetCSV, probe, &flags)
		},
	}
	flags.attach(cmd)
	cmd.Flags().StringVar(&fleetCSV, "fleet", "", "Comma-separated tool roster (default claude,codex,opencode,aider)")
	cmd.Flags().BoolVar(&probe, "probe", false, "Also probe capability (`<tool> --version`, 3s timeout) and cache it")
	return cmd
}

type fleetRow struct {
	Tool        string `json:"tool"`
	Available   bool   `json:"available"` // installed AND not cooling down
	Found       bool   `json:"found"`     // binary resolves on PATH
	Path        string `json:"path,omitempty"`
	CoolingUnit string `json:"cooling_until,omitempty"` // RFC3339 local
	Probed      bool   `json:"probed,omitempty"`
	Capable     bool   `json:"capable,omitempty"` // --version exited cleanly
	Version     string `json:"version,omitempty"`
}

// fleetProbeEntry is one tool's cached capability probe.
type fleetProbeEntry struct {
	Capable  bool      `json:"capable"`
	Version  string    `json:"version"`
	Path     string    `json:"path"`
	ProbedAt time.Time `json:"probed_at"`
}

func runWeaveFleet(cmd *cobra.Command, fleetCSV string, probe bool, flags *weaveOutputFlags) error {
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
	cache := loadFleetProbeCache(dir)
	dirty := false
	rows := make([]fleetRow, 0, len(fleet))
	for _, tool := range fleet {
		row, d := fleetRowFor(dir, tool, now, probe, cache)
		dirty = dirty || d
		rows = append(rows, row)
	}
	if dirty {
		saveFleetProbeCache(dir, cache)
	}

	if mode == weavecli.OutputJSON {
		tools := make([]map[string]any, len(rows))
		for i, r := range rows {
			m := map[string]any{"tool": r.Tool, "available": r.Available, "found": r.Found}
			if r.Path != "" {
				m["path"] = r.Path
			}
			if r.CoolingUnit != "" {
				m["cooling_until"] = r.CoolingUnit
			}
			if r.Probed {
				m["probed"], m["capable"] = true, r.Capable
				if r.Version != "" {
					m["version"] = r.Version
				}
			}
			tools[i] = m
		}
		return ec(weavecli.EmitOK(cmd.OutOrStdout(), mode, "weave fleet", map[string]any{"tools": tools}))
	}

	for _, r := range rows {
		switch {
		case !r.Found:
			fmt.Fprintf(cmd.OutOrStdout(), "%-12s NOT FOUND on PATH\n", r.Tool)
		case r.CoolingUnit != "":
			until, _ := toolAvailableAt(dir, r.Tool)
			fmt.Fprintf(cmd.OutOrStdout(), "%-12s cooling until %s\n", r.Tool, until.Local().Format("15:04"))
		case r.Probed && !r.Capable:
			fmt.Fprintf(cmd.OutOrStdout(), "%-12s installed but --version failed (%s)\n", r.Tool, r.Path)
		case r.Probed:
			fmt.Fprintf(cmd.OutOrStdout(), "%-12s available  %s (%s)\n", r.Tool, r.Version, r.Path)
		default:
			fmt.Fprintf(cmd.OutOrStdout(), "%-12s available  (%s)\n", r.Tool, r.Path)
		}
	}
	return ec(weavecli.EmitOK(cmd.OutOrStdout(), mode, "weave fleet", nil))
}

// fleetRowFor evaluates one tool's assignability: existence on PATH (always),
// throttle cooldown, and — under probe — a cached capability check. Returns the
// row and whether the probe cache was updated (so the caller can persist it).
// A tool is Available only if its binary resolves AND it is not cooling down.
func fleetRowFor(dir, tool string, now time.Time, probe bool, cache map[string]fleetProbeEntry) (fleetRow, bool) {
	row := fleetRow{Tool: tool}
	dirty := false
	// Existence: does the binary resolve on PATH? (cheap, never cached)
	if p, lookErr := exec.LookPath(tool); lookErr == nil {
		row.Found, row.Path = true, p
	}
	// Cooldown: recorded by a prior throttled `weave start`.
	cooling := false
	if until, ok := toolAvailableAt(dir, tool); ok && until.After(now) {
		cooling = true
		row.CoolingUnit = until.Local().Format(time.RFC3339)
	}
	// Capability (opt-in, cached): only meaningful if the binary exists.
	if probe && row.Found {
		row.Probed = true
		ent, fresh := cache[tool]
		if !fresh || now.Sub(ent.ProbedAt) > fleetProbeTTL || ent.Path != row.Path {
			ent = probeToolCapability(tool, row.Path, now)
			cache[tool] = ent
			dirty = true
		}
		row.Capable, row.Version = ent.Capable, ent.Version
	}
	row.Available = row.Found && !cooling
	return row, dirty
}

// probeToolCapability runs `<tool> --version` with a short timeout. A clean
// exit means the CLI is launchable; the first output line is kept as a version
// hint. This is the only place fleet executes a tool, and only under --probe.
func probeToolCapability(tool, path string, now time.Time) fleetProbeEntry {
	ent := fleetProbeEntry{Path: path, ProbedAt: now}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err == nil {
		ent.Capable = true
		if line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]); line != "" {
			ent.Version = line
		}
	}
	return ent
}

func fleetProbeCachePath(dir string) string { return filepath.Join(dir, "fleet-probe-cache.json") }

func loadFleetProbeCache(dir string) map[string]fleetProbeEntry {
	m := map[string]fleetProbeEntry{}
	if b, err := os.ReadFile(fleetProbeCachePath(dir)); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func saveFleetProbeCache(dir string, m map[string]fleetProbeEntry) {
	if b, err := json.MarshalIndent(m, "", "  "); err == nil {
		_ = os.WriteFile(fleetProbeCachePath(dir), b, 0o644)
	}
}
