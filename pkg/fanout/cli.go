package fanout

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/chat"
)

// DefaultRoot resolves where boards live: $BASHY_FANOUT_DIR, else
// <repo-root>/.agents/bashy/fanout (co-located with graph-contrib), else
// <cwd>/.agents/bashy/fanout.
func DefaultRoot() string {
	if d := os.Getenv("BASHY_FANOUT_DIR"); d != "" {
		return d
	}
	cwd, _ := os.Getwd()
	root := repoRoot(cwd)
	if root == "" {
		root = cwd
	}
	return filepath.Join(root, ".agents", "bashy", "fanout")
}

// repoRoot walks up to a .git directory; "" if none.
func repoRoot(start string) string {
	dir := start
	for {
		if fi, err := os.Stat(filepath.Join(dir, ".git")); err == nil && fi.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// boardName resolves --board > $BASHY_BOARD > "default".
func boardName(flag string) string {
	if flag != "" {
		return flag
	}
	if b := os.Getenv("BASHY_BOARD"); b != "" {
		return b
	}
	return "default"
}

// principal is who a post is attributed to: the launcher-injected principal,
// then the short agent id, then a generic fallback.
func principal() string {
	for _, k := range []string{"BASHY_PRINCIPAL", "BASHY_AGENT_ID", "WEAVE_AGENT"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "operator"
}

// NewFanoutCmd builds the `bashy fanout` command tree.
func NewFanoutCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "fanout",
		Short: "Run parallel agents against one shared context (the blackboard pattern)",
		Long: "Fan N agent instances out concurrently against ONE shared, evolving " +
			"context (a board): each runs a slightly different instruction, reads a " +
			"SCOPED view of the board, and posts contributions back.\n\n" +
			"See dhnt/docs/agentic-design-pattern-gaps.md.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.AddCommand(newStartCmd(), newRunCmd(), newPostCmd(), newReadCmd(), newStatusCmd())
	return c
}

func newStartCmd() *cobra.Command {
	var board, goal string
	var refs []string
	c := &cobra.Command{
		Use:           "start",
		Short:         "Create a board and seed its shared context",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(goal) == "" {
				return fmt.Errorf("fanout start: --goal is required")
			}
			b := Open(DefaultRoot(), boardName(board))
			if err := b.Seed(goal, refs, principal()); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "board %q seeded → %s\n", b.Name(), b.Path())
			return nil
		},
	}
	c.Flags().StringVar(&board, "board", "", "board name (default: $BASHY_BOARD or \"default\")")
	c.Flags().StringVar(&goal, "goal", "", "the shared goal / seed context")
	c.Flags().StringArrayVar(&refs, "ref", nil, "a file/kb reference for the seed (repeatable)")
	return c
}

