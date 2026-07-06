// Package actrunner runs Gitea's act_runner (the GitHub-Actions-compatible CI
// runner) as a managed external binary (pkg/binmgr) — the CI executor for the
// dhnt mesh. The binary is downloaded → sha256-verified → cached by binmgr,
// never compiled in. act_runner is the documented CI pick (docs/mesh-wrap-
// stack.md): it registers against loom/Gitea and dials OUT (so it's NAT-friendly
// over the mesh), executing .gitea/workflows/*.yml.
//
// act_runner releases live on dl.gitea.com (NOT GitHub), so this resolves the
// binary via binmgr.URLSpec rather than GitHubSpec. gitea/act_runner is MIT.
// See dhnt/docs/local-p2p-cicd.md.
package actrunner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/binmgr"
)

const (
	// DefaultVersion pins the act_runner release (no "v" prefix — dl.gitea.com
	// paths use the bare number). Override via $ACT_RUNNER_VERSION / --version.
	DefaultVersion = "0.2.13"
	// DefaultLabels maps the `host` runs-on label to the HOST executor, so jobs
	// run directly on the host shell — the runner needs no container runtime of
	// its own; only image-build steps invoke the managed podman.
	DefaultLabels = "host:host"
)

// Spec is the binmgr URL spec act_runner resolves its binary from.
func Spec(version string) binmgr.URLSpec {
	if version == "" {
		version = DefaultVersion
	}
	return binmgr.URLSpec{
		Name:        "act_runner",
		Version:     version,
		URLTemplate: "https://dl.gitea.com/act_runner/{version}/act_runner-{version}-{goos}-{goarch}{ext}",
		// dl.gitea.com publishes a per-file .sha256 sidecar (binmgr's default).
	}
}

// DefaultDataDir is act_runner's work dir (its .runner registration + state):
// $ACT_RUNNER_DIR or ~/.agents/bashy/act_runner.
func DefaultDataDir() string {
	if d := strings.TrimSpace(os.Getenv("ACT_RUNNER_DIR")); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "act_runner")
	}
	return filepath.Join(home, ".agents", "bashy", "act_runner")
}

// RegisterOptions configures a one-shot registration against a Gitea instance.
type RegisterOptions struct {
	Version  string
	DataDir  string
	Instance string // Gitea base URL (e.g. the loopback addr from `mesh dial git`)
	Token    string // runner registration token (minted in Gitea)
	Name     string // runner name (default: hostname)
	Labels   string // executor labels (default DefaultLabels)
}

func (o *RegisterOptions) defaults() {
	if o.Version == "" {
		o.Version = strings.TrimSpace(os.Getenv("ACT_RUNNER_VERSION"))
	}
	if o.DataDir == "" {
		o.DataDir = DefaultDataDir()
	}
	if o.Labels == "" {
		o.Labels = DefaultLabels
	}
	if o.Name == "" {
		if h, err := os.Hostname(); err == nil {
			o.Name = h
		}
	}
}

// Register ensures the binary and runs `act_runner register --no-interactive`,
// writing the .runner config into the data dir. Idempotent only insofar as Gitea
// allows re-registration; callers typically guard on the .runner file existing.
func Register(ctx context.Context, o RegisterOptions) error {
	o.defaults()
	if o.Instance == "" || o.Token == "" {
		return fmt.Errorf("actrunner: register needs --instance and --token")
	}
	if err := os.MkdirAll(o.DataDir, 0o755); err != nil {
		return err
	}
	bin, err := resolve(ctx, o.Version)
	if err != nil {
		return err
	}
	args := []string{
		"register", "--no-interactive",
		"--instance", o.Instance,
		"--token", o.Token,
		"--labels", o.Labels,
	}
	if o.Name != "" {
		args = append(args, "--name", o.Name)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = o.DataDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// Registered reports whether a .runner config already exists in the data dir.
func Registered(dataDir string) bool {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	_, err := os.Stat(filepath.Join(dataDir, ".runner"))
	return err == nil
}

// Daemon runs `act_runner daemon` in the foreground from the data dir, blocking
// until the context is cancelled or SIGINT/SIGTERM arrives. The daemon dials OUT
// to the registered Gitea instance — no inbound port, mesh/NAT-friendly. extraEnv
// (may be nil) is appended to the inherited environment — e.g. OTEL_* so the CI
// runner + the deploy jobs it launches self-export into the caller's telemetry
// plane under their own service.name.
func Daemon(ctx context.Context, version, dataDir string, extraEnv ...string) error {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	if !Registered(dataDir) {
		return fmt.Errorf("actrunner: not registered (no %s/.runner) — run `register` first", dataDir)
	}
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	bin, err := resolve(ctx, version)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin, "daemon")
	cmd.Dir = dataDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	return cmd.Run()
}

func resolve(ctx context.Context, version string) (string, error) {
	tool, err := binmgr.ResolveURL(ctx, Spec(version))
	if err != nil {
		return "", err
	}
	return binmgr.Ensure(ctx, tool)
}

// NewCmd builds the `act-runner` command tree (bashy front-door + any cobra host).
func NewCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "act-runner",
		Short: "Run Gitea act_runner (the mesh CI executor) as a managed external binary",
		Long: `act-runner runs Gitea's act_runner — downloaded, sha256-verified, and cached
by binmgr (not compiled in). Register it against loom/Gitea, then run the daemon;
it dials OUT to Gitea so it works over the mesh behind NAT. See
dhnt/docs/local-p2p-cicd.md.`,
	}

	var ro RegisterOptions
	register := &cobra.Command{
		Use:   "register",
		Short: "Register this runner against a Gitea instance (writes .runner)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return Register(cmd.Context(), ro)
		},
	}
	register.Flags().StringVar(&ro.Version, "version", "", "act_runner version (default: pinned)")
	register.Flags().StringVar(&ro.DataDir, "data", "", "work dir (default ~/.agents/bashy/act_runner)")
	register.Flags().StringVar(&ro.Instance, "instance", "", "Gitea base URL (e.g. from `outpost mesh dial git`)")
	register.Flags().StringVar(&ro.Token, "token", "", "runner registration token (minted in Gitea)")
	register.Flags().StringVar(&ro.Name, "name", "", "runner name (default: hostname)")
	register.Flags().StringVar(&ro.Labels, "labels", DefaultLabels, "executor labels")

	var dVersion, dData string
	daemon := &cobra.Command{
		Use:   "daemon",
		Short: "Run the act_runner daemon (foreground; dials out to Gitea)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return Daemon(cmd.Context(), dVersion, dData)
		},
	}
	daemon.Flags().StringVar(&dVersion, "version", "", "act_runner version (default: pinned)")
	daemon.Flags().StringVar(&dData, "data", "", "work dir (default ~/.agents/bashy/act_runner)")

	var pVersion string
	pathCmd := &cobra.Command{
		Use:   "path",
		Short: "Print the cached act_runner binary path (fetching it if needed)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := resolve(cmd.Context(), pVersion)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t(act_runner %s)\n", p, func() string {
				if pVersion == "" {
					return DefaultVersion
				}
				return pVersion
			}())
			return nil
		},
	}
	pathCmd.Flags().StringVar(&pVersion, "version", "", "act_runner version (default: pinned)")

	root.AddCommand(register, daemon, pathCmd)
	return root
}
