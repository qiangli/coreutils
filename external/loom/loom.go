// Package loom runs Gitea as a managed external binary (pkg/binmgr) — the git
// forge for the dhnt mesh. The Gitea binary is downloaded → sha256-verified →
// cached by binmgr, never compiled in; bashy ("the OS of binaries") launches it
// via `bashy loom`, and outpost exposes it over the mesh. "loom" keeps ycode's
// name for the gitea-backed forge. See dhnt/docs/external-binary-builtins.md.
package loom

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/binmgr"
)

const (
	// DefaultVersion pins the Gitea release loom runs. "" / "latest" resolves the
	// newest release dynamically; pin for reproducibility ($LOOM_GITEA_VERSION
	// or --gitea-version override). go-gitea/gitea is MIT.
	DefaultVersion = "latest"
	DefaultAddr    = "127.0.0.1"
	DefaultPort    = 3000
)

// Spec is the binmgr GitHub spec loom resolves the Gitea binary from.
func Spec(version string) binmgr.GitHubSpec {
	if version == "" {
		version = DefaultVersion
	}
	return binmgr.GitHubSpec{Name: "loom", Repo: "go-gitea/gitea", Version: version}
}

// DefaultDataDir is loom's work-path (Gitea data + config + repos).
func DefaultDataDir() string {
	if d := os.Getenv("LOOM_DATA_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "loom")
	}
	return filepath.Join(home, ".agents", "bashy", "loom")
}

// Options configures Serve.
type Options struct {
	Version string // Gitea release (default DefaultVersion)
	DataDir string // work-path (default DefaultDataDir)
	Addr    string // HTTP bind addr (default 127.0.0.1 — loopback for the mesh)
	Port    int    // HTTP port (default 3000)
	// Actions toggles Gitea's built-in GitHub-Actions-compatible CI. nil = the
	// default (on, unless $LOOM_ACTIONS is "0"/"false"). This is what makes loom
	// a local CI control plane: act_runner registers against it and dials out
	// over the mesh. See dhnt/docs/local-p2p-cicd.md.
	Actions *bool
	Stdout  io.Writer
	Stderr  io.Writer
}

// actionsOn resolves the effective Actions toggle: an explicit Options.Actions
// wins; otherwise on unless $LOOM_ACTIONS is "0"/"false"/"no"/"off".
func (o *Options) actionsOn() bool {
	if o.Actions != nil {
		return *o.Actions
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOOM_ACTIONS"))) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func (o *Options) defaults() {
	if o.Version == "" {
		o.Version = DefaultVersion
	}
	if o.DataDir == "" {
		o.DataDir = DefaultDataDir()
	}
	if o.Addr == "" {
		o.Addr = DefaultAddr
	}
	if o.Port == 0 {
		o.Port = DefaultPort
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
}

// Instance is a running loom (Gitea) server.
type Instance struct {
	URL     string // http://addr:port it serves on
	Addr    string // addr:port (for `mesh service add git <addr>`)
	Version string // resolved Gitea version
	proc    *binmgr.Process
}

// Stop terminates the Gitea process gracefully.
func (i *Instance) Stop() error {
	if i == nil || i.proc == nil {
		return nil
	}
	return i.proc.Stop(10 * time.Second)
}

// Start resolves + launches `gitea web` bound to a loopback port (NON-blocking):
// it returns once the server answers (or errors). The config is seeded on first
// run (INSTALL_LOCK, SQLite, a generated SECRET_KEY) so it comes up ready, not on
// the /install screen. The caller owns the returned Instance's lifecycle — this
// is what outpost's wrap-harness builtin supervises and exposes over the mesh.
func Start(ctx context.Context, o Options) (*Instance, error) {
	o.defaults()
	tool, err := binmgr.ResolveGitHub(ctx, Spec(o.Version))
	if err != nil {
		return nil, fmt.Errorf("loom: resolve gitea: %w", err)
	}
	bin, err := binmgr.Ensure(ctx, tool)
	if err != nil {
		return nil, fmt.Errorf("loom: fetch gitea: %w", err)
	}
	cfg, err := ensureConfig(o.DataDir, o.Addr, o.Port, o.actionsOn())
	if err != nil {
		return nil, err
	}
	addr := fmt.Sprintf("%s:%d", o.Addr, o.Port)
	url := "http://" + addr
	proc, err := binmgr.Launch(ctx, bin, binmgr.RunSpec{
		Args:       []string{"web", "--config", cfg},
		Env:        []string{"GITEA_WORK_DIR=" + o.DataDir},
		Dir:        o.DataDir,
		Stdout:     o.Stdout,
		Stderr:     o.Stderr,
		HealthURL:  url,
		HealthWait: 60 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("loom: start gitea: %w", err)
	}
	return &Instance{URL: url, Addr: addr, Version: tool.Version, proc: proc}, nil
}

// Serve runs loom in the foreground, blocking until the context is cancelled or
// SIGINT/SIGTERM arrives (the `bashy loom serve` path).
func Serve(ctx context.Context, o Options) error {
	o.defaults()
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	inst, err := Start(ctx, o)
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "loom (gitea %s) serving on %s — work dir %s\n", inst.Version, inst.URL, o.DataDir)
	fmt.Fprintln(o.Stdout, "expose it over the mesh:  outpost mesh service add git "+inst.Addr)

	<-ctx.Done()
	fmt.Fprintln(o.Stdout, "loom: shutting down…")
	return inst.Stop()
}

// ensureConfig writes a minimal Gitea app.ini on first run and returns its path.
// On an existing app.ini it reuses it, but ensures the [actions] section matches
// the requested toggle — so an already-initialized data dir lights up (or drops)
// Actions on the next serve without a manual edit.
func ensureConfig(dataDir, addr string, port int, actions bool) (string, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", err
	}
	cfgPath := filepath.Join(dataDir, "app.ini")
	if existing, err := os.ReadFile(cfgPath); err == nil {
		merged := ensureActionsSection(string(existing), actions)
		if merged != string(existing) {
			if err := os.WriteFile(cfgPath, []byte(merged), 0o600); err != nil {
				return "", err
			}
		}
		return cfgPath, nil // already configured — reuse (with actions reconciled)
	}
	secret, err := randomHex(32)
	if err != nil {
		return "", err
	}
	ini := renderConfig(dataDir, addr, port, secret, actions)
	if err := os.WriteFile(cfgPath, []byte(ini), 0o600); err != nil {
		return "", err
	}
	return cfgPath, nil
}

// ensureActionsSection reconciles the [actions] ENABLED key in an existing
// app.ini to want. It rewrites an existing key, or appends the section if absent.
// Minimal INI surgery — Gitea owns the rest of the file, we touch only this key.
func ensureActionsSection(ini string, want bool) string {
	val := "false"
	if want {
		val = "true"
	}
	lines := strings.Split(ini, "\n")
	inActions := false
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			inActions = t == "[actions]"
			continue
		}
		if inActions {
			k := strings.TrimSpace(strings.SplitN(t, "=", 2)[0])
			if strings.EqualFold(k, "ENABLED") {
				lines[i] = "ENABLED = " + val
				return strings.Join(lines, "\n")
			}
		}
	}
	// No [actions] ENABLED found — append a fresh section.
	suffix := "\n[actions]\nENABLED = " + val + "\n"
	if strings.HasSuffix(ini, "\n") {
		return ini + suffix[1:]
	}
	return ini + suffix
}

