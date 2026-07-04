// Package sphere is the `bashy sphere` front-door for the dhnt SPHERE tier
// (execution tier 4): multi-node, peer-direct pooled p2p inference/compute — the
// layer between a single-node sandbox and an orchestrated cluster.
//
// The sphere data plane (libp2p mesh, model sharding, LLM pool, peer discovery)
// is owned by the outpost mesh agent. bashy is the userland keystone and must
// stay standalone, so this front-door has ZERO build dependency on outpost: it
// RESOLVES the `outpost` binary at runtime and execs it (the same "download/exec,
// never link" discipline as `bashy podman`/`kubectl`). Without outpost there is
// no p2p sphere — so a missing binary is reported clearly, not papered over.
package sphere

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// errHandled signals the caller to exit non-zero without re-printing (the message
// was already emitted cleanly). SilenceErrors keeps cobra from adding "Error:".
var errHandled = errors.New("sphere: handled")

// subVerbs are the outpost subcommands that make up the sphere tier. `bashy
// sphere <v> …` maps straight to `outpost <v> …`.
var subVerbs = map[string]string{
	"mesh":  "libp2p peer-to-peer transport (the data plane)",
	"shard": "intra-LAN model sharding over the mesh",
	"pool":  "this node's LLM-pool participation",
	"peers": "discovery cache, reachability, predictions",
}

// NewSphereCmd builds the `bashy sphere` command: a thin passthrough to the
// outpost mesh agent's sphere subcommands.
func NewSphereCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "sphere",
		Short: "Peer-direct pooled p2p inference/compute — the sphere tier (via the outpost mesh agent)",
		Long: `sphere is dhnt execution tier 4: multi-node, PEER-DIRECT pooled inference and
compute over the libp2p mesh (no control plane — that is the cluster tier). The
data plane is the outpost mesh agent; this is a thin front-door that execs it, so
bashy stays standalone (no build dependency on outpost). Without outpost running
there is no p2p sphere.

Subcommands (pass straight through to outpost):
  bashy sphere mesh  …    libp2p peer-to-peer transport (the data plane)
  bashy sphere shard …    intra-LAN model sharding over the mesh
  bashy sphere pool  …    this node's LLM-pool participation
  bashy sphere peers …    discovery cache, reachability, predictions
  bashy sphere status     quick overview (= outpost peers status)

Set $OUTPOST_BIN to point at a specific outpost binary.`,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true, // we print clean guidance ourselves (no "Error:" prefix)
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
				fmt.Fprint(cmd.OutOrStdout(), cmd.Long, "\n")
				return nil
			}
			sub := args[0]
			rest := args[1:]
			// `status` is a friendly alias for the most useful overview.
			if sub == "status" {
				sub, rest = "peers", append([]string{"status"}, rest...)
			}
			if _, ok := subVerbs[sub]; !ok {
				fmt.Fprintf(cmd.ErrOrStderr(), "sphere: unknown subcommand %q — try: mesh, shard, pool, peers, status\n", sub)
				return errHandled
			}
			bin, err := resolveOutpost()
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), err) // the clean, inviting join guidance
				return errHandled
			}
			c := exec.CommandContext(cmd.Context(), bin, append([]string{sub}, rest...)...)
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			return c.Run()
		},
	}
	return c
}

// ResolveOutpost finds the outpost mesh-agent binary WITHOUT linking it:
// $OUTPOST_BIN, then $PATH, then the usual install spots. Returns ("", false)
// when none is found. Exported so callers (e.g. `bashy doctor`) can report sphere
// readiness with the same resolution logic.
func ResolveOutpost() (string, bool) {
	if p := strings.TrimSpace(os.Getenv("OUTPOST_BIN")); p != "" {
		return p, isExec(p)
	}
	if p, err := exec.LookPath("outpost"); err == nil && p != "" {
		return p, true
	}
	if home, err := os.UserHomeDir(); err == nil {
		for _, rel := range []string{"bin/outpost", ".local/bin/outpost"} {
			if cand := filepath.Join(home, rel); isExec(cand) {
				return cand, true
			}
		}
	}
	return "", false
}

// resolveOutpost wraps ResolveOutpost with the clear "the sphere tier needs
// outpost" error for the passthrough path.
func resolveOutpost() (string, error) {
	if p := strings.TrimSpace(os.Getenv("OUTPOST_BIN")); p != "" && !isExec(p) {
		return "", fmt.Errorf("sphere: $OUTPOST_BIN=%q is not an executable", p)
	}
	if p, ok := ResolveOutpost(); ok {
		return p, nil
	}
	return "", fmt.Errorf("%s", joinLines)
}

// joinLines is the clear, inviting message shown when this machine isn't part of
// a sphere yet: pool inference/compute across the computers you own by joining
// Tessaro (the front door), which pairs this machine via the outpost mesh agent.
var joinLines = strings.Join([]string{
	"sphere: this machine isn't part of a p2p sphere yet.",
	"",
	"The sphere tier pools inference + compute across the computers you already own,",
	"peer-to-peer. Join the mesh through Tessaro — the front door:",
	"",
	"    1. Sign in / sign up:   https://tessaro.sh",
	"    2. Add this machine     (installs the outpost mesh agent + pairs it)",
	"",
	"Then `bashy sphere mesh|shard|pool|peers` drives your mesh.",
	"(Already have outpost installed? Put it on PATH or set $OUTPOST_BIN.)",
}, "\n")

func isExec(p string) bool {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return false
	}
	return fi.Mode()&0o111 != 0
}
