// Package kb is the host-scope shared knowledge base for agents — the
// collective memory of every agent tool working on this machine, across all
// repositories. It fills the ring between the per-repo stores (weave
// campaign memory, the graph contribution log) and the org/cloud tier: one
// wiki of small OKF-style markdown pages (frontmatter + distilled body)
// under ~/.bashy/kb, shared by claude/codex/opencode/… alike.
//
// The cooperative loop it exists for: an agent SEARCHES the kb before
// undertaking a task; if nothing relevant exists it CONTRIBUTES a candidate
// entry; after the task it runs a RETRO — validate or correct what it
// consulted (add / update / supersede / validate / noop — never
// blind-append).
//
// It is part of the AgentOS hub (consumed by bashy as `bashy kb`),
// standalone-first: no cloudbox, no network, no LLM — deterministic
// substring/tag retrieval with a small default K (precision over recall).
// The store is plain files, so agents without the CLI can grep it directly;
// index.md is the entry point.
package kb

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/scope"
)

// resolveKBDir picks the kb store directory using the shared scope resolver and
// reports a one-word scope label ("repo" | "user" | "dir"). Precedence: an
// explicit --dir wins; then $BASHY_KB_DIR forces the host store (tests, relocated
// homes) unless --repo/--base-dir asked otherwise; otherwise auto-detect (this
// repo's docs/kb/ inside a git repo, else the host store).
func resolveKBDir(dir *string, forceRepo, forceUser bool, baseDir string) (string, error) {
	if strings.TrimSpace(*dir) != "" {
		return "dir", nil
	}
	if env := strings.TrimSpace(os.Getenv("BASHY_KB_DIR")); env != "" && baseDir == "" && !forceRepo {
		*dir = env
		return "user", nil
	}
	sc, err := scope.Resolve(scope.Options{
		RepoSub:   RepoSub,
		HostDir:   func() (string, error) { return DefaultDir(), nil },
		ForceRepo: forceRepo,
		ForceUser: forceUser,
		BaseDir:   baseDir,
	})
	if err != nil {
		return "", err
	}
	*dir = sc.Dir()
	return string(sc.Kind), nil
}