// renderConfig is the minimal app.ini: INSTALL_LOCK so it boots ready, SQLite so
// there's no external DB, loopback bind so only the mesh reaches it. Gitea
// auto-generates + persists INTERNAL_TOKEN on first run when it's absent.
func renderConfig(dataDir, addr string, port int, secret string, actions bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "APP_NAME = loom\nRUN_MODE = prod\n\n")
	fmt.Fprintf(&b, "[server]\nHTTP_ADDR = %s\nHTTP_PORT = %d\nROOT_URL = http://%s:%d/\nDISABLE_SSH = true\n\n", addr, port, addr, port)
	fmt.Fprintf(&b, "[database]\nDB_TYPE = sqlite3\nPATH = %s\n\n", filepath.Join(dataDir, "gitea.db"))
	fmt.Fprintf(&b, "[repository]\nROOT = %s\n\n", filepath.Join(dataDir, "repositories"))
	fmt.Fprintf(&b, "[security]\nINSTALL_LOCK = true\nSECRET_KEY = %s\n\n", secret)
	fmt.Fprintf(&b, "[service]\nDISABLE_REGISTRATION = true\n\n")
	// [actions] turns loom into a local CI control plane: act_runner registers
	// against it and dials out over the mesh. DEFAULT_ACTIONS_URL = github lets
	// workflows reference `uses: actions/checkout` at setup; a mesh-local mirror
	// is a follow-up (docs/local-p2p-cicd.md).
	enabled := "false"
	if actions {
		enabled = "true"
	}
	fmt.Fprintf(&b, "[actions]\nENABLED = %s\nDEFAULT_ACTIONS_URL = github\n", enabled)
	return b.String()
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// NewLoomCmd builds the `loom` command tree (bashy front-door + any cobra host).
func NewLoomCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "loom",
		Short: "Run Gitea (the mesh git forge) as a managed external binary",
		Long: `loom runs Gitea — downloaded, sha256-verified, and cached by binmgr (not
compiled in). Expose it over the mesh with: outpost mesh service add git <addr>.`,
	}
	var o Options
	var actions bool
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Download (if needed) + run gitea web on a loopback port",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("actions") {
				o.Actions = &actions
			}
			return Serve(cmd.Context(), o)
		},
	}
	serve.Flags().StringVar(&o.Version, "gitea-version", "", "Gitea release tag (default: latest)")
	serve.Flags().StringVar(&o.DataDir, "data", "", "work dir (default ~/.agents/bashy/loom)")
	serve.Flags().StringVar(&o.Addr, "addr", DefaultAddr, "HTTP bind address")
	serve.Flags().IntVar(&o.Port, "port", DefaultPort, "HTTP port")
	serve.Flags().BoolVar(&actions, "actions", true, "enable Gitea Actions (local CI; $LOOM_ACTIONS=0 to disable)")

	pathCmd := &cobra.Command{
		Use:   "path",
		Short: "Print the cached gitea binary path (fetching it if needed)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			tool, err := binmgr.ResolveGitHub(cmd.Context(), Spec(o.Version))
			if err != nil {
				return err
			}
			p, err := binmgr.Ensure(cmd.Context(), tool)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t(gitea %s)\n", p, tool.Version)
			return nil
		},
	}
	pathCmd.Flags().StringVar(&o.Version, "gitea-version", "", "Gitea release tag (default: latest)")

	root.AddCommand(serve, pathCmd)
	return root
}
