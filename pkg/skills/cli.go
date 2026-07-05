package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// config is assembled from Options by NewSkillsCmd.
type config struct {
	sources  []Source
	statics  map[string]string
	cfgDir   string
	cacheTTL time.Duration
}

type Option func(*config)

// WithSource appends a skill source. Order matters: later sources
// shadow earlier ones (mount embedded first, local last).
func WithSource(s Source) Option { return func(c *config) { c.sources = append(c.sources, s) } }

// WithHostVersion injects a fact only the host binary knows (its own
// version) as a static probe, e.g. ("bashy", "0.9.1").
func WithHostVersion(name, version string) Option {
	return func(c *config) {
		if version != "" {
			c.statics[name] = version
		}
	}
}

// WithConfigDir overrides the ring-1 directory (default
// ~/.config/bashy/skills): the local skill store + the probe cache.
func WithConfigDir(dir string) Option { return func(c *config) { c.cfgDir = dir } }

func defaultConfigDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".config", "bashy", "skills")
	}
	return ""
}

// NewSkillsCmd builds the `skills` command tree: probe / list / show.
// Bare `skills` behaves like `skills list` (back-compat with the
// pre-cobra dispatcher).
func NewSkillsCmd(opts ...Option) *cobra.Command {
	cfg := &config{statics: map[string]string{}, cacheTTL: 24 * time.Hour}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.cfgDir == "" {
		cfg.cfgDir = defaultConfigDir()
	}

	root := &cobra.Command{
		Use:           "skills",
		Short:         "workspace skills, gated by this host's space-time coordinate",
		Long:          "skills lists, inspects, and probes the tier-2 workspace skills available\non this host. `list` shows only skills applicable here (env-gated via each\nskill's metadata.requires); `probe` prints the host coordinate the gate\nevaluates against.",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          func(cmd *cobra.Command, args []string) error { return runList(cmd, cfg, false, false) },
	}

	var all, asJSON bool
	list := &cobra.Command{
		Use:   "list",
		Short: "list skills applicable at this coordinate (--all: everything, annotated)",
		RunE:  func(cmd *cobra.Command, args []string) error { return runList(cmd, cfg, all, asJSON) },
	}
	list.Flags().BoolVar(&all, "all", false, "include inapplicable skills, with the failing clause")
	list.Flags().BoolVar(&asJSON, "json", false, "machine-readable listing")

	var refresh, probeJSON bool
	probe := &cobra.Command{
		Use:   "probe",
		Short: "print this host's space-time coordinate (probes + context key)",
		RunE:  func(cmd *cobra.Command, args []string) error { return runProbe(cmd, cfg, refresh, probeJSON) },
	}
	probe.Flags().BoolVar(&refresh, "refresh", false, "re-measure lazy probes (drop the cache)")
	probe.Flags().BoolVar(&probeJSON, "json", false, "machine-readable output")

	var ref bool
	show := &cobra.Command{
		Use:   "show <name>",
		Short: "print a skill's SKILL.md (--reference: its reference.md)",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return runShow(cmd, cfg, args[0], ref) },
	}
	show.Flags().BoolVarP(&ref, "reference", "r", false, "print the deep-companion reference.md")

	var force, addJSON bool
	add := &cobra.Command{
		Use:   "add <dir>",
		Short: "install a skill folder into the host-local store (verified admission)",
		Long:  "add installs a skill folder (SKILL.md + optional reference.md/skill.dhnt)\ninto the host-local store after a verified-admission gate: frontmatter must\nparse with name+description, metadata.requires must parse, and a skill.dhnt\ncanonical face must be valid (transpilable, content-addressed). Inapplicable-\nhere is reported, not refused — a skill may be installed for a tool you have\nnot provisioned yet.",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return runAdd(cmd, cfg, args[0], force, addJSON) },
	}
	add.Flags().BoolVar(&force, "force", false, "replace an already-installed skill of the same name")
	add.Flags().BoolVar(&addJSON, "json", false, "machine-readable admission report")

	var verifyJSON bool
	verify := &cobra.Command{
		Use:   "verify <name>",
		Short: "dry gate: structural validity + applicability at this coordinate (exit 0 iff both)",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return runVerify(cmd, cfg, args[0], verifyJSON) },
	}
	verify.Flags().BoolVar(&verifyJSON, "json", false, "machine-readable report")

	var runJSON, adapt bool
	var repairAgent, target string
	var attempts int
	run := &cobra.Command{
		Use:   "run <name>",
		Short: "execute a dhnt skill and attest it (exit 0 iff the contract held within the cap)",
		Long:  "run executes a skill's canonical face through the in-process userland:\ncontract predicates and step primitives resolve their concrete commands from\nSKILL.md metadata (check-*/step-* keys), a static pre-flight audit refuses to\nstart when the declared effect cap cannot cover what the bindings report, and\nevery run emits a re-checkable attestation stored in the host-local store.\nA host that has learned a fixed version of the skill runs it transparently.\nWith --adapt, a failing run asks the repair agent for corrected steps,\nverifies them under the ORIGINAL contract and effect cap, folds the fix into\na guarded environment arm, and saves it to the host overlay — the next run\n(by any agent on this host) reuses it. Command output streams to stderr;\nthe receipt is stdout.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if target != "" && adapt {
				return fmt.Errorf("skills: --target and --adapt do not combine (adapt repairs canonical steps, not dag targets)")
			}
			if target != "" {
				return runTarget(cmd, cfg, args[0], target, runJSON)
			}
			return runRun(cmd, cfg, args[0], runJSON, adapt, repairAgent, attempts)
		},
	}
	run.Flags().BoolVar(&runJSON, "json", false, "machine-readable receipt")
	run.Flags().BoolVar(&adapt, "adapt", false, "self-heal: repair, verify, and fold a fix on failure")
	run.Flags().StringVar(&repairAgent, "repair-agent", "", "headless agent CLI for repair proposals; the prompt is appended as the last argument (e.g. \"claude -p\")")
	run.Flags().IntVar(&attempts, "attempts", 2, "max repair attempts under --adapt")
	run.Flags().StringVar(&target, "target", "", "execute this dag target from the skill's tasks.md (contracted skills stay attested)")

	var learnJSON bool
	learn := &cobra.Command{
		Use:   "learn <dir>",
		Short: "execution-verified admission: add a skill AND prove its contract holds here",
		Long:  "learn is the writeback gate for skills distilled from experience: the same\nstructural admission as `add`, PLUS the skill must actually run and satisfy\nits contract at this coordinate before it stays in the store. A skill that\nfails the run gate is removed again — the library only accretes procedures\nthat have worked here.",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return runLearn(cmd, cfg, args[0], learnJSON) },
	}
	learn.Flags().BoolVar(&learnJSON, "json", false, "machine-readable receipt")

	var promoteOut string
	promote := &cobra.Command{
		Use:   "promote <name>",
		Short: "render the human-review bundle for pushing a learned skill upstream (never commits)",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return runPromote(cmd, cfg, args[0], promoteOut) },
	}
	promote.Flags().StringVar(&promoteOut, "out", "", "bundle output directory (default ./promote-<name>)")

	var expTo string
	var expUser, expRepo, expForce bool
	export := &cobra.Command{
		Use:   "export <name>",
		Short: "install a catalog skill into agent skill directories (user scope, a dir, or --repo)",
		Long:  "export writes a skill folder where agentic tools read skills:\n  --user  ~/.agents/skills (the vendor-neutral standard) plus each DETECTED\n          vendor root (~/.claude/skills, ~/.copilot/skills)\n  --to    any directory (a workspace, a team catalog checkout)\n  --repo  .agents/skills at the repo root (+ .claude/skills if .claude exists);\n          repo writes are explicit-only — your repository, your call\nEvery export carries an ownership marker; re-exports refresh only folders we\nwrote (--force overrides). Content is the standard portable skill folder.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd, cfg, args[0], expTo, expUser, expRepo, expForce)
		},
	}
	export.Flags().StringVar(&expTo, "to", "", "export into this directory")
	export.Flags().BoolVar(&expUser, "user", false, "install at user scope (detected agent roots)")
	export.Flags().BoolVar(&expRepo, "repo", false, "install at repo scope (explicit consent)")
	export.Flags().BoolVar(&expForce, "force", false, "replace folders not exported by us")

	root.AddCommand(list, probe, show, add, verify, run, learn, promote, export)
	return root
}