// NewKBCmd returns the `kb` cobra command tree — the host-agnostic entry
// point a front end mounts (e.g. `bashy kb`).
func NewKBCmd() *cobra.Command {
	var dir, baseDir string
	var forceRepo, forceUser bool
	cmd := &cobra.Command{
		Use:   "kb",
		Short: "Knowledge base for agents — auto: repo docs/kb/ if in a git repo, else your host store",
		Long: `kb is agent memory as a wiki of small markdown pages (YAML frontmatter + a
distilled body). Like todo, the SCOPE is git-repo aware, so no flag is needed
for the common case:

  in a git repo   → THAT repo's docs/kb/ (committed, travels with the clone) —
                    knowledge TRUE OF THIS REPO; the structured replacement for
                    ad-hoc docs/*.md notes.
  not in a repo   → the host store (~/.bashy/kb, $BASHY_KB_DIR to override) —
                    cross-repo / this-machine knowledge, shared by every agent.

Overrides: --base-dir <root> reads ANOTHER project's store (<root>/docs/kb/) so
one agent can travel repos in a session; --user forces the host store even inside
a repo; --repo forces the repo store; --dir <path> points at any store directly.
'kb search --federate' additionally reads the current repo's graph/weave rings.

The loop: SEARCH before undertaking a task; if nothing relevant, ADD a
candidate entry; after the task, RETRO — validate or correct what you
consulted (update / supersede / validate), never blind-append.

Write distilled strategy, not transcripts. Capture failures as guardrails
("X looks right but Y"). The description field is the routing surface —
phrase it as "what + WHEN this applies" with trigger keywords.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		// Resolve the scope ONCE, before any subcommand runs, and populate `dir`
		// so every subcommand's Open(*dir) lands on the right store. A header on
		// stderr names WHICH store, so which kb you are on is never in doubt.
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			label, err := resolveKBDir(&dir, forceRepo, forceUser, baseDir)
			if err != nil {
				return err
			}
			fmt.Fprintf(c.ErrOrStderr(), "kb [%s] %s\n", label, dir)
			return nil
		},
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.PersistentFlags().StringVar(&dir, "dir", "", "point at a kb store directory directly (bypasses scope detection)")
	cmd.PersistentFlags().BoolVar(&forceRepo, "repo", false, "force THIS repo's committed store (docs/kb/); error if not in a git repo")
	cmd.PersistentFlags().BoolVar(&forceUser, "user", false, "force the host store (~/.bashy/kb), even inside a repo")
	cmd.PersistentFlags().StringVar(&baseDir, "base-dir", "", "read ANOTHER project root's store (<root>/docs/kb/) — travel repos without cd")

	cmd.AddCommand(newSearchCmd(&dir))
	cmd.AddCommand(newShowCmd(&dir))
	cmd.AddCommand(newAddCmd(&dir))
	cmd.AddCommand(newUpdateCmd(&dir))
	cmd.AddCommand(newSupersedeCmd(&dir))
	cmd.AddCommand(newValidateCmd(&dir))
	cmd.AddCommand(newRetroCmd(&dir))
	cmd.AddCommand(newSourcesCmd(&dir))
	cmd.AddCommand(newTransferCmd(&dir))
	cmd.AddCommand(newListCmd(&dir))
	cmd.AddCommand(newIndexCmd(&dir))
	cmd.AddCommand(newLogCmd(&dir))
	return cmd
}

// --- search --------------------------------------------------------------

func newSearchCmd(dir *string) *cobra.Command {
	var (
		repo, goos     string
		tags           []string
		k              int
		all, jsonOut   bool
		full, federate bool
	)
	cmd := &cobra.Command{
		Use:   "search <term>...",
		Short: "Find relevant pages (run this BEFORE starting a task)",
		Long: `Deterministic ranked search over the page index: substring terms scored
against title/description/tags/body, filtered by activation scope
(repo/os), weighted by the validation ladder (validated > candidate >
stale; superseded excluded). Output is token-lean and capped small on
purpose — open a page with 'kb show <slug>' only when its description
matches the task.

--federate additionally reads the CURRENT repo's rings read-only: the
graph contribution log (.agents/bashy/graph/contrib.jsonl) and the weave
campaign memory (~/.bashy/weave/...). No terms lists everything (use
--tags to filter).`,
		Args: cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			store := Open(*dir)
			pages, err := store.List()
			if err != nil {
				return err
			}
			if repo == "" {
				if cwd, err := os.Getwd(); err == nil {
					if root := repoRootOf(cwd); root != "" {
						repo = filepath.Base(root)
					}
				}
			}
			// Tokenize the raw CLI args through the shared Terms() tokenizer
			// (as transfer.go does) so quoted task-shaped queries — "how do I
			// gate a merge" — match per word instead of as one 5-word term.
			terms := Terms(strings.Join(args, " "))
			q := Query{Terms: terms, Repo: repo, OS: goos, Tags: tags, K: k, All: all}
			hits := Search(pages, q)
			var fed []FedHit
			if federate {
				if cwd, err := os.Getwd(); err == nil {
					fed = FederatedSearch(cwd, terms, k)
				}
			}
			out := c.OutOrStdout()
			if jsonOut {
				return writeSearchJSON(out, hits, fed)
			}
			if len(hits) == 0 && len(fed) == 0 {
				fmt.Fprintln(out, "no matching kb pages — if this task teaches something durable, contribute one: bashy kb add")
				return nil
			}
			for _, h := range hits {
				p := h.Page
				fmt.Fprintf(out, "%s  [%s/%s] %s — %s\n", p.Slug, p.Status, p.Type, p.Title, p.Description)
				if full {
					if body := strings.TrimSpace(p.Body); body != "" {
						fmt.Fprintln(out, indent(body, "    "))
					}
				}
			}
			for _, f := range fed {
				fmt.Fprintf(out, "%s  %s\n", f.Origin, f.Text)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "filter to pages applicable to this repo basename (default: current repo)")
	cmd.Flags().StringVar(&goos, "os", runtime.GOOS, "filter to pages applicable to this OS")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "require at least one of these tags")
	cmd.Flags().IntVar(&k, "k", DefaultK, "max results")
	cmd.Flags().BoolVar(&all, "all", false, "include superseded pages")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	cmd.Flags().BoolVar(&full, "full", false, "print page bodies, not just index lines")
	cmd.Flags().BoolVar(&federate, "federate", false, "also search the current repo's contribution log + weave memory")
	return cmd
}

type searchHitJSON struct {
	Slug        string   `json:"slug"`
	Type        string   `json:"type"`
	Status      string   `json:"status"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Evidence    string   `json:"evidence,omitempty"`
	Body        string   `json:"body,omitempty"`
	Score       float64  `json:"score"`
}

func writeSearchJSON(w io.Writer, hits []Hit, fed []FedHit) error {
	payload := struct {
		Pages     []searchHitJSON `json:"pages"`
		Federated []FedHit        `json:"federated,omitempty"`
	}{Pages: []searchHitJSON{}, Federated: fed}
	for _, h := range hits {
		p := h.Page
		payload.Pages = append(payload.Pages, searchHitJSON{
			Slug: p.Slug, Type: p.Type, Status: p.Status, Title: p.Title,
			Description: p.Description, Tags: p.Tags, Evidence: p.Evidence,
			Body: p.Body, Score: h.Score,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// --- show ----------------------------------------------------------------

func newShowCmd(dir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show <slug>",
		Short: "Print one page (frontmatter + body)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			store := Open(*dir)
			b, err := os.ReadFile(store.PagePath(args[0]))
			if err != nil {
				return err
			}
			_, err = c.OutOrStdout().Write(b)
			return err
		},
	}
}

// --- add -----------------------------------------------------------------

// pageFlags are the authoring flags shared by add and supersede.
type pageFlags struct {
	typ, title, desc     string
	tags, repos          []string
	goos, evidence, slug string
	body, bodyFile       string
}

func (f *pageFlags) register(cmd *cobra.Command, requireTitle bool) {
	cmd.Flags().StringVar(&f.typ, "type", TypeLesson, "page type: lesson|gotcha|runbook|decision|fact")
	cmd.Flags().StringVar(&f.title, "title", "", "page title")
	cmd.Flags().StringVar(&f.desc, "description", "", "what + WHEN this applies (the routing surface)")
	cmd.Flags().StringSliceVar(&f.tags, "tags", nil, "tags")
	cmd.Flags().StringSliceVar(&f.repos, "repos", nil, "scope: applies only to these repo basenames")
	cmd.Flags().StringVar(&f.goos, "os", "", "scope: applies only to this OS (GOOS value)")
	cmd.Flags().StringVar(&f.evidence, "evidence", "", "how this was verified (command, commit, issue)")
	cmd.Flags().StringVar(&f.slug, "slug", "", "override the auto slug")
	cmd.Flags().StringVar(&f.body, "body", "", "page body text")
	cmd.Flags().StringVarP(&f.bodyFile, "file", "f", "", "read the body from FILE ('-' = stdin)")
	if requireTitle {
		_ = cmd.MarkFlagRequired("title")
		_ = cmd.MarkFlagRequired("description")
	}
}

func (f *pageFlags) buildPage(c *cobra.Command) (*Page, error) {
	if !ValidType(f.typ) {
		return nil, fmt.Errorf("kb: invalid type %q (lesson|gotcha|runbook|decision|fact)", f.typ)
	}
	body := f.body
	if f.bodyFile != "" {
		var b []byte
		var err error
		if f.bodyFile == "-" {
			b, err = io.ReadAll(c.InOrStdin())
		} else {
			b, err = os.ReadFile(f.bodyFile)
		}
		if err != nil {
			return nil, err
		}
		body = string(b)
	}
	slug := f.slug
	if slug == "" {
		slug = Slugify(f.title)
	}
	p := &Page{
		Slug:        slug,
		Type:        f.typ,
		Title:       f.title,
		Description: strings.TrimSpace(f.desc),
		Tags:        f.tags,
		Evidence:    f.evidence,
		Status:      StatusCandidate,
		Source:      &Source{Tool: ToolID(), Host: HostID(), Episode: EpisodeID()},
		Body:        strings.TrimSpace(body),
	}
	if len(f.repos) > 0 || f.goos != "" {
		p.Scope = &Scope{Repos: f.repos, OS: f.goos}
	}
	return p, nil
}

func newAddCmd(dir *string) *cobra.Command {
	var flags pageFlags
	var force bool
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Contribute a new page (reconciles against existing pages first)",
		Long: `Add a candidate page. The add RECONCILES first: if an existing live page
looks like the same knowledge, add refuses and points at it — update or
supersede that page instead (--force overrides). Distill strategy, not
transcript; failures are as valuable as successes.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			store := Open(*dir)
			p, err := flags.buildPage(c)
			if err != nil {
				return err
			}
			pages, err := store.List()
			if err != nil {
				return err
			}
			if !force {
				if dup := NearDuplicate(pages, p.Title, p.Description); dup != nil {
					return fmt.Errorf("kb: looks like a duplicate of %q (%s) — use 'kb update %s' or 'kb supersede %s' (or --force)",
						dup.Title, dup.Slug, dup.Slug, dup.Slug)
				}
			}
			if err := store.Write(p, "add"); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "added %s (candidate) — validate after it proves out: bashy kb validate %s --evidence \"...\"\n", p.Slug, p.Slug)
			return nil
		},
	}
	flags.register(cmd, true)
	cmd.Flags().BoolVar(&force, "force", false, "skip the duplicate check")
	return cmd
}

// --- update --------------------------------------------------------------

func newUpdateCmd(dir *string) *cobra.Command {
	var (
		title, desc, status, evidence string
		tags                          []string
		body, bodyFile                string
	)
	cmd := &cobra.Command{
		Use:   "update <slug>",
		Short: "Refresh an existing page (the entry was right — extend or correct in place)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			store := Open(*dir)
			p, err := store.Load(args[0])
			if err != nil {
				return err
			}
			changed := false
			set := func(dst *string, v string) {
				if v != "" {
					*dst = v
					changed = true
				}
			}
			set(&p.Title, title)
			set(&p.Description, strings.TrimSpace(desc))
			set(&p.Evidence, evidence)
			if status != "" {
				switch status {
				case StatusCandidate, StatusValidated, StatusStale, StatusSuperseded:
					p.Status = status
					changed = true
				default:
					return fmt.Errorf("kb: invalid status %q (candidate|validated|stale|superseded)", status)
				}
			}
			if len(tags) > 0 {
				p.Tags = tags
				changed = true
			}
			if bodyFile != "" {
				var b []byte
				var rerr error
				if bodyFile == "-" {
					b, rerr = io.ReadAll(c.InOrStdin())
				} else {
					b, rerr = os.ReadFile(bodyFile)
				}
				if rerr != nil {
					return rerr
				}
				body = string(b)
			}
			if body != "" {
				p.Body = strings.TrimSpace(body)
				changed = true
			}
			if !changed {
				return fmt.Errorf("kb: nothing to update — pass at least one of --title/--description/--status/--evidence/--tags/--body/-f")
			}
			if err := store.Write(p, "update"); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "updated %s\n", p.Slug)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new title (slug is kept)")
	cmd.Flags().StringVar(&desc, "description", "", "new description")
	cmd.Flags().StringVar(&status, "status", "", "new status: candidate|validated|stale|superseded")
	cmd.Flags().StringVar(&evidence, "evidence", "", "new evidence")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "replace tags")
	cmd.Flags().StringVar(&body, "body", "", "replace the body text")
	cmd.Flags().StringVarP(&bodyFile, "file", "f", "", "replace the body from FILE ('-' = stdin)")
	return cmd
}

// --- supersede -----------------------------------------------------------

func newSupersedeCmd(dir *string) *cobra.Command {
	var flags pageFlags
	cmd := &cobra.Command{
		Use:   "supersede <slug>",
		Short: "Replace a wrong page with a corrected one (the correction stays linked, nothing is deleted)",
		Long: `Write a new page and flip the old one to status: superseded, with
supersedes/superseded_by links both ways. Never delete knowledge — an
invalidated lesson plus its correction is itself knowledge.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			store := Open(*dir)
			old, err := store.Load(args[0])
			if err != nil {
				return err
			}
			p, err := flags.buildPage(c)
			if err != nil {
				return err
			}
			if p.Slug == old.Slug {
				return fmt.Errorf("kb: new page needs its own identity — pass a different --title or --slug")
			}
			p.Supersedes = old.Slug
			if err := store.Write(p, "supersede"); err != nil {
				return err
			}
			old.Status = StatusSuperseded
			old.SupersededBy = p.Slug
			if err := store.Write(old, "supersede"); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "superseded %s -> %s\n", old.Slug, p.Slug)
			return nil
		},
	}
	flags.register(cmd, true)
	return cmd
}

