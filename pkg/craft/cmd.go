package craft

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/skills"
)

// config is assembled from Options by NewCraftCmd.
type config struct {
	// storeDir is the ring-1 skills store craft reads evidence from. It is
	// deliberately the SAME directory pkg/skills writes to: craft is a layer
	// over the catalog, not a parallel store. A second store would be an
	// eighth disjoint outcome log in a tree that already has seven.
	storeDir string
}

// Option configures the craft command tree.
type Option func(*config)

// WithStoreDir overrides the skills store craft reads
// (default ~/.config/bashy/skills).
func WithStoreDir(dir string) Option { return func(c *config) { c.storeDir = dir } }

func defaultStoreDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".config", "bashy", "skills")
	}
	return ""
}

// NewCraftCmd builds the `craft` command tree — the living skill graph over
// the catalog pkg/skills manages.
//
// The split is deliberate. `bashy skills` is the CATALOG: an Agent
// Skills-compatible store you list, show, add, verify, run, and export.
// `bashy craft` is what the catalog ACCUMULATES INTO: evidence gathered across
// runs, coordinates, and implementations. One is the shelf; the other is what
// the practitioner has learned from working it.
func NewCraftCmd(opts ...Option) *cobra.Command {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.storeDir == "" {
		cfg.storeDir = defaultStoreDir()
	}

	root := &cobra.Command{
		Use:   "craft",
		Short: "the living skill graph: what this host has learned from running skills",
		Long: "craft is the accumulated body of practical skill on this host.\n\n" +
			"Where `bashy skills` manages the catalog — the skills you have — craft is\n" +
			"what running them has taught: which contract held, at which space-time\n" +
			"coordinate, under which executor, and how often.\n\n" +
			"Evidence is keyed two ways. By NAME, which identifies one skill. And by\n" +
			"CAPABILITY — a content address over a skill's contract and effect cap — so\n" +
			"two differently-named skills that make the same guarantee pool their\n" +
			"evidence instead of competing as strangers.",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          func(cmd *cobra.Command, args []string) error { return runHistory(cmd, cfg, "", "", false, false) },
	}

	var histJSON, histAll bool
	var histCapability string
	history := &cobra.Command{
		Use:   "history [name]",
		Short: "stored run receipts, per skill or pooled per capability",
		Long: "history reads back the attestation ledger every `skills run` writes.\n" +
			"Each row is one recorded run at one space-time coordinate: whether the\n" +
			"contract held, which executor tier ran it, and when.\n\n" +
			"With no argument it summarises every skill with evidence. With a name it\n" +
			"summarises that skill. With --capability it pools evidence across every\n" +
			"implementation that makes the same guarantee, which is the question worth\n" +
			"asking when two skills are interchangeable.\n\n" +
			"This reports; it decides nothing. Retirement and election need an evidence\n" +
			"floor a single host does not reach alone, and acting on a handful of runs\n" +
			"would discard a good skill on noise.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var name string
			if len(args) == 1 {
				name = args[0]
			}
			return runHistory(cmd, cfg, name, histCapability, histAll, histJSON)
		},
	}
	history.Flags().BoolVar(&histJSON, "json", false, "machine-readable summary")
	history.Flags().BoolVar(&histAll, "all", false, "list individual runs, not just the summary")
	history.Flags().StringVar(&histCapability, "capability", "", "pool evidence across implementations of this capability key")

	var studyLicense, studyOrigin, studyRef string
	var studyAgent, studyLang string
	var studyBand int
	var studyJSON bool
	study := &cobra.Command{
		Use:   "study <dir>",
		Short: "absorb a directory of external skills — digest, do not memorize",
		Long: "study reads a directory of skill folders and resolves each against what is\n" +
			"already known, BY CAPABILITY rather than by name:\n\n" +
			"  novel        a guarantee nothing here makes yet   -> the catalog grows\n" +
			"  alternative  same guarantee, another implementation\n" +
			"  duplicate    same guarantee, same implementation\n" +
			"  quarantined  no contract, so no capability is knowable — held, not merged\n" +
			"  refused      license, or a face that will not parse — nothing is stored\n\n" +
			"Only `novel` grows the catalog, and the ratio of growth to input is the\n" +
			"DIGESTION RATIO. At 1:1 nothing was digested: each external skill became\n" +
			"its own entry, which is how a catalog turns into a pile that makes an\n" +
			"agent measurably worse rather than better.\n\n" +
			"The license gate is fail-closed. Absence of a license is not permission —\n" +
			"it is all-rights-reserved by default — and licence is read PER SKILL, never\n" +
			"inherited from a repository.\n\n" +
			"Prose with no contract is QUARANTINED, not guessed at: inventing a contract\n" +
			"would file a skill under a promise nobody verified.\n\n" +
			"To decompose prose into a typed contract, pass --normalise-agent with the\n" +
			"model's --band. This is the one step that genuinely needs a model, and it is\n" +
			"the work worth paying for once: the contract it produces is checked for free\n" +
			"by every later run. It requires band >= 3 and REFUSES below that — an\n" +
			"under-banded decomposition is worse than none, because a plausible-but-wrong\n" +
			"contract is a promise nobody verified that every later reader inherits.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStudy(cmd, cfg, args[0], Source{
				Origin:  studyOrigin,
				Ref:     studyRef,
				License: studyLicense,
			}, studyJSON, studyAgent, studyBand, studyLang)
		},
	}
	study.Flags().StringVar(&studyAgent, "normalise-agent", "", "headless agent CLI that decomposes prose into a typed contract; the prompt is appended as the last argument (e.g. \"claude -p\")")
	study.Flags().IntVar(&studyBand, "band", 0, "capability band of the normalise-agent's model (must be >= 3; an unverified band is not a passing band)")
	study.Flags().StringVar(&studyLang, "lang", "en", "source natural language of the prose (the canonical form is language-neutral)")
	study.Flags().StringVar(&studyLicense, "license", "", "SPDX id these skills are under (required; absence is not permission)")
	study.Flags().StringVar(&studyOrigin, "origin", "", "upstream repo or URL, recorded as provenance")
	study.Flags().StringVar(&studyRef, "ref", "", "upstream commit sha, recorded as provenance")
	study.Flags().BoolVar(&studyJSON, "json", false, "machine-readable report")

	root.AddCommand(history, study)
	return root
}

