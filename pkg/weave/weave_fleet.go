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
var weaveDefaultFleet = []string{"claude", "codex", "opencode", "aider", "agy"}

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
	cmd.AddCommand(newWeaveFleetInterviewCmd())
	cmd.AddCommand(newWeaveFleetReviewCmd())
	return cmd
}

// newWeaveFleetReviewCmd implements
// `weave fleet review <tool> --role <role> --rating <0-5> --verdict "…" [--note …]`:
// records a qualitative suitability review for a role into the system-wide
// profile store, so the conductor's routing decisions are documented + tracked.
func newWeaveFleetReviewCmd() *cobra.Command {
	var flags weaveOutputFlags
	var role, verdict string
	var rating int
	var notes []string
	cmd := &cobra.Command{
		Use:   "review <tool>",
		Short: "Record a role-suitability review (rating + verdict) into the profile store",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := flags.mode()
			dir, err := weaveToolsDir()
			if err != nil {
				return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave fleet review", weavecli.ExitGenericFail, err))
			}
			if role == "" {
				return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave fleet review", weavecli.ExitGenericFail,
					fmt.Errorf("--role is required")))
			}
			if err := setRoleReview(dir, args[0], role, rating, verdict, notes, time.Now()); err != nil {
				return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave fleet review", weavecli.ExitGenericFail, err))
			}
			if mode != weavecli.OutputJSON {
				fmt.Fprintf(cmd.OutOrStdout(), "%s as %s: %d/5 — %s\n", args[0], role, rating, verdict)
			}
			return ec(weavecli.EmitOK(cmd.OutOrStdout(), mode, "weave fleet review",
				map[string]any{"tool": args[0], "role": role, "rating": rating, "verdict": verdict}))
		},
	}
	flags.attach(cmd)
	cmd.Flags().StringVar(&role, "role", "", "Role being reviewed (conductor|coder|qa|doc|…)")
	cmd.Flags().IntVar(&rating, "rating", 0, "Suitability rating 0–5")
	cmd.Flags().StringVar(&verdict, "verdict", "", "One-line verdict")
	cmd.Flags().StringArrayVar(&notes, "note", nil, "Rationale note (repeatable)")
	return cmd
}

// newWeaveFleetInterviewCmd implements `weave fleet interview [tool…|--all]`:
// the conductor's one-on-one calibration. It resolves each tool's binary +
// version and records its unattended launch contract into a persistent profile
// (~/.bashy/weave/tools/<tool>.json), seeding a first interview from the known
// contracts. This makes the headless launch recipe explicit + discoverable so a
// campaign never bare-launches a tool into its trust/welcome prompt.
func newWeaveFleetInterviewCmd() *cobra.Command {
	var flags weaveOutputFlags
	var all bool
	cmd := &cobra.Command{
		Use:   "interview [tool...]",
		Short: "Calibrate tool profiles (launch contract + version) into the profile store",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := flags.mode()
			dir, err := weaveToolsDir()
			if err != nil {
				return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave fleet interview",
					weavecli.ExitGenericFail, err))
			}
			tools := args
			if all || len(tools) == 0 {
				tools = append([]string(nil), weaveDefaultFleet...)
			}
			now := time.Now()
			profiles := make([]map[string]any, 0, len(tools))
			for _, tool := range tools {
				p, ierr := interviewTool(dir, tool, now)
				if ierr != nil {
					return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave fleet interview",
						weavecli.ExitGenericFail, ierr))
				}
				if mode != weavecli.OutputJSON {
					found := "NOT FOUND"
					if p.Bin != "" {
						found = p.Bin
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%-10s %s  %s\n", p.Tool, found, p.Version)
					fmt.Fprintf(cmd.OutOrStdout(), "  launch:  %s %s \"<body>\"\n", p.Tool, strings.Join(p.HeadlessArgs, " "))
					if p.TrustClear != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "  trust:   %s\n", p.TrustClear)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  steer:   say=%v  graceful-quit=%v\n", p.SupportsSay, p.SupportsGracefulQuit)
					if p.Notes != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "  notes:   %s\n", p.Notes)
					}
					for _, role := range sortedRoleNames(p) {
						r := p.Roles[role]
						if r.Verdict != "" {
							fmt.Fprintf(cmd.OutOrStdout(), "  role %-9s %d/5 — %s\n", role, r.Rating, r.Verdict)
						} else {
							fmt.Fprintf(cmd.OutOrStdout(), "  role %-9s %d/%d pass\n", role, r.Passed, r.Runs)
						}
					}
				}
				profiles = append(profiles, map[string]any{
					"tool": p.Tool, "bin": p.Bin, "version": p.Version,
					"headless_args": p.HeadlessArgs, "trust_clear": p.TrustClear,
					"supports_say": p.SupportsSay, "supports_graceful_quit": p.SupportsGracefulQuit,
				})
			}
			return ec(weavecli.EmitOK(cmd.OutOrStdout(), mode, "weave fleet interview", map[string]any{"profiles": profiles}))
		},
	}
	flags.attach(cmd)
	cmd.Flags().BoolVar(&all, "all", false, "Interview the whole default fleet")
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
