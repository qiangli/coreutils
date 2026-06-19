// Package yccmd registers `yc`, the AgentOS code-intelligence command:
// treesitter-backed symbol search and a token-budgeted repo map, reachable
// through every coreutils surface (shell builtin, multicall, MCP run_tool).
//
// The verbs are thin CLI wrappers; the engines live in
// coreutils/pkg/{treesitter,repomap}. The gfy-backed `graph` verb is left
// out here so the bare binary stays free of gfy's document-parsing deps.
package yccmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/pkg/repomap"
	"github.com/qiangli/coreutils/pkg/treesitter"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "yc",
	Synopsis: "AgentOS code intelligence: symbols, search-symbols, refs, repomap",
	Usage:    "yc <verb> [args...]   (verbs: symbols, search-symbols, refs, repomap)",
}

func init() {
	cmd.Run = run
	tool.Register(cmd)
}

func run(rc *tool.RunContext, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(rc.Err, "yc: missing verb. Try: symbols | search-symbols | refs | repomap")
		return 2
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "symbols":
		return runSymbols(rc, rest)
	case "search-symbols":
		return runSearchSymbols(rc, rest)
	case "refs":
		return runRefs(rc, rest)
	case "repomap":
		return runRepomap(rc, rest)
	case "help", "--help", "-h":
		fmt.Fprintln(rc.Out, cmd.Usage)
		return 0
	default:
		fmt.Fprintf(rc.Err, "yc: unknown verb %q. Try: symbols | search-symbols | refs | repomap\n", verb)
		return 2
	}
}

// --- shared helpers (parity with ycode's builtins) ---

var supportedExtension = map[string]string{
	"go": "go", "py": "python", "js": "javascript", "jsx": "javascript",
	"ts": "typescript", "tsx": "tsx", "rs": "rust", "java": "java",
	"c": "c", "h": "c", "rb": "ruby",
}

func languageFromPath(path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	return supportedExtension[ext]
}

// walkSourceFiles invokes fn for every supported source file under root. If
// root is a single file, fn is called once. Skips .git/node_modules/vendor.
func walkSourceFiles(root string, fn func(path, lang string) error) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		if lang := languageFromPath(root); lang != "" {
			return fn(root, lang)
		}
		return fmt.Errorf("file %q has unsupported language", root)
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if lang := languageFromPath(path); lang != "" {
			return fn(path, lang)
		}
		return nil
	})
}

func formatSymbolLine(s treesitter.Symbol) string {
	if s.Signature != "" {
		return s.Signature
	}
	return s.Kind + " " + s.Name
}

// --- yc symbols ---

func runSymbols(rc *tool.RunContext, args []string) int {
	asJSON := false
	var target string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			if target == "" {
				target = a
			}
		}
	}
	if target == "" {
		fmt.Fprintln(rc.Err, "yc symbols: missing path argument")
		return 2
	}
	abs := rc.Path(target)
	parser := treesitter.NewParser()

	var all []treesitter.Symbol
	if err := walkSourceFiles(abs, func(path, lang string) error {
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		tree, perr := parser.Parse(rc.Ctx, src, lang)
		if perr != nil {
			return nil
		}
		all = append(all, parser.ExtractSymbols(tree, path)...)
		return nil
	}); err != nil {
		fmt.Fprintf(rc.Err, "yc symbols: %v\n", err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(rc.Out)
		enc.SetIndent("", "  ")
		_ = enc.Encode(all)
		return 0
	}
	for _, s := range all {
		fmt.Fprintf(rc.Out, "%s:%d: %s\n", s.File, s.Line, formatSymbolLine(s))
	}
	if len(all) == 0 {
		fmt.Fprintln(rc.Err, "(no symbols found)")
	}
	return 0
}

// --- yc search-symbols ---

func runSearchSymbols(rc *tool.RunContext, args []string) int {
	asJSON := false
	var pattern, target string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			if pattern == "" {
				pattern = a
			} else if target == "" {
				target = a
			}
		}
	}
	if pattern == "" {
		fmt.Fprintln(rc.Err, "yc search-symbols: missing pattern")
		return 2
	}
	if target == "" {
		target = "."
	}
	abs := rc.Path(target)
	parser := treesitter.NewParser()

	var matches []treesitter.Symbol
	needle := strings.ToLower(pattern)
	if err := walkSourceFiles(abs, func(path, lang string) error {
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		tree, perr := parser.Parse(rc.Ctx, src, lang)
		if perr != nil {
			return nil
		}
		for _, s := range parser.ExtractSymbols(tree, path) {
			if strings.Contains(strings.ToLower(s.Name), needle) {
				matches = append(matches, s)
			}
		}
		return nil
	}); err != nil {
		fmt.Fprintf(rc.Err, "yc search-symbols: %v\n", err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(rc.Out)
		enc.SetIndent("", "  ")
		_ = enc.Encode(matches)
		return 0
	}
	for _, s := range matches {
		fmt.Fprintf(rc.Out, "%s:%d: %s\n", s.File, s.Line, formatSymbolLine(s))
	}
	if len(matches) == 0 {
		fmt.Fprintln(rc.Err, "(no symbols match)")
		return 1
	}
	return 0
}

// --- yc refs ---

func runRefs(rc *tool.RunContext, args []string) int {
	asJSON := false
	var symbol, workspace string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			if symbol == "" {
				symbol = a
			} else if workspace == "" {
				workspace = a
			}
		}
	}
	if symbol == "" {
		fmt.Fprintln(rc.Err, "yc refs: missing symbol")
		return 2
	}
	if workspace == "" {
		workspace = "."
	}
	abs := rc.Path(workspace)
	parser := treesitter.NewParser()

	impacts, err := treesitter.Analyze(rc.Ctx, parser, symbol, "", abs)
	if err != nil {
		fmt.Fprintf(rc.Err, "yc refs: %v\n", err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(rc.Out)
		enc.SetIndent("", "  ")
		_ = enc.Encode(impacts)
		return 0
	}
	for _, im := range impacts {
		fmt.Fprintf(rc.Out, "%s:%d: %-12s %s\n", im.File, im.Line, im.Kind, im.Symbol)
		if im.Context != "" {
			fmt.Fprintf(rc.Out, "    %s\n", strings.TrimSpace(im.Context))
		}
	}
	if len(impacts) == 0 {
		fmt.Fprintln(rc.Err, "(no references found)")
		return 1
	}
	return 0
}

// --- yc repomap ---

func runRepomap(rc *tool.RunContext, args []string) int {
	asJSON := false
	target := ""
	opts := repomap.DefaultOptions()
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case strings.HasPrefix(a, "--budget="):
			n := 0
			if _, err := fmt.Sscanf(a[len("--budget="):], "%d", &n); err != nil || n <= 0 {
				fmt.Fprintf(rc.Err, "yc repomap: invalid --budget value %q\n", a)
				return 2
			}
			opts.MaxTokens = n
		case strings.HasPrefix(a, "--query="):
			opts.RelevanceQuery = a[len("--query="):]
		default:
			if target == "" {
				target = a
			}
		}
	}
	if target == "" {
		target = rc.Dir
		if target == "" {
			target = "."
		}
	} else {
		target = rc.Path(target)
	}

	rm, err := repomap.Generate(target, opts)
	if err != nil {
		fmt.Fprintf(rc.Err, "yc repomap: %v\n", err)
		return 1
	}
	if asJSON {
		enc := json.NewEncoder(rc.Out)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rm.Entries)
		return 0
	}
	fmt.Fprint(rc.Out, rm.Format())
	return 0
}