func runStudy(cmd *cobra.Command, cfg *config, dir string, src Source, asJSON bool, agent string, band int, lang string) error {
	// Build the normaliser BEFORE reading anything. An under-banded or
	// misconfigured agent must fail here, loudly and with nothing absorbed —
	// not halfway through a run with part of a catalog already quarantined
	// under a reason that blames the content rather than the configuration.
	var norm Normaliser
	if strings.TrimSpace(agent) != "" {
		n, err := NewNormaliser(ExecCompleter(agent), NormaliserOptions{Band: band, Lang: lang})
		if err != nil {
			return err
		}
		norm = n
	}

	cands, err := LoadDir(dir, src)
	if err != nil {
		return fmt.Errorf("craft: reading %s: %w", dir, err)
	}
	if len(cands) == 0 {
		return fmt.Errorf("craft: no skill folders found under %s (a candidate is a directory holding a SKILL.md)", dir)
	}

	// Absorption resolves against the capabilities already held. Reading the
	// ledger is a stand-in until the catalog itself is capability-indexed:
	// evidence is where capabilities are currently observable.
	known := map[string][]string{}
	if l, err := ReadLedger(cfg.storeDir); err == nil {
		for _, o := range l.Observations {
			if o.Capability != "" {
				known[o.Capability] = appendDistinct(known[o.Capability], o.Identity)
			}
		}
	}

	rep := Study(cands, known, norm)

	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}

	out := cmd.OutOrStdout()
	for _, o := range rep.Outcomes {
		line := fmt.Sprintf("%-12s %-28s", o.Disposition, craftTruncate(o.Name, 28))
		if o.Capability != "" {
			line += " " + skills.ShortID(o.Capability)
		}
		fmt.Fprintln(out, line)
		if o.Reason != "" {
			fmt.Fprintf(out, "%-12s   %s\n", "", o.Reason)
		}
	}
	fmt.Fprintf(out, "\n%d absorbed, %d capabilities added — digestion ratio %.2f\n",
		rep.Absorbed, rep.Grew, rep.DigestionRatio())
	if rep.DigestionRatio() >= 1.0 && rep.Absorbed > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"craft: ratio is 1.00 — every skill became its own capability, so nothing was digested\n")
	}
	return nil
}

// ExitCode maps an Execute error to the repo exit convention.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	return 1
}

// craftTruncate shortens a display cell to n runes, rune-aware so a multi-byte
// name is not cut mid-character.
func craftTruncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return string(runes[:n])
	}
	return string(runes[:n-1]) + "…"
}

