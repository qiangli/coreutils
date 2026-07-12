// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package lexicon

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/fleet"
)

// Synopses is set by the embedding shell (bashy) so the lexicon can carry a verb's
// one-liner. The atlas holds classification, not prose; this keeps the package
// usable by any project rather than hard-wiring bashy's help text.
var Synopses = map[string]string{}

// NewLexiconCmd builds `bashy lexicon`.
func NewLexiconCmd(opts ...fleet.Option) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lexicon",
		Short: "what do this project's words mean HERE?",
		Long: `lexicon is the project's jargon, projected from the registries that already
define it — never a hand-written glossary.

The problem it solves: a user says "handoff this to codex". Neither word means what
the dictionary says. "handoff" is a bashy verb; "codex" is an agent binding ON THIS
HOST (a CLI tool plus a bound model), and the same word denotes a different binding
on another machine.

It is 20% glossary and 80% PRECEDENCE + LOOKUP:

  precedence   one sentence, in the always-on tier every tool reads:
               "in this workspace these words are never their English senses"
  lookup       a name is RESOLVED, never memorised

It STORES NOTHING. Verbs are projected from the Command Atlas, agent bindings from
the fleet registry. Only two things are hand-written, because a machine cannot infer
them: what the team actually SAYS (alt labels), and the precedence rule (scope
notes). Everything that can go stale is generated.

In written artifacts a term may be marked [[handoff]] — that is how the term set is
TAUGHT and how mentions become machine-detectable. In conversation the word is used
plainly, like any jargon. The marker is optional emphasis, never required syntax.`,
		Example: `  bashy lexicon                          # the vocabulary of this project
  bashy lexicon resolve codex --json     # what does that word mean HERE?
  bashy lexicon emit --write AGENTS.md   # seed every tool's always-on tier
  bashy lexicon scan docs/               # find [[terms]] that resolve to NOTHING`,
	}
	cmd.AddCommand(newListCmd(opts), newResolveCmd(opts), newEmitCmd(opts), newScanCmd(opts))
	return cmd
}

func build(opts []fleet.Option) *Store {
	host, _ := os.Hostname()
	return Build(fleet.New(opts...), Synopses, host, Overlay{})
}

func newListCmd(opts []fleet.Option) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "every term that resolves in this project",
		RunE: func(cmd *cobra.Command, args []string) error {
			s := build(opts)
			if asJSON {
				b, _ := json.MarshalIndent(s.Concepts, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
				return nil
			}
			for _, c := range s.Concepts {
				fmt.Fprintf(cmd.OutOrStdout(), "%-22s %-14s %s\n", c.PrefLabel, c.Kind, oneLine(c.Definition))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the concepts")
	return cmd
}

func newResolveCmd(opts []fleet.Option) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "resolve <term>",
		Short: "what does this word mean HERE?",
		Long: `resolve answers the question that makes jargon work: what does this word denote
in THIS workspace, on THIS host?

A name is resolved by a lookup, never memorised. That one rule is what lets "codex"
mean a live binding here and something else on another machine, without anyone
maintaining a glossary.

It accepts the bare word, the [[marked]] form, and a namespaced form ([[agent:codex]]).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := build(opts)
			c, ok := s.Resolve(args[0])
			if !ok {
				// An unknown term is an ERROR, not an empty answer. Silence would
				// invite the agent to fall back on the English word — the exact
				// failure this whole feature exists to prevent.
				return fmt.Errorf("%q is not a term in this project's lexicon.\n"+
					"It may simply be an ordinary English word — but if you expected it to name "+
					"something here, `bashy lexicon list` shows what does", args[0])
			}
			if asJSON {
				b, _ := json.MarshalIndent(c, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
				return nil
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s  (%s)\n", c.PrefLabel, c.Kind)
			if c.Definition != "" {
				fmt.Fprintf(out, "  %s\n", c.Definition)
			}
			if c.Host != "" {
				fmt.Fprintf(out, "  host:  %s\n", c.Host)
			}
			if len(c.AltLabels) > 0 {
				fmt.Fprintf(out, "  also: %s\n", strings.Join(c.AltLabels, ", "))
			}
			if c.Use != "" {
				fmt.Fprintf(out, "  use:  %s\n", c.Use)
			}
			if c.ScopeNote != "" {
				fmt.Fprintf(out, "  note: %s\n", c.ScopeNote)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the concept")
	return cmd
}

func newEmitCmd(opts []fleet.Option) *cobra.Command {
	var write string
	cmd := &cobra.Command{
		Use:   "emit",
		Short: "render the managed lexicon block for AGENTS.md / CLAUDE.md",
		Long: `emit renders the always-on tier: the precedence rule, a SELECTION of the
highest-value terms, and the resolver command.

A selection, not a dump — deliberately. Term/tool selection accuracy DEGRADES past
roughly 15-20 items in active rotation, and near-synonyms are the top failure mode.
More vocabulary in context does not mean better resolution. The long tail is reached
by lookup, which is why the resolver line is the most important one in the block.

With --write it splices the block into a file, replacing any previous one.
Idempotent, so it is safe to wire into a hook or a gate.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := build(opts)
			cwd, _ := os.Getwd()
			block := s.EmitAgentsMD(filepath.Base(cwd))
			if write == "" {
				fmt.Fprint(cmd.OutOrStdout(), block)
				return nil
			}
			if err := WriteInto(write, block); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "lexicon: managed block written into %s\n", write)
			return nil
		},
	}
	cmd.Flags().StringVar(&write, "write", "", "splice the block into this file (e.g. AGENTS.md)")
	return cmd
}

func newScanCmd(opts []fleet.Option) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan [path...]",
		Short: "find [[terms]] in artifacts that resolve to NOTHING",
		Long: `scan walks the project's artifacts, collects every [[marked]] term, and reports
the ones that resolve to nothing.

This is what makes the lexicon FALSIFIABLE, and it is the property a prose glossary
can never have: a prose glossary rots silently, a linked one cannot. A [[term]] that
resolves to nothing is a broken link, and broken links are findable.

It also means the term set is derived from HOW THE TEAM ACTUALLY WRITES, rather than
from a list someone has to remember to maintain.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				args = []string{"."}
			}
			s := build(opts)
			broken := map[string][]string{}
			for _, root := range args {
				_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
					if err != nil || d.IsDir() {
						if d != nil && d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
							return filepath.SkipDir
						}
						return nil
					}
					if !strings.HasSuffix(p, ".md") {
						return nil
					}
					b, err := os.ReadFile(p)
					if err != nil {
						return nil
					}
					if u := s.Unresolved(string(b)); len(u) > 0 {
						broken[p] = u
					}
					return nil
				})
			}
			if len(broken) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "lexicon: every [[term]] resolves")
				return nil
			}
			for p, terms := range broken {
				for _, t := range terms {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: [[%s]] resolves to nothing\n", p, t)
				}
			}
			return fmt.Errorf("%d file(s) contain terms that resolve to nothing", len(broken))
		},
	}
	return cmd
}