func runExport(cmd *cobra.Command, cfg *config, name, to string, user, repo, force bool) error {
	sk, src, ok := cfg.catalog().Get(name)
	if !ok {
		return fmt.Errorf("skills: %q not found", name)
	}
	var roots []string
	if to != "" {
		roots = append(roots, to)
	}
	if user {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		roots = append(roots, userExportRoots(home)...)
	}
	if repo {
		roots = append(roots, repoExportRoots(findRepoRoot(mustGetwd()))...)
	}
	if len(roots) == 0 {
		return fmt.Errorf("skills: pick a target: --user, --repo, or --to DIR")
	}
	var firstErr error
	for _, root := range roots {
		dst, err := ExportTo(sk, src, root, force)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "skills: %v\n", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "exported: %s\n", dst)
	}
	return firstErr
}

func (c *config) probes(refresh bool) (*ProbeSet, *FileCache) {
	var fc *FileCache
	var cache Cache = NopCache()
	if c.cfgDir != "" {
		fc = NewFileCache(c.cfgDir, c.cacheTTL)
		if !refresh {
			cache = fc
		}
	}
	ps := DefaultProbes(cache)
	for name, v := range c.statics {
		ps.SetStatic(name, v)
	}
	return ps, fc
}

func (c *config) catalog() *Catalog {
	cat := &Catalog{Sources: c.sources}
	if c.cfgDir != "" {
		cat.Sources = append(cat.Sources, DirSource(c.cfgDir))
	}
	return cat
}