// historyRow is one summarised subject in the ledger — a skill, or a capability.
type historyRow struct {
	Subject     string   `json:"subject"`
	Kind        string   `json:"kind"` // "skill" | "capability"
	Runs        int      `json:"runs"`
	Passed      int      `json:"passed"`
	Failed      int      `json:"failed"`
	Rate        float64  `json:"contribution"`
	Coordinates []string `json:"coordinates,omitempty"`
	Tiers       []string `json:"tiers,omitempty"`
	First       string   `json:"first,omitempty"`
	Last        string   `json:"last,omitempty"`
}

type historyReport struct {
	Schema string        `json:"schema"`
	Rows   []historyRow  `json:"rows"`
	Runs   []Observation `json:"observations,omitempty"`
	// Malformed is surfaced, never swallowed: evidence that failed to decode
	// is missing evidence, and a summary that hid it would present an absence
	// as a clean result.
	Malformed int      `json:"malformed,omitempty"`
	Files     []string `json:"files,omitempty"`
}

const historySchema = "bashy-craft-history-v1"

func runHistory(cmd *cobra.Command, cfg *config, name, capability string, all, asJSON bool) error {
	l, err := ReadLedger(cfg.storeDir)
	if err != nil {
		return fmt.Errorf("craft: reading the attestation ledger: %w", err)
	}

	rep := historyReport{Schema: historySchema, Malformed: l.Malformed, Files: l.Files}
	switch {
	case capability != "":
		rep.Rows = []historyRow{rowOf(capability, "capability", l.ForCapability(capability))}
		if all {
			rep.Runs = filterObs(l.Observations, func(o Observation) bool { return o.Capability == capability })
		}
	case name != "":
		rep.Rows = []historyRow{rowOf(name, "skill", l.ForSkill(name))}
		if all {
			rep.Runs = filterObs(l.Observations, func(o Observation) bool { return o.Name == name })
		}
	default:
		for _, n := range l.Names() {
			rep.Rows = append(rep.Rows, rowOf(n, "skill", l.ForSkill(n)))
		}
		if all {
			rep.Runs = l.Observations
		}
	}

	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	writeHistoryText(cmd, rep, name, capability)
	return nil
}

func rowOf(subject, kind string, s Stats) historyRow {
	r := historyRow{
		Subject:     subject,
		Kind:        kind,
		Runs:        s.Runs,
		Passed:      s.Passed,
		Failed:      s.Failed,
		Rate:        s.Contribution(),
		Coordinates: s.Coordinates,
		Tiers:       s.Tiers,
	}
	if !s.First.IsZero() {
		r.First = s.First.UTC().Format(time.RFC3339)
	}
	if !s.Last.IsZero() {
		r.Last = s.Last.UTC().Format(time.RFC3339)
	}
	return r
}

func filterObs(in []Observation, keep func(Observation) bool) []Observation {
	var out []Observation
	for _, o := range in {
		if keep(o) {
			out = append(out, o)
		}
	}
	return out
}

func writeHistoryText(cmd *cobra.Command, rep historyReport, name, capability string) {
	out := cmd.OutOrStdout()

	total := 0
	for _, r := range rep.Rows {
		total += r.Runs
	}
	if total == 0 {
		// An empty ledger is a legitimate state, not a failure — and saying
		// so plainly beats printing an empty table that reads like a bug.
		subject := "any skill"
		if capability != "" {
			subject = "capability " + skills.ShortID(capability)
		} else if name != "" {
			subject = name
		}
		fmt.Fprintf(out, "no recorded runs for %s\n", subject)
		fmt.Fprintf(out, "evidence accrues from `skills run`; only contracted skills attest\n")
		return
	}

	fmt.Fprintf(out, "%-28s %5s %5s %5s %7s  %s\n", "SKILL", "RUNS", "PASS", "FAIL", "RATE", "COORDINATES")
	for _, r := range rep.Rows {
		subject := r.Subject
		if r.Kind == "capability" {
			subject = skills.ShortID(subject)
		}
		fmt.Fprintf(out, "%-28s %5d %5d %5d %+7.2f  %d\n",
			craftTruncate(subject, 28), r.Runs, r.Passed, r.Failed, r.Rate, len(r.Coordinates))
	}

	if len(rep.Runs) > 0 {
		fmt.Fprintln(out)
		for _, o := range rep.Runs {
			state := "FAIL"
			if o.Valid {
				state = "pass"
			}
			fmt.Fprintf(out, "%s  %-4s  %-24s %s  %s\n",
				o.At.UTC().Format(time.RFC3339), state, craftTruncate(o.Name, 24), skills.ShortID(o.ContextKey), o.Tier)
		}
	}

	if rep.Malformed > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"skills: %d ledger line(s) could not be decoded and are NOT counted above\n", rep.Malformed)
	}
}
