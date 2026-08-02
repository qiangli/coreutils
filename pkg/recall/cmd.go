package recall

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/craft"
	"github.com/qiangli/coreutils/pkg/kb"
	"github.com/qiangli/coreutils/pkg/redact"
)

// ExitBrokenRing is returned when a ring EXISTED but could not be read.
//
// The exit-code contract is the part third parties depend on most, and it has one
// rule that is easy to get wrong: an EMPTY ANSWER IS EXIT 0. A host that knows
// nothing about a topic is a fact, not a failure. Only a ring that was present
// and unreadable is exit 1 — because a caller that cannot distinguish "nothing is
// known" from "something broke" will proceed on a false negative, which is the
// specific harm this package exists to avoid.
const ExitBrokenRing = 1

// NewRecallCmd builds the `recall` verb.
func NewRecallCmd() *cobra.Command {
	var (
		asJSON     bool
		k          int
		budget     int
		resolution string
		rings      []string
		since      time.Duration
		minCover   float64
		explain    bool
		storeDir   string
	)

	cmd := &cobra.Command{
		Use:   "recall <query>...",
		Short: "what is known about a topic, across every memory ring",
		Long: `recall is the third-party READ surface over this host's memory.

It answers "what is known about X" for any tool, not just bashy: ranked hits from
each memory ring, every one carrying its ring label, a re-openable source pointer,
its validation status, and why it matched.

Three things about its behaviour are contract, not implementation:

  EMPTY IS SUCCESS. A host that knows nothing exits 0 with no hits. Only a ring
  that exists and cannot be read exits 1, so a caller can always tell "nothing
  known" from "something broke".

  --k IS PER RING. Scores from different rings are not comparable — a kb page's
  BM25 score is computed over prose lessons, a capability's over contracts — so
  each ring contributes at most k hits ranked within itself and rings are emitted
  in precedence order. Fusing them was measured and rejected.

  FACTS ARE NOT A RING. craft facts are entity-bound and host-local with no export
  path, and that absence is the enforcement. recall's output is designed to be
  piped into software we do not control, so it can never read them — not behind a
  flag, not with --all. A fold derived from facts is the supported path.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := Query{
				Text: strings.Join(args, " "),
				K:    k, Budget: budget, Rings: rings,
				Since: since, MinCoverage: minCover,
				Resolution: parseResolution(resolution),
				OS:         hostOS(),
				Repo:       repoName(),
			}
			res := Recall(q, openRings(storeDir)...)

			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(res); err != nil {
					return err
				}
			} else {
				render(out, res, explain)
			}
			// Errors are reported in the envelope AND in the exit code: a
			// machine reads the code, a human reads the lines.
			if len(res.Errors) > 0 {
				for _, e := range res.Errors {
					fmt.Fprintf(cmd.ErrOrStderr(), "recall: ring %s: %s\n", e.Ring, e.Err)
				}
				os.Exit(ExitBrokenRing)
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.BoolVar(&asJSON, "json", false, "emit the recall-v1 envelope")
	f.IntVar(&k, "k", 3, "max hits PER RING (not a global cap — see --help)")
	f.IntVar(&budget, "budget", 2000, "hard ceiling on rendered output, in tokens (0 = none)")
	f.StringVar(&resolution, "resolution", "line", "how much of each record to render: cue|line|full")
	f.StringSliceVar(&rings, "ring", nil, "restrict to these rings (repeatable); default all")
	f.DurationVar(&since, "since", 0, "ignore records older than this")
	f.Float64Var(&minCover, "min-coverage", 0, "abstain unless a hit covers this fraction of the query")
	f.BoolVar(&explain, "explain", false, "print the ranking inputs, not just why")
	f.StringVar(&storeDir, "store", "", "override the store root (default ~/.bashy)")
	return cmd
}

// openRings resolves the readable rings. A MISSING store is not an error — it is
// a host that has not learned anything there yet — so each reader is handed a nil
// store rather than failing the whole call.
func openRings(root string) []Reader {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		root = filepath.Join(home, ".bashy")
	}
	var rs []Reader
	if kbDir := filepath.Join(root, "kb"); isDir(kbDir) {
		rs = append(rs, HostRing{Store: kb.Open(kbDir)})
	}
	if craftDir := filepath.Join(root, "craft"); isDir(craftDir) {
		rs = append(rs, CapabilityRing{Folds: craft.OpenFolds(craftDir, redact.New())})
	}
	return rs
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func parseResolution(s string) kb.Resolution {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "cue":
		return kb.ResCue
	case "full":
		return kb.ResFull
	default:
		return kb.ResLine
	}
}

func hostOS() string { return osGOOS }

func repoName() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for d := wd; ; {
		if isDir(filepath.Join(d, ".git")) {
			return filepath.Base(d)
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// render writes the human view. Deliberately citation-shaped: every line carries
// its ring and id so a reader can go open the source. There is no --summarise and
// there must not be one — that is the flag that turns a citation tool into a
// confabulation surface.
func render(w io.Writer, res Result, explain bool) {
	if res.Abstained {
		fmt.Fprintf(w, "nothing is known about %q above the coverage threshold\n", res.Query)
		return
	}
	if len(res.Hits) == 0 {
		fmt.Fprintf(w, "nothing is known about %q\n", res.Query)
		return
	}
	ring := ""
	for _, h := range res.Hits {
		if h.Ring != ring {
			ring = h.Ring
			fmt.Fprintf(w, "\n%s\n", ring)
		}
		fmt.Fprintf(w, "  %-44s %s\n", truncate(h.Cue, 44), h.ID)
		if h.Gist != "" {
			fmt.Fprintf(w, "  %-44s %s\n", "", truncate(h.Gist, 60))
		}
		if explain {
			fmt.Fprintf(w, "  %-44s score %.2f  why: %s\n", "", h.Score, strings.Join(h.Why, ", "))
			for _, s := range h.Source {
				fmt.Fprintf(w, "  %-44s source %s\n", "", s.URI)
			}
		}
		if h.Compose != "" {
			fmt.Fprintf(w, "  %-44s → %s\n", "", h.Compose)
		}
	}
	if res.Budget.Truncated {
		fmt.Fprintf(w, "\n(truncated at %d/%d tokens — raise --budget for more)\n",
			res.Budget.Spent, res.Budget.Tokens)
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