func runList(cmd *cobra.Command, cfg *config, all, asJSON bool) error {
	ps, _ := cfg.probes(false)
	rows, err := cfg.catalog().List(ps)
	if err != nil {
		return err
	}
	if asJSON {
		type row struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
			Ring        string `json:"ring"`
			Applicable  bool   `json:"applicable"`
			Failing     string `json:"failing,omitempty"`
			Unchecked   string `json:"unchecked_compat,omitempty"`
			Shadows     bool   `json:"shadows,omitempty"`
			HasDhnt     bool   `json:"dhnt,omitempty"`
			Identity    string `json:"identity,omitempty"`
			Warning     string `json:"warning,omitempty"`
		}
		out := make([]row, 0, len(rows))
		for _, r := range rows {
			if !all && !r.Verdict.Applicable {
				continue
			}
			var id string
			if r.Dhnt.Valid() {
				id = r.Dhnt.Identity
			}
			out = append(out, row{r.Name, r.Description, r.Ring.String(), r.Verdict.Applicable,
				r.Verdict.Failing, r.Verdict.Unchecked, r.Shadows, r.HasDhnt, id, r.Warning})
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
	}
	for _, r := range rows {
		switch {
		case r.Verdict.Applicable:
			fmt.Fprintln(cmd.OutOrStdout(), r.Name)
		case all:
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t# inapplicable: %s\n", r.Name, r.Verdict.Failing)
		}
	}
	return nil
}

func runProbe(cmd *cobra.Command, cfg *config, refresh, asJSON bool) error {
	ps, fc := cfg.probes(refresh)
	vals := ps.Core()
	if fc != nil && !refresh {
		for k, v := range fc.Entries(ps.PathHash()) {
			if v != "" {
				vals[k] = v
			}
		}
	}
	key := ContextKey(vals)
	if asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
			Probes     map[string]string `json:"probes"`
			ContextKey string            `json:"context_key"`
		}{vals, key})
	}
	names := make([]string, 0, len(vals))
	for k := range vals {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", k, vals[k])
	}
	fmt.Fprintf(cmd.OutOrStdout(), "context=%s\n", key)
	return nil
}

func runShow(cmd *cobra.Command, cfg *config, name string, ref bool) error {
	cat := cfg.catalog()
	sk, src, ok := cat.Get(name)
	if !ok {
		return fmt.Errorf("skills: %q not found", name)
	}
	rel := "SKILL.md"
	if ref {
		rel = "reference.md"
	}
	body, ok := src.File(name, rel)
	if !ok {
		return fmt.Errorf("skills: %q has no %s", name, rel)
	}
	// stdout stays byte-identical to the skill content; the verdict is a
	// one-line stderr annotation (the hint-engine idiom), so existing
	// consumers parsing stdout are unaffected.
	fmt.Fprint(cmd.OutOrStdout(), string(body))
	if !ref {
		ps, _ := cfg.probes(false)
		v := verdictOf(sk, ps)
		status := "applicable"
		if !v.Applicable {
			status = "inapplicable: " + v.Failing
		}
		dhnt := "absent"
		if sk.Dhnt.Valid() {
			dhnt = sk.Dhnt.Identity[:13] // content-address prefix
		} else if sk.HasDhnt {
			dhnt = "invalid"
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "skills: %s — ring=%s dhnt=%s %s\n", sk.Name, sk.Ring, dhnt, status)
	}
	return nil
}

