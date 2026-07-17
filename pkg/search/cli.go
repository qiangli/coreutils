package search

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/telemetry"
)

// NewSearchCmd is `bashy search` — the find-things primitive. P0a is web search
// through the provider ladder; `--local` (grep/find/ast/kb/graph) is P0b.
func NewSearchCmd() *cobra.Command {
	var (
		asJSON  bool
		local   bool
		max     int
		backend string
	)
	cmd := &cobra.Command{
		Use:   "search QUERY...",
		Short: "web search (query → cited results) — the find-things primitive",
		Long: "Search the web through a provider ladder (auto by available key, or --backend):\n" +
			"  1. tavily  (TAVILY_API_KEY)\n" +
			"  2. brave   (BRAVE_API_KEY)\n" +
			"  3. serper  (SERPER_API_KEY)\n" +
			"Keys come from the environment (project them with `eval \"$(bashy secrets env)\"`).\n" +
			"Results are cited (url + retrieved-at) so a caller can verify they resolve.",
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(c *cobra.Command, args []string) error {
			if local {
				return fmt.Errorf("search: --local is not implemented yet (P0b) — use grep/find/ast/kb/graph for now")
			}
			query := strings.Join(args, " ")
			ctx := c.Context()
			results, used, err := Web(ctx, query, Options{MaxResults: max, Backend: backend})
			if err != nil {
				return err
			}
			telemetry.Provenance(ctx, "search.results", int64(len(results)), used)

			if asJSON {
				out := struct {
					SchemaVersion string   `json:"schema_version"`
					Query         string   `json:"query"`
					Backend       string   `json:"backend"`
					Count         int      `json:"count"`
					Results       []Result `json:"results"`
				}{"bashy-search-v1", query, used, len(results), results}
				b, _ := json.MarshalIndent(out, "", "  ")
				fmt.Println(string(b))
				return nil
			}
			for i, r := range results {
				fmt.Printf("%d. %s\n   %s\n", i+1, r.Title, r.URL)
				if s := strings.TrimSpace(r.Snippet); s != "" {
					fmt.Printf("   %s\n", truncate(s, 200))
				}
			}
			fmt.Fprintf(os.Stderr, "(%d results via %s)\n", len(results), used)
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&local, "local", false, "local search (grep/find/ast/kb/graph) — P0b, not yet implemented")
	f.BoolVar(&asJSON, "json", false, "print a bashy-search-v1 JSON envelope")
	f.IntVar(&max, "max", 8, "maximum results")
	f.StringVar(&backend, "backend", "", "force a backend: tavily | brave | serper (default: auto by available key)")
	return cmd
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
