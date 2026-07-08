package kb

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// kb transfer is the retro-shaped helper for agent-to-agent knowledge
// transfer: it prints the deterministic ground truth (detected sources,
// what each source already contributed, the pages that already exist for
// the topic) and the checklist of literal commands — then gets out of the
// way. The judgment (what to transfer, how to distill) is the agent's,
// guided by the knowledge-transfer skill. Writes nothing, ever.

func newTransferCmd(dir *string) *cobra.Command {
	var k int
	cmd := &cobra.Command{
		Use:   "transfer [<topic term>...]",
		Short: "Structure a knowledge transfer: sources, existing pages, and the checklist (writes nothing)",
		Long: `Run when one agent's knowledge (private memory, in-context recall) should
become team knowledge other agents on this host inherit. Prints the ground
truth — detected source stores, what each already contributed (xfer:<source>
tags), existing pages related to the topic — and the transfer checklist.

Deterministic (no LLM) and read-only: the judgment is yours, transfer
structures it. The full procedure is the knowledge-transfer skill:
bashy skills show knowledge-transfer.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			store := Open(*dir)
			pages, err := store.List()
			if err != nil {
				return err
			}
			home, err := os.UserHomeDir()
			if err != nil {
				home = ""
			}
			cwd, _ := os.Getwd()
			repo := ""
			if cwd != "" {
				if root := repoRootOf(cwd); root != "" {
					repo = filepath.Base(root)
				}
			}
			out := c.OutOrStdout()
			fmt.Fprintln(out, "kb transfer — one agent's knowledge into the team kb")

			fmt.Fprintln(out, "sources on this host (read-only — kb never writes them):")
			writeSourcesSummary(out, DetectSources(home, cwd), TransferredCounts(pages))

			// Related pages for the topic — free text tokenized through the
			// shared Terms() tokenizer, so quoted task-shaped queries match.
			terms := Terms(strings.Join(args, " "))
			if len(args) > 0 {
				fmt.Fprintf(out, "existing pages (query: %s):\n", strings.Join(args, " "))
				hits := Search(pages, Query{Terms: terms, Repo: repo, OS: runtime.GOOS, K: k})
				if len(hits) == 0 {
					fmt.Fprintln(out, "  (none — greenfield topic)")
				}
				for _, h := range hits {
					p := h.Page
					fmt.Fprintf(out, "  %s  [%s/%s] %s — %s\n", p.Slug, p.Status, p.Type, p.Title, p.Description)
				}
			} else {
				fmt.Fprintln(out, "existing pages: pass topic terms to see them (kb transfer <topic>)")
			}

			fmt.Fprint(out, `per selected claim (durable + team-relevant + non-derivable, redacted):
  FACT/GOTCHA  bashy kb add --type gotcha --title "..." --description "<what + WHEN>" --tags xfer:<source> --evidence "..."
  PROCEDURE    bashy skills learn <dir>            # executable + checkable contract -> a skill, not a page
  EXISTS-OK    bashy kb update <slug> ...          # page was right - extend it
  EXISTS-WRONG bashy kb supersede <slug> ...       # page was wrong - correction stays linked
  SKIP         (record the reason in your transfer report)
tag every transferred page xfer:<source> (claude-memory|memex|weave-memory|repo-graph|recall);
transferred pages land as CANDIDATE - a SECOND agent validates through use:
  bashy kb validate <slug> --evidence "used in <task>, held"
full procedure: bashy skills show knowledge-transfer
`)
			return nil
		},
	}
	cmd.Flags().IntVar(&k, "k", DefaultK, "max related pages shown")
	return cmd
}