func runAdd(cmd *cobra.Command, cfg *config, dir string, force, asJSON bool) error {
	if strings.Contains(dir, "://") {
		return fmt.Errorf("skills: url sources are not supported yet — pass a local skill directory")
	}
	if cfg.cfgDir == "" {
		return fmt.Errorf("skills: no host-local store directory configured")
	}
	sk, err := loadSkillDir(dir)
	if err != nil {
		return err
	}
	ps, _ := cfg.probes(false)
	a := admit(sk, ps)
	if asJSON {
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(a); err != nil {
			return err
		}
	} else {
		renderAdmission(cmd.OutOrStdout(), a)
	}
	if !a.Valid {
		return fmt.Errorf("skills: %q failed the admission gate", sk.Name)
	}
	dst, err := installSkill(dir, cfg.cfgDir, sk.Name, force)
	if err != nil {
		return err
	}
	if !asJSON {
		fmt.Fprintf(cmd.OutOrStdout(), "installed: %s\n", dst)
	}
	if !a.Applicable {
		fmt.Fprintf(cmd.ErrOrStderr(), "skills: %s installed but not applicable here (%s)\n", sk.Name, a.Failing)
	}
	return nil
}

func runVerify(cmd *cobra.Command, cfg *config, name string, asJSON bool) error {
	sk, _, ok := cfg.catalog().Get(name)
	if !ok {
		return fmt.Errorf("skills: %q not found", name)
	}
	ps, _ := cfg.probes(false)
	a := admit(sk, ps)
	if asJSON {
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(a); err != nil {
			return err
		}
	} else {
		renderAdmission(cmd.OutOrStdout(), a)
	}
	if !a.Valid || !a.Applicable {
		return fmt.Errorf("skills: %q did not verify (valid=%v applicable=%v)", name, a.Valid, a.Applicable)
	}
	return nil
}

func runRun(cmd *cobra.Command, cfg *config, name string, asJSON, adapt bool, repairAgent string, attempts int) error {
	sk, src, ok := cfg.catalog().Get(name)
	if !ok {
		return fmt.Errorf("skills: %q not found", name)
	}
	ps, _ := cfg.probes(false)

	var rec AttestRecord
	var outcome AdaptOutcome
	var err error
	if adapt {
		var complete Completer
		if repairAgent != "" {
			complete = execCompleter(repairAgent)
		}
		rec, outcome, err = adaptiveRun(cfg, sk, src, ps, mustGetwd(), cmd.ErrOrStderr(), complete, attempts)
	} else {
		rec, _, err = runSkill(cfg, sk, src, ps, mustGetwd(), cmd.ErrOrStderr())
	}
	if err != nil && rec.Name == "" {
		return err
	}
	renderReceipt(cmd, rec, string(outcome), asJSON)
	if err != nil {
		return err
	}
	if !rec.Attest.Valid {
		return fmt.Errorf("skills: %q contract not satisfied", name)
	}
	return nil
}

func runTarget(cmd *cobra.Command, cfg *config, name, target string, asJSON bool) error {
	sk, src, ok := cfg.catalog().Get(name)
	if !ok {
		return fmt.Errorf("skills: %q not found", name)
	}
	ps, _ := cfg.probes(false)
	rec, attested, err := runTargetSkill(cfg, sk, src, ps, target, mustGetwd(), cmd.ErrOrStderr())
	if !attested {
		// Executable-but-uncontracted rung: plain result, no receipt.
		if err != nil {
			return err
		}
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
				"skill": name, "target": target, "ok": true,
			})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "skill: %s\ntarget: %s\nok: true\n", name, target)
		return nil
	}
	if err != nil {
		return err
	}
	renderReceipt(cmd, rec, "target:"+target, asJSON)
	if !rec.Attest.Valid {
		return fmt.Errorf("skills: %q contract not satisfied", name)
	}
	return nil
}

func renderReceipt(cmd *cobra.Command, rec AttestRecord, outcome string, asJSON bool) {
	if asJSON {
		out := struct {
			AttestRecord
			Outcome string `json:"outcome,omitempty"`
		}{rec, outcome}
		_ = json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		return
	}
	a := rec.Attest
	fmt.Fprintf(cmd.OutOrStdout(), "skill: %s\ntier: %s\nvalid: %v\n", rec.Name, rec.Tier, a.Valid)
	if outcome != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "outcome: %s\n", outcome)
	}
	if len(a.Passed) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "passed: %s\n", strings.Join(a.Passed, " "))
	}
	if len(a.Failed) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "failed: %s\n", strings.Join(a.Failed, " "))
	}
	var effs []string
	for _, e := range a.Effects {
		effs = append(effs, e.String())
	}
	fmt.Fprintf(cmd.OutOrStdout(), "effects: %s\ncontext: %s\n", strings.Join(effs, " "), rec.ContextKey)
}