func newRunCmd() *cobra.Command {
	var board, goal, instFile string
	var agents []string
	var inst []string
	var jobs int
	var timeout time.Duration
	var asJSON, dryRun bool
	c := &cobra.Command{
		Use:   "run",
		Short: "Fan agents out over the board",
		Long: "Each instruction line becomes one instance. Bind a tool→facet " +
			"explicitly with an `<agent> <scope>:` prefix (the division-of-labor " +
			"form, e.g. `codex code: …`, `agy tests: …`); a line with no agent " +
			"falls back to round-robin over --agents. --dry-run prints the plan " +
			"without launching.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(agents) == 0 {
				return fmt.Errorf("fanout run: --agents is required")
			}
			instructions, err := gatherInstructions(instFile, inst)
			if err != nil {
				return err
			}
			if len(instructions) == 0 {
				return fmt.Errorf("fanout run: no instructions (--instructions FILE or --inst \"…\")")
			}
			b := Open(DefaultRoot(), boardName(board))
			if strings.TrimSpace(goal) != "" {
				if err := b.Seed(goal, nil, principal()); err != nil {
					return err
				}
			} else if !b.Exists() {
				return fmt.Errorf("fanout run: board %q has no seed — pass --goal or run `fanout start` first", b.Name())
			}

			instances := pairInstances(agents, instructions)
			if dryRun {
				out := cmd.OutOrStdout()
				waves, werr := computeWaves(instances)
				if werr != nil {
					return werr
				}
				fmt.Fprintf(out, "board %q — %d instance(s) in %d wave(s):\n", b.Name(), len(instances), len(waves))
				for w, wave := range waves {
					fmt.Fprintf(out, "wave %d:\n", w)
					for _, idx := range wave {
						inst := instances[idx]
						after := ""
						if len(inst.Needs) > 0 {
							after = "  (after " + strings.Join(inst.Needs, ",") + ")"
						}
						fmt.Fprintf(out, "  %-12s %-10s %s%s\n", inst.Agent, inst.Scope, inst.Instruction, after)
					}
				}
				return nil
			}
			// Board-aware agents can call `bashy fanout {read,post}` mid-run; make
			// the board reachable to the children via the environment.
			os.Setenv("BASHY_BOARD", b.Name())
			os.Setenv("BASHY_FANOUT_DIR", DefaultRoot())

			results, err := Run(cmd.Context(), Config{
				Board: b, Instances: instances, Jobs: jobs, Timeout: timeout,
			}, chatLauncher{})
			if err != nil {
				return err
			}
			return reportRun(cmd, results, asJSON)
		},
	}
	c.Flags().StringVar(&board, "board", "", "board name (default: $BASHY_BOARD or \"default\")")
	c.Flags().StringVar(&goal, "goal", "", "seed/replace the board's shared context before running")
	c.Flags().StringSliceVar(&agents, "agents", nil, "comma-separated agent pool (tool:model or nick)")
	c.Flags().StringVar(&instFile, "instructions", "", "file of per-instance instructions, one per line")
	c.Flags().StringArrayVar(&inst, "inst", nil, "an inline instruction (repeatable)")
	c.Flags().IntVarP(&jobs, "jobs", "j", 0, "max concurrent instances (default: all)")
	c.Flags().DurationVar(&timeout, "timeout", 0, "per-instance timeout (0 = none)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print the plan (agent/scope/instruction) without launching")
	c.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return c
}

func newPostCmd() *cobra.Command {
	var board, scope string
	var tags []string
	c := &cobra.Command{
		Use:           "post <text>",
		Short:         "Append a contribution to the board (agent-side)",
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			b := Open(DefaultRoot(), boardName(board))
			text := strings.Join(args, " ")
			if err := b.Post(text, principal(), scope, tags, os.Getenv("BASHY_EPISODE")); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "posted to %q\n", b.Name())
			return nil
		},
	}
	c.Flags().StringVar(&board, "board", "", "board name (default: $BASHY_BOARD)")
	c.Flags().StringVar(&scope, "scope", "", "the lens this contribution belongs to")
	c.Flags().StringArrayVar(&tags, "tag", nil, "a tag (repeatable)")
	return c
}

func newReadCmd() *cobra.Command {
	var board, scope string
	var limit int
	var asJSON bool
	c := &cobra.Command{
		Use:           "read",
		Short:         "Read a scoped view of the board",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b := Open(DefaultRoot(), boardName(board))
			v, err := b.Read(scope, "", limit)
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(v)
			}
			fmt.Fprint(cmd.OutOrStdout(), v.Render())
			return nil
		},
	}
	c.Flags().StringVar(&board, "board", "", "board name (default: $BASHY_BOARD)")
	c.Flags().StringVar(&scope, "scope", "", "read only the slice relevant to this lens")
	c.Flags().IntVar(&limit, "limit", 0, "cap the number of contributions (0 = no cap)")
	c.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return c
}

