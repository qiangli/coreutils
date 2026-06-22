// Package secrets is the `bashy secrets ...` front door to cloudbox's
// AES-encrypted API-key vault. It replaces a plaintext shell rc file of API
// keys/tokens (the historical `~/.novigensrc` pattern) with a single
//
//	eval "$(bashy secrets env)"
//
// line: the secrets live encrypted in cloudbox (one place to rotate /
// revoke / audit), and the rc file holds no secret material at all.
//
// It is part of the AgentOS hub (consumed by bashy as `bashy secrets`),
// stdlib + cobra only — no new dependency. The on-the-wire contract is
// cloudbox's Bearer /api/v1/secrets surface (secrets:read for env/ls/get,
// secrets:write for set/import/rm).
package secrets

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// NewSecretsCmd returns the `secrets` cobra command tree — the
// host-agnostic entry point a front end mounts (e.g. `bashy secrets`).
func NewSecretsCmd() *cobra.Command { return newSecretsCmd() }

func newSecretsCmd() *cobra.Command {
	var cfg Config
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Cloudbox-managed API keys/tokens for your shell (replaces a plaintext rc file)",
		Long: `secrets fetches your API keys/tokens from cloudbox's encrypted vault
instead of keeping them in a plaintext shell rc file. Put your keys in
cloudbox once (bashy secrets import ~/.novigensrc), then replace the rc
file body with a single line:

  eval "$(bashy secrets env)"

Every new shell pulls the current values over an authenticated, audited,
revocable token; nothing secret stays on disk. 'env' caches the rendered
exports locally and falls back to that cache when cloudbox is unreachable,
so opening a shell never blocks or breaks.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.PersistentFlags().StringVar(&cfg.URL, "url", "", "cloudbox base URL (default $DHNT_BASE_URL minus /v1, else https://ai.dhnt.io)")
	cmd.PersistentFlags().StringVar(&cfg.Token, "token", "", "Bearer token (default ~/.kg/cloudbox-token or $DHNT_API_KEY)")

	cmd.AddCommand(newEnvCmd(&cfg))
	cmd.AddCommand(newLsCmd(&cfg))
	cmd.AddCommand(newGetCmd(&cfg))
	cmd.AddCommand(newSetCmd(&cfg))
	cmd.AddCommand(newImportCmd(&cfg))
	cmd.AddCommand(newRmCmd(&cfg))
	return cmd
}

// --- env ---------------------------------------------------------------

func newEnvCmd(cfg *Config) *cobra.Command {
	var refresh, noCache bool
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Print 'export NAME=VALUE' lines for eval in your shell rc",
		Long: `Print the vault as shell export statements, intended for:

  eval "$(bashy secrets env)"

Results are cached (mode 0600) under $XDG_CACHE_HOME/bashy and reused for
the TTL ($BASHY_SECRETS_TTL, default 1h). If cloudbox is unreachable the
last good cache is printed instead, so 'env' never breaks shell startup —
it always exits 0 and prints a leading comment on degraded paths.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runEnv(c.OutOrStdout(), c.ErrOrStderr(), *cfg, refresh, noCache)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "ignore the cache and fetch fresh")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "do not read or write the cache")
	return cmd
}

func runEnv(out, errOut io.Writer, cfg Config, refresh, noCache bool) error {
	// env must never break shell startup: on any error fall back to cache,
	// and if even that fails emit a harmless comment and exit 0.
	cachePath := cacheFile()
	if !noCache && !refresh {
		if data, ok := freshCache(cachePath, cacheTTL()); ok {
			_, _ = out.Write(data)
			return nil
		}
	}

	client, err := cfg.Resolve()
	if err == nil {
		var items []Item
		items, err = client.List()
		if err == nil {
			rendered := renderEnv(items)
			if !noCache {
				_ = writeCache(cachePath, rendered)
			}
			_, _ = out.Write(rendered)
			return nil
		}
	}

	// Degraded: try any cache regardless of age.
	if !noCache {
		if data, e := os.ReadFile(cachePath); e == nil {
			fmt.Fprintf(errOut, "bashy secrets: cloudbox unreachable (%v); using cached values\n", err)
			fmt.Fprintf(out, "# bashy secrets: served from cache (cloudbox unreachable: %v)\n", err)
			_, _ = out.Write(data)
			return nil
		}
	}
	fmt.Fprintf(errOut, "bashy secrets: %v\n", err)
	fmt.Fprintf(out, "# bashy secrets unavailable: %v\n", err)
	return nil
}

// renderEnv turns secrets into deterministic, safely-quoted export lines.
func renderEnv(items []Item) []byte {
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	var b strings.Builder
	for _, it := range items {
		fmt.Fprintf(&b, "export %s=%s\n", it.Name, shellSingleQuote(it.Value))
	}
	return []byte(b.String())
}

