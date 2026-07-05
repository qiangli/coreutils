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

	root.AddCommand(list, probe, show, add, verify)
	return root
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
