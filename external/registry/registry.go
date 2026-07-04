// Package registry is the declarative catalog of self-provisioning managed
// external CLIs — the tier-5/6 client tools bashy fronts (kubectl, helm, doctl,
// and, as they land, aws/azure/gcloud/…). Each entry is DATA describing how to
// provision a tool (a binmgr GitHub/URL spec + metadata: tier, license,
// synopsis); a generic NewCmd turns any entry into a `bashy <tool>` passthrough.
//
// This is the COMPILED-IN BASELINE — always available, offline, standalone
// (bashy assumes no cloudbox). Its Entry shape is intentionally serializable so a
// cloudbox `/tools` overlay (a managed-CLI `kind`) can deserialize into the same
// struct and be merged on top when paired — cloud as an optional overlay, never a
// dependency.
//
// Licensing: entries are DOWNLOADED + exec'd as separate processes (never
// linked), so a non-permissive provider CLI is allowed as a separate program on
// its own license. Prefer permissive; record the License for `bashy doctor`.
package registry

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/binmgr"
)

// Entry describes one managed external CLI, declaratively.
type Entry struct {
	Name     string // the verb / cache key / binary basename
	Tier     int    // execution tier (5 cluster, 6 cloud, …)
	License  string // SPDX id, e.g. "Apache-2.0" (for doctor; download+exec ≠ bundle)
	Synopsis string // one line for `bashy commands`
	Long     string // help body
	// EnvVersion is the env var that pins the release (e.g. "DOCTL_VERSION"); ""
	// means always-latest.
	EnvVersion string
	// Resolve builds the binmgr.Tool for a version ("" = resolve latest). Usually
	// a thin wrapper over binmgr.ResolveGitHub / ResolveURL.
	Resolve func(ctx context.Context, version string) (binmgr.Tool, error)
	// ExecEnv optionally augments the child env (e.g. a DKS kubeconfig default);
	// nil = inherit the parent env.
	ExecEnv func() []string
}

// Ensure provisions the tool (cache-first: an already-downloaded copy is used
// with no network/version-resolution unless a version is pinned) and returns its
// executable path.
func (e Entry) Ensure(ctx context.Context) (string, error) {
	version := ""
	if e.EnvVersion != "" {
		version = strings.TrimSpace(os.Getenv(e.EnvVersion))
	}
	if version == "" {
		if p := binmgr.CachedBinary(e.Name); p != "" {
			return p, nil
		}
	}
	tool, err := e.Resolve(ctx, version)
	if err != nil {
		return "", fmt.Errorf("%s: %w", e.Name, err)
	}
	return binmgr.Ensure(ctx, tool)
}

// NewCmd builds the `bashy <name>` front-door: a transparent passthrough to the
// managed binary. All args pass through unchanged.
func (e Entry) NewCmd() *cobra.Command {
	short := e.Synopsis
	long := e.Long
	if long == "" {
		long = e.Synopsis
	}
	return &cobra.Command{
		Use:                e.Name,
		Short:              short,
		Long:               long,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			bin, err := e.Ensure(cmd.Context())
			if err != nil {
				return err
			}
			c := exec.CommandContext(cmd.Context(), bin, args...)
			if e.ExecEnv != nil {
				c.Env = e.ExecEnv()
			}
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			return c.Run()
		},
	}
}

// entries is the compiled-in baseline, keyed by name. Add a tool = add a row.
var entries = map[string]Entry{}

func register(e Entry) { entries[e.Name] = e }

// Lookup returns the entry for a verb, if registered.
func Lookup(name string) (Entry, bool) {
	e, ok := entries[name]
	return e, ok
}

// Names returns the registered verb names, sorted.
func Names() []string {
	out := make([]string, 0, len(entries))
	for n := range entries {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// All returns every entry, sorted by name.
func All() []Entry {
	out := make([]Entry, 0, len(entries))
	for _, n := range Names() {
		out = append(out, entries[n])
	}
	return out
}