// shellSingleQuote wraps s in single quotes, the only POSIX-safe way to
// quote arbitrary content: 'it'\”s' for an embedded single quote.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// --- ls ----------------------------------------------------------------

func newLsCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List secret names (values are never printed)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			client, err := cfg.Resolve()
			if err != nil {
				return err
			}
			items, err := client.List()
			if err != nil {
				return err
			}
			for _, it := range items {
				fmt.Fprintln(c.OutOrStdout(), it.Name)
			}
			return nil
		},
	}
}

// --- get ---------------------------------------------------------------

func newGetCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "get NAME",
		Short: "Print one secret value (for KEY=$(bashy secrets get NAME))",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			client, err := cfg.Resolve()
			if err != nil {
				return err
			}
			val, err := client.Get(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), val)
			return nil
		},
	}
}

// --- set ---------------------------------------------------------------

func newSetCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set NAME [VALUE]",
		Short: "Set one secret; with no VALUE, read it from stdin (keeps it out of shell history)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			client, err := cfg.Resolve()
			if err != nil {
				return err
			}
			var value string
			if len(args) == 2 {
				value = args[1]
			} else {
				b, rerr := io.ReadAll(c.InOrStdin())
				if rerr != nil {
					return rerr
				}
				value = strings.TrimRight(string(b), "\r\n")
			}
			if err := client.Put([]Item{{Name: args[0], Value: value}}); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "set %s\n", args[0])
			return nil
		},
	}
	return cmd
}

// --- import ------------------------------------------------------------

func newImportCmd(cfg *Config) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "import [FILE]",
		Short: "Import 'export NAME=VALUE' lines from an rc file (default stdin) into the vault",
		Long: `Parse a shell rc file (or stdin) and upsert every 'export NAME=VALUE'
(or bare 'NAME=VALUE') line into the cloudbox vault. Commented (#) and
blank lines are skipped; surrounding single/double quotes are stripped;
values are stored verbatim (no shell expansion). Idempotent — re-running
overwrites in place.`,
		Args: cobra.RangeArgs(0, 1),
		RunE: func(c *cobra.Command, args []string) error {
			var r io.Reader = c.InOrStdin()
			if len(args) == 1 {
				f, err := os.Open(args[0])
				if err != nil {
					return err
				}
				defer f.Close()
				r = f
			}
			items, err := parseEnvFile(r)
			if err != nil {
				return err
			}
			if len(items) == 0 {
				return fmt.Errorf("no NAME=VALUE assignments found")
			}
			if dryRun {
				for _, it := range items {
					fmt.Fprintln(c.OutOrStdout(), it.Name)
				}
				fmt.Fprintf(c.OutOrStdout(), "# %d secret(s) would be imported (dry run)\n", len(items))
				return nil
			}
			client, err := cfg.Resolve()
			if err != nil {
				return err
			}
			if err := client.Put(items); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "imported %d secret(s)\n", len(items))
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the names that would be imported, don't write")
	return cmd
}

// parseEnvFile extracts NAME=VALUE assignments from a shell-style rc file.
func parseEnvFile(r io.Reader) ([]Item, error) {
	var items []Item
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:eq])
		if !validName(name) {
			continue
		}
		val := strings.TrimSpace(line[eq+1:])
		val = stripInlineComment(val)
		val = unquote(val)
		items = append(items, Item{Name: name, Value: val})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// validName accepts a POSIX-ish env var name (letters, digits, underscore;
// not starting with a digit) so we don't try to import malformed lines.
func validName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// stripInlineComment drops a trailing " # ..." comment from an UNQUOTED
// value. Quoted values keep '#' verbatim.
func stripInlineComment(s string) string {
	if strings.HasPrefix(s, "'") || strings.HasPrefix(s, `"`) {
		return s
	}
	if i := strings.Index(s, " #"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// unquote strips a single matching pair of surrounding quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// --- rm ----------------------------------------------------------------

func newRmCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "rm NAME",
		Short: "Delete one secret from the vault",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			client, err := cfg.Resolve()
			if err != nil {
				return err
			}
			if err := client.Delete(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "deleted %s\n", args[0])
			return nil
		},
	}
}

// --- cache -------------------------------------------------------------

func cacheFile() string {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".cache")
	}
	return filepath.Join(dir, "bashy", "secrets-env.sh")
}

func cacheTTL() time.Duration {
	if v := os.Getenv("BASHY_SECRETS_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return time.Hour
}

func freshCache(path string, ttl time.Duration) ([]byte, bool) {
	if path == "" {
		return nil, false
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if fileAge(fi.ModTime()) > ttl {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return data, true
}

// fileAge is split out so tests can reason about it; time.Since uses the
// monotonic clock which is fine for a TTL check.
func fileAge(mod time.Time) time.Duration { return time.Since(mod) }

func writeCache(path string, data []byte) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
