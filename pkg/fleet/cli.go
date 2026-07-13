package fleet

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/assetring"
)

// ExitCode maps a fleet command's Execute error to the repo exit
// convention: 2 usage, 1 otherwise, 0 for nil.
func ExitCode(err error) int { return assetring.ExitCode(err) }

// NewToolsCmd builds the `tools` verb tree.
func NewToolsCmd(opts ...Option) *cobra.Command {
	return newRoot("tools", "Agentic CLI harnesses the fleet can drive",
		newToolsList(opts),
		newToolsShow(opts),
		newToolsAdd(opts),
		newToolsSet(opts),
		newRm(KindTool, opts, (*Catalog).RemoveTool),
		newEdit(KindTool, opts, (*Catalog).MaterializeTool),
		newSync(KindTool, opts),
		newVerify(KindTool, opts, func(c *Catalog, n string) Check {
			return c.VerifyTool(n, Probes(nil))
		}),
	)
}

// NewModelsCmd builds the `models` verb tree.
func NewModelsCmd(opts ...Option) *cobra.Command {
	return newRoot("models", "Inference backends the fleet can bind to",
		newModelsList(opts),
		newModelsShow(opts),
		newModelsAdd(opts),
		newModelsSet(opts),
		newRm(KindModel, opts, (*Catalog).RemoveModel),
		newEdit(KindModel, opts, (*Catalog).MaterializeModel),
		newSync(KindModel, opts),
		newVerify(KindModel, opts, func(c *Catalog, n string) Check {
			return c.VerifyModel(n, Probes(nil))
		}),
	)
}

// NewAgentsCmd builds the `agents` verb tree.
func NewAgentsCmd(opts ...Option) *cobra.Command {
	return newRoot("agents", "Named tool:model bindings — the enlistable unit",
		newAgentsList(opts),
		newAgentsShow(opts),
		newAgentsAdd(opts),
		newAgentsSet(opts),
		newRm(KindAgent, opts, (*Catalog).RemoveAgent),
		newEdit(KindAgent, opts, (*Catalog).MaterializeAgent),
		newSync(KindAgent, opts),
		newVerify(KindAgent, opts, func(c *Catalog, n string) Check {
			return c.VerifyAgent(n, Probes(nil))
		}),
	)
}

// newRoot wires a noun's verb tree. The bare noun is its `list` verb, so
// `bashy tools` and `bashy tools list` agree — the same shorthand
// `bashy skills` already offers.
func newRoot(name, short string, list *cobra.Command, rest ...*cobra.Command) *cobra.Command {
	c := &cobra.Command{
		Use:           name,
		Short:         short,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          list.RunE,
	}
	c.Flags().AddFlagSet(list.Flags())
	c.CompletionOptions.DisableDefaultCmd = true
	c.AddCommand(list)
	c.AddCommand(rest...)
	return c
}

// --- tools --------------------------------------------------------------

type toolRow struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Aliases []string `json:"aliases,omitempty"`
	Binary  string   `json:"binary,omitempty"`
	Model   bool     `json:"selects_model"`
	Ring    string   `json:"ring"`
}

func newToolsList(opts []Option) *cobra.Command {
	var asJSON, all bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List agentic CLI tools",
		Long: "List agentic CLI tools.\n\n" +
			"The asset registry's tool namespace is shared with MCP-style function kits;\n" +
			"only kind:cli entries are fleet tools. --all shows the rest.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := New(opts...)
			tools, errs := c.Tools(all)
			rows := make([]toolRow, 0, len(tools))
			for _, t := range tools {
				rows = append(rows, toolRow{
					Name: t.Name, Kind: t.Kind, Aliases: t.Aliases,
					Binary: t.CLI.Binary, Model: t.TakesModel(), Ring: t.Ring.String(),
				})
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), rows)
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tKIND\tBINARY\tMODEL\tRING")
			for _, r := range rows {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.Name, r.Kind, r.Binary, yesNo(r.Model), r.Ring)
			}
			tw.Flush()
			return reportParseErrs(cmd.ErrOrStderr(), errs)
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	c.Flags().BoolVar(&all, "all", false, "include non-cli tool kinds (func, web, system)")
	return c
}