// --- validate ------------------------------------------------------------

func newValidateCmd(dir *string) *cobra.Command {
	var evidence string
	cmd := &cobra.Command{
		Use:   "validate <slug>",
		Short: "Promote a candidate to validated (requires evidence)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			store := Open(*dir)
			p, err := store.Load(args[0])
			if err != nil {
				return err
			}
			p.Status = StatusValidated
			p.Evidence = evidence
			if err := store.Write(p, "validate"); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "validated %s\n", p.Slug)
			return nil
		},
	}
	cmd.Flags().StringVar(&evidence, "evidence", "", "how it was verified (command, commit, issue)")
	_ = cmd.MarkFlagRequired("evidence")
	return cmd
}

// --- retro ---------------------------------------------------------------

func newRetroCmd(dir *string) *cobra.Command {
	var k int
	cmd := &cobra.Command{
		Use:   "retro [<term>...]",
		Short: "Post-task write-back: review related pages, then ADD / UPDATE / SUPERSEDE / VALIDATE / NOOP",
		Long: `Run AFTER completing a task. Shows the pages related to what you just did
and the one decision to make about the knowledge — never blind-append.
Deterministic (no LLM): the judgment is yours, retro structures it.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			store := Open(*dir)
			pages, err := store.List()
			if err != nil {
				return err
			}
			repo := ""
			if cwd, err := os.Getwd(); err == nil {
				if root := repoRootOf(cwd); root != "" {
					repo = filepath.Base(root)
				}
			}
			hits := Search(pages, Query{Terms: Terms(strings.Join(args, " ")), Repo: repo, OS: runtime.GOOS, K: k})
			out := c.OutOrStdout()
			fmt.Fprintln(out, "kb retro — post-task knowledge write-back")
			if len(args) > 0 {
				fmt.Fprintf(out, "related pages (query: %s):\n", strings.Join(args, " "))
			} else {
				fmt.Fprintln(out, "related pages:")
			}
			if len(hits) == 0 {
				fmt.Fprintln(out, "  (none)")
			}
			for _, h := range hits {
				p := h.Page
				fmt.Fprintf(out, "  %s  [%s/%s] %s — %s\n", p.Slug, p.Status, p.Type, p.Title, p.Description)
			}
			fmt.Fprint(out, `decide ONE:
  ADD        bashy kb add --type lesson --title "<distilled insight>" --description "<what + WHEN it applies>"   # nothing relevant existed and the task taught something durable
  UPDATE     bashy kb update <slug> --evidence "..."       # a page was right — extend/refresh it
  SUPERSEDE  bashy kb supersede <slug> --title "..." ...   # a page was wrong — replace it, correction stays linked
  VALIDATE   bashy kb validate <slug> --evidence "..."     # a candidate page was consulted and proved correct
  NOOP       nothing durable learned — do nothing
write distilled strategy, not transcript; capture failures as guardrails.
`)
			return nil
		},
	}
	cmd.Flags().IntVar(&k, "k", DefaultK, "max related pages shown")
	return cmd
}

// --- list / index / log --------------------------------------------------

func newListCmd(dir *string) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every page, one line each",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			store := Open(*dir)
			pages, err := store.List()
			if err != nil {
				return err
			}
			if jsonOut {
				return writeSearchJSON(c.OutOrStdout(), toHits(pages), nil)
			}
			for _, p := range pages {
				fmt.Fprintf(c.OutOrStdout(), "%s  [%s/%s] %s — %s\n", p.Slug, p.Status, p.Type, p.Title, p.Description)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func toHits(pages []*Page) []Hit {
	out := make([]Hit, 0, len(pages))
	for _, p := range pages {
		out = append(out, Hit{Page: p})
	}
	return out
}

func newIndexCmd(dir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "index",
		Short: "Regenerate index.md (the always-load entry point)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			store := Open(*dir)
			if err := store.RebuildIndex(); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "wrote %s\n", filepath.Join(store.Dir(), "index.md"))
			return nil
		},
	}
}

func newLogCmd(dir *string) *cobra.Command {
	var n int
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show recent journal entries (who wrote what, when)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			store := Open(*dir)
			lines, err := store.JournalTail(n)
			if err != nil {
				return err
			}
			for _, l := range lines {
				fmt.Fprintln(c.OutOrStdout(), l)
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&n, "n", "n", 20, "number of entries")
	return cmd
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}