func newStatusCmd() *cobra.Command {
	var board string
	var asJSON bool
	c := &cobra.Command{
		Use:           "status",
		Short:         "Show what has been posted to the board",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b := Open(DefaultRoot(), boardName(board))
			byAuthor, total, err := b.Status()
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"board": b.Name(), "total": total, "by_author": byAuthor,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "board %q: %d contributions\n", b.Name(), total)
			for who, n := range byAuthor {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-24s %d\n", who, n)
			}
			return nil
		},
	}
	c.Flags().StringVar(&board, "board", "", "board name (default: $BASHY_BOARD)")
	c.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return c
}

// gatherInstructions merges --inst flags and an --instructions file.
func gatherInstructions(file string, inline []string) ([]string, error) {
	var out []string
	out = append(out, inline...)
	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			out = append(out, line)
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// pairInstances turns instruction lines into instances. Each line may bind its
// tool→facet explicitly — the practical division-of-labor form:
//
//	codex code:     implement the rate limiter
//	agy tests:      write table tests
//	opencode data:  provide edge-case JSON
//
// Rules: if the first token (a trailing ':' is tolerated) names an agent in the
// pool, it is that instance's agent; a following `scope:` names the lens.
// A line with no explicit agent falls back to round-robin over the pool, so the
// old positional form still works.
func pairInstances(agents, instructions []string) []Instance {
	inPool := map[string]bool{}
	for _, a := range agents {
		inPool[strings.TrimSpace(a)] = true
	}
	out := make([]Instance, 0, len(instructions))
	rr := 0
	for i, line := range instructions {
		agent := ""
		text := line
		if fields := strings.Fields(line); len(fields) > 0 {
			if name := strings.TrimRight(fields[0], ":"); inPool[name] {
				agent = name
				text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), fields[0]))
			}
		}
		// Prefix grammar before the first ':' — `scope` or
		// `scope after dep1,dep2` (the dependency form). A natural-language
		// colon ("review this: thing") does not match and is left intact.
		scope := fmt.Sprintf("lens-%d", i+1)
		var needs []string
		if idx := strings.Index(text, ":"); idx > 0 && idx < 60 {
			pf := strings.Fields(strings.TrimSpace(text[:idx]))
			if len(pf) == 1 || (len(pf) >= 3 && pf[1] == "after") {
				scope = pf[0]
				if len(pf) >= 3 && pf[1] == "after" {
					for _, d := range strings.Split(strings.Join(pf[2:], ""), ",") {
						if d = strings.TrimSpace(d); d != "" {
							needs = append(needs, d)
						}
					}
				}
				text = strings.TrimSpace(text[idx+1:])
			}
		}
		if agent == "" && len(agents) > 0 {
			agent = strings.TrimSpace(agents[rr%len(agents)])
			rr++
		}
		out = append(out, Instance{Agent: agent, Instruction: text, Scope: scope, Needs: needs})
	}
	return out
}

func reportRun(cmd *cobra.Command, results []Result, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(results)
	}
	out := cmd.OutOrStdout()
	ok := 0
	for _, r := range results {
		status := "ok"
		if r.Err != nil {
			status = "ERR: " + r.Err.Error()
		} else if r.ExitCode != 0 {
			status = fmt.Sprintf("exit %d", r.ExitCode)
		} else {
			ok++
		}
		posted := ""
		if r.Posted {
			posted = " (posted)"
		}
		fmt.Fprintf(out, "  %-20s %-12s %s%s\n", r.Agent, r.Scope, status, posted)
	}
	fmt.Fprintf(out, "%d/%d instances ok\n", ok, len(results))
	return nil
}

// chatLauncher is the production Launcher: it drives a real agent via
// chat.Invoke (nil runner = the default exec launcher), reusing all of chat's
// resolution, principal-env injection, and model-flag handling.
type chatLauncher struct{}

func (chatLauncher) Launch(ctx context.Context, agent, prompt string, timeout time.Duration) (string, int, error) {
	res, err := chat.Invoke(ctx, chat.Options{
		Agent:       agent,
		Instruction: prompt,
		Timeout:     timeout,
	}, nil)
	return res.Output, res.ExitCode, err
}