func newToolsShow(opts []Option) *cobra.Command {
	var asJSON, asYAML bool
	c := &cobra.Command{
		Use:           "show <name>",
		Short:         "Print a tool's definition",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(asJSON, asYAML); err != nil {
				return err
			}
			t, ok := New(opts...).Tool(args[0])
			if !ok {
				return fmt.Errorf("fleet: no tool %q", args[0])
			}
			return emit(cmd.OutOrStdout(), t, asJSON)
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit JSON instead of the canonical YAML")
	c.Flags().BoolVar(&asYAML, "yaml", false, "emit the canonical YAML asset blob (the default)")
	return c
}

// --- models -------------------------------------------------------------

// BandLabel renders a band for humans. An unpegged model shows as "-"
// rather than "L0", because 0 is not a band — it is a model nobody has
// placed yet, and it should look unanswered, not weak.
func BandLabel(band int) string {
	if band < 1 {
		return "-"
	}
	return "L" + strconv.Itoa(band)
}

type modelRow struct {
	Name     string   `json:"name"`
	Band     int      `json:"band,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Provider string   `json:"provider,omitempty"`
	Target   string   `json:"target,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
	Ring     string   `json:"ring"`
}

func newModelsList(opts []Option) *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List inference backends",
		Long: "List inference backends.\n\n" +
			"BAND is the model's capability peg, L1 (basic) to L4 (frontier),\n" +
			"normalized across providers — a vendor's own tier ladder is never\n" +
			"mapped positionally. Agents inherit the band of the model they bind.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			models, errs := New(opts...).Models()
			rows := make([]modelRow, 0, len(models))
			for _, m := range models {
				rows = append(rows, modelRow{
					Name: m.Name, Band: m.Band, Kind: m.Kind, Provider: m.Provider,
					Target: m.Target(), Aliases: m.Names()[1:], Ring: m.Ring.String(),
				})
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), rows)
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tBAND\tKIND\tPROVIDER\tTARGET\tALIASES\tRING")
			for _, r := range rows {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Name, BandLabel(r.Band),
					r.Kind, r.Provider, r.Target, strings.Join(r.Aliases, ","), r.Ring)
			}
			tw.Flush()
			return reportParseErrs(cmd.ErrOrStderr(), errs)
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return c
}