func runLearn(cmd *cobra.Command, cfg *config, dir string, asJSON bool) error {
	if cfg.cfgDir == "" {
		return fmt.Errorf("skills: no host-local store directory configured")
	}
	sk, err := loadSkillDir(dir)
	if err != nil {
		return err
	}
	ps, _ := cfg.probes(false)
	a := admit(sk, ps)
	if !a.Valid {
		if !asJSON {
			renderAdmission(cmd.OutOrStdout(), a)
		}
		return fmt.Errorf("skills: %q failed the admission gate", sk.Name)
	}
	if !a.Applicable {
		return fmt.Errorf("skills: %q is not applicable here (%s) — learn requires proving the contract at this coordinate; use `add` to install without the run gate", sk.Name, a.Failing)
	}
	if _, err := installSkill(dir, cfg.cfgDir, sk.Name, false); err != nil {
		return err
	}
	// The run gate: the contract must actually hold here, or the skill
	// comes back out of the store.
	installed, src, ok := cfg.catalog().Get(sk.Name)
	if !ok {
		return fmt.Errorf("skills: %q vanished after install", sk.Name)
	}
	rec, _, err := runSkill(cfg, installed, src, ps, mustGetwd(), cmd.ErrOrStderr())
	if err != nil || !rec.Attest.Valid {
		_ = os.RemoveAll(filepath.Join(cfg.cfgDir, sk.Name))
		if err != nil {
			return fmt.Errorf("skills: %q not learned: %w", sk.Name, err)
		}
		renderReceipt(cmd, rec, "rejected", asJSON)
		return fmt.Errorf("skills: %q not learned — the contract did not hold here (the failed run is attested)", sk.Name)
	}
	renderReceipt(cmd, rec, "learned", asJSON)
	return nil
}

func runPromote(cmd *cobra.Command, cfg *config, name, out string) error {
	sk, src, ok := cfg.catalog().Get(name)
	if !ok {
		return fmt.Errorf("skills: %q not found", name)
	}
	if out == "" {
		out = "promote-" + name
	}
	dir, err := promoteBundle(cfg, sk, src, out)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "bundle: %s\n", dir)
	fmt.Fprintln(cmd.OutOrStdout(), "review PROMOTION.md, then merge through the catalog's normal change process")
	return nil
}

func mustGetwd() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// Advertised is the L1 surface of one applicable skill — what a host
// injects into an agent's first-hop context (progressive disclosure:
// names + one-liners here; full bodies via `skills show`).
type Advertised struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Ring        string `json:"ring,omitempty"`
	Verified    bool   `json:"verified,omitempty"` // carries a valid canonical face (identity + contract)
}

// Applicable returns the skills applicable at this host's coordinate,
// with descriptions truncated to an L1-sized budget. Errors degrade to
// an empty list — first-hop context must never fail on skills.
func Applicable(opts ...Option) []Advertised {
	cfg := &config{statics: map[string]string{}, cacheTTL: 24 * time.Hour}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.cfgDir == "" {
		cfg.cfgDir = defaultConfigDir()
	}
	ps, _ := cfg.probes(false)
	rows, err := cfg.catalog().List(ps)
	if err != nil {
		return nil
	}
	var out []Advertised
	for _, r := range rows {
		if !r.Verdict.Applicable {
			continue
		}
		out = append(out, Advertised{
			Name:        r.Name,
			Description: truncate(r.Description, 160),
			Ring:        r.Ring.String(),
			Verified:    r.Dhnt.Valid(),
		})
	}
	return out
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return strings.TrimSpace(string(runes[:n-1])) + "…"
}

// ExitCode maps a NewSkillsCmd Execute error to the repo exit
// convention: 2 for usage errors, 1 otherwise, 0 for nil.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	msg := err.Error()
	for _, p := range []string{"unknown command", "unknown flag", "unknown shorthand", "accepts ", "requires ", "invalid argument"} {
		if strings.Contains(msg, p) {
			return 2
		}
	}
	return 1
}