func newModelsShow(opts []Option) *cobra.Command {
	var asJSON, asYAML bool
	c := &cobra.Command{
		Use:           "show <name>",
		Short:         "Print a model's definition",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(asJSON, asYAML); err != nil {
				return err
			}
			m, ok := New(opts...).Model(args[0])
			if !ok {
				return fmt.Errorf("fleet: no model %q", args[0])
			}
			return emit(cmd.OutOrStdout(), m, asJSON)
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit JSON instead of the canonical YAML")
	c.Flags().BoolVar(&asYAML, "yaml", false, "emit the canonical YAML asset blob (the default)")
	return c
}

// --- agents -------------------------------------------------------------

type agentRow struct {
	Name        string   `json:"name"`
	Nick        string   `json:"nick,omitempty"`
	Band        int      `json:"band,omitempty"`
	Tool        string   `json:"tool"`
	Model       string   `json:"model"`
	Binding     string   `json:"binding"`
	Reliability string   `json:"reliability,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	Resolves    bool     `json:"resolves"`
	Reason      string   `json:"reason,omitempty"`
	Ring        string   `json:"ring"`
}

func newAgentsList(opts []Option) *cobra.Command {
	var asJSON, all bool
	var band, minBand int
	c := &cobra.Command{
		Use:   "list",
		Short: "List named tool:model bindings",
		Long: "List named tool:model bindings.\n\n" +
			"An agent resolves when both halves of its binding are in the catalog.\n" +
			"Dangling agents are hidden unless --all is given.\n\n" +
			"BAND is inherited from the model — L1 (basic) to L4 (frontier) — and is\n" +
			"how you select a roster without naming anyone: --min-band 3 is every\n" +
			"agent worth seating at a design discussion. NICK is the agent's human\n" +
			"name, assigned from its binding unless one was set with --nick.",
		Example: "  bashy agents list --min-band 3\n" +
			"  bashy agents list --json",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if band != 0 && minBand != 0 {
				return fmt.Errorf("fleet: --band and --min-band are alternatives; give one")
			}
			cat := New(opts...)
			agents, errs := cat.Agents()
			rows := make([]agentRow, 0, len(agents))
			for _, a := range agents {
				r := agentRow{
					Name: a.Name, Nick: a.NickName(), Tool: a.Tool, Model: a.Model,
					Binding: a.MatrixKey(), Aliases: a.Aliases, Resolves: true,
					Ring: a.Ring.String(),
				}
				if a.Ledger != nil {
					r.Reliability = a.Ledger.Reliability
				}
				if _, _, m, err := cat.Binding(a.Name); err != nil {
					r.Resolves, r.Reason = false, err.Error()
				} else {
					r.Band = m.Band
				}
				if !r.Resolves && !all {
					continue
				}
				// An unpegged or dangling agent is never silently swept into a
				// band filter: it has no band, so it matches no band.
				if band != 0 && r.Band != band {
					continue
				}
				if minBand != 0 && r.Band < minBand {
					continue
				}
				rows = append(rows, r)
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), rows)
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tNICK\tBAND\tTOOL\tMODEL\tRELIAB\tRESOLVES\tRING")
			for _, r := range rows {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					r.Name, dashIfEmpty(r.Nick), BandLabel(r.Band), r.Tool, r.Model,
					dashIfEmpty(r.Reliability), yesNo(r.Resolves), r.Ring)
			}
			tw.Flush()
			if err := reportCollisions(cmd.ErrOrStderr(), cat.CheckAliases()); err != nil {
				return err
			}
			return reportParseErrs(cmd.ErrOrStderr(), errs)
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	c.Flags().BoolVar(&all, "all", false, "include agents whose tool or model does not resolve")
	c.Flags().IntVar(&band, "band", 0, "only agents in exactly this band (1-4)")
	c.Flags().IntVar(&minBand, "min-band", 0, "only agents in this band or above (1-4)")
	return c
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func newAgentsShow(opts []Option) *cobra.Command {
	var asJSON, asYAML bool
	c := &cobra.Command{
		Use:           "show <name>",
		Short:         "Print an agent's binding",
		Long:          "Print an agent's binding. <name> may be a nickname, an alias, or a bare tool:model.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(asJSON, asYAML); err != nil {
				return err
			}
			cat := New(opts...)
			a, ok := cat.Agent(args[0])
			if !ok {
				return fmt.Errorf("fleet: no agent %q", args[0])
			}
			if asJSON {
				return emit(cmd.OutOrStdout(), a, true)
			}
			// An agent's asset blob is the envelope, not the bare agent —
			// that is the shape the store holds and the control plane serves.
			if asYAML {
				return emit(cmd.OutOrStdout(), AgentFile{Agents: []Agent{a}}, false)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s  (%s)\n", a.Name, a.MatrixKey())
			if len(a.Aliases) > 0 {
				fmt.Fprintf(out, "aliases: %s\n", strings.Join(a.Aliases, " "))
			}
			_, tool, model, err := cat.Binding(a.Name)
			if err != nil {
				fmt.Fprintf(out, "resolves: no (%v)\n", err)
				return nil
			}
			fmt.Fprintf(out, "tool:    %s (%s)\n", tool.Name, tool.CLI.Binary)
			fmt.Fprintf(out, "model:   %s → %s\n", model.Name, model.Target())
			fmt.Fprintf(out, "launch:  %s\n", strings.Join(tool.Argv(model.Target(), PromptToken), " "))
			if !tool.TakesModel() {
				fmt.Fprintf(out, "warning: %s cannot select a model; the binding is a label, not a selection\n", tool.Name)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit JSON instead of a summary")
	c.Flags().BoolVar(&asYAML, "yaml", false, "emit the canonical YAML asset blob")
	return c
}

// --- shared helpers ------------------------------------------------------

// checkFormat rejects asking for two output formats at once, rather than
// silently letting one win.
func checkFormat(asJSON, asYAML bool) error {
	if asJSON && asYAML {
		return fmt.Errorf("fleet: --json and --yaml are mutually exclusive")
	}
	return nil
}

func emit(w io.Writer, v any, asJSON bool) error {
	if asJSON {
		return writeJSON(w, v)
	}
	data, err := Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// reportParseErrs surfaces broken entries on stderr and fails the verb.
// A catalog that silently shortened its list would be worse than one that
// refuses: the caller would never learn an asset is malformed.
func reportParseErrs(w io.Writer, errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(errs))
	for _, e := range errs {
		msgs = append(msgs, e.Error())
	}
	sort.Strings(msgs)
	for _, m := range msgs {
		fmt.Fprintln(w, "warning:", m)
	}
	noun := "entries"
	if len(errs) == 1 {
		noun = "entry"
	}
	return fmt.Errorf("fleet: %d %s could not be read", len(errs), noun)
}

func reportCollisions(w io.Writer, cols []AliasCollision) error {
	if len(cols) == 0 {
		return nil
	}
	for _, c := range cols {
		fmt.Fprintln(w, "error:", c.Error())
	}
	noun := "collisions"
	if len(cols) == 1 {
		noun = "collision"
	}
	return fmt.Errorf("fleet: %d name %s — one name may not mean two things", len(cols), noun)
}

// Main runs a fleet verb tree as a standalone program. Hosts that mount
// the tree themselves (bashy) call the New*Cmd constructors directly.
func Main(cmd *cobra.Command, args []string) {
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitCode(err))
	}
}
