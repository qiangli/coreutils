// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package posixgatecmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/qiangli/coreutils/pkg/posixprovider"
	"github.com/qiangli/coreutils/tool"
)

// Finding is one rejection. An empty findings list is the ONLY passing state;
// every check either produces positive evidence or produces a Finding.
type Finding struct {
	Check  string // which gate rejected (count-drift, ownership, path-owner, …)
	Name   string // the command name, empty for inventory-level findings
	Detail string
}

func (f Finding) String() string {
	if f.Name == "" {
		return fmt.Sprintf("[%s] %s", f.Check, f.Detail)
	}
	return fmt.Sprintf("[%s] %s: %s", f.Check, f.Name, f.Detail)
}

// registeredFn answers "does the live tool registry own this name". It is a
// seam so hermetic tests can model registries; production is tool.Lookup over
// whatever the embedding binary registered (cmds/all in the multicall).
var registeredFn = func(name string) bool { return tool.Lookup(name) != nil }

// VerifyRegistry is the hermetic ownership gate: the embedded inventory against
// the live tool registry and the embedded provider manifest. It touches no
// cache, no network, and spawns nothing. optOutValue is the observed value of
// BASHY_POSIX_PROVIDERS — with the opt-out in effect the provider names are
// deliberately unregistered, which a certification runtime must treat as a
// hard failure, not a configuration choice.
func VerifyRegistry(optOutValue string) []Finding {
	spec, err := loadSpec()
	if err != nil {
		return []Finding{{Check: "spec", Detail: err.Error()}}
	}
	var out []Finding
	out = append(out, verifyInventory(spec, posixprovider.Names())...)
	out = append(out, verifyPins(posixprovider.Entries())...)
	out = append(out, verifyOwnership(spec, registeredFn, posixprovider.Has,
		!posixprovider.EnabledIn(optOutValue))...)
	return out
}

// verifyInventory rejects count drift and provider-set drift. The counts are
// compared against the hard pins, both directions: a name added anywhere
// without updating the pins is as much a failure as one removed.
func verifyInventory(spec []specRow, providerNames []string) []Finding {
	var out []Finding
	counts := map[Owner]int{}
	specProviders := map[string]bool{}
	for _, r := range spec {
		counts[r.Owner]++
		if r.Owner == OwnerProvider {
			specProviders[r.Command] = true
		}
	}
	pin := func(what string, got, want int) {
		if got != want {
			out = append(out, Finding{Check: "count-drift",
				Detail: fmt.Sprintf("%s count is %d, pinned at %d", what, got, want)})
		}
	}
	pin("required-name", len(spec), pinTotal)
	pin("go-applet", counts[OwnerGoApplet], pinGoApplets)
	pin("shell", counts[OwnerShell], pinShell)
	pin("provider", counts[OwnerProvider], pinProviders)

	manifest := map[string]bool{}
	for _, n := range providerNames {
		manifest[n] = true
		if !specProviders[n] {
			out = append(out, Finding{Check: "provider-set", Name: n,
				Detail: "pinned in the provider manifest but not an external_provider in the inventory"})
		}
	}
	for n := range specProviders {
		if !manifest[n] {
			out = append(out, Finding{Check: "provider-set", Name: n,
				Detail: "inventory says external_provider but the provider manifest does not pin it"})
		}
	}
	return out
}

// verifyPins asserts every provider row carries a complete pin. The manifest
// parser already refuses an unpinned row; this re-checks independently so a
// softened parser cannot silently un-pin a provider under this gate.
func verifyPins(entries []posixprovider.Entry) []Finding {
	var out []Finding
	for _, e := range entries {
		switch {
		case e.Version == "":
			out = append(out, Finding{Check: "provider-pin", Name: e.Command, Detail: "no pinned version"})
		case len(e.SHA256) != 64:
			out = append(out, Finding{Check: "provider-pin", Name: e.Command, Detail: "no full sha256 source pin"})
		case len(e.Platforms) == 0:
			out = append(out, Finding{Check: "provider-pin", Name: e.Command, Detail: "no declared platforms"})
		case e.URL == "":
			out = append(out, Finding{Check: "provider-pin", Name: e.Command, Detail: "no upstream source URL"})
		}
	}
	return out
}

// verifyOwnership checks each name's intended owner against the registry:
// every go_applet and provider name must be registered (the multicall must OWN
// it, or the harness's PATH wiring silently takes the host's binary), every
// shell name must NOT be (a registered tool under a shell-owned name is
// ambiguous ownership), and no applet may double as a pinned provider.
func verifyOwnership(spec []specRow, registered func(string) bool, providerHas func(string) bool, optOut bool) []Finding {
	var out []Finding
	if optOut {
		out = append(out, Finding{Check: "opt-out",
			Detail: "BASHY_POSIX_PROVIDERS=off: provider names are unregistered, so the runtime cannot own them"})
	}
	for _, r := range spec {
		switch r.Owner {
		case OwnerGoApplet:
			if !registered(r.Command) {
				out = append(out, Finding{Check: "ownership", Name: r.Command,
					Detail: "intended owner is a Go applet, but the name is not in the tool registry (host PATH would supply it)"})
			}
			if providerHas(r.Command) {
				out = append(out, Finding{Check: "ownership", Name: r.Command,
					Detail: "ambiguous: dispositioned as a Go applet but also pinned in the provider manifest"})
			}
		case OwnerProvider:
			if !providerHas(r.Command) {
				out = append(out, Finding{Check: "ownership", Name: r.Command,
					Detail: "intended owner is a pinned provider, but the manifest does not pin it"})
			}
			if !optOut && !registered(r.Command) {
				out = append(out, Finding{Check: "ownership", Name: r.Command,
					Detail: "intended owner is a pinned provider, but the name is not in the tool registry (host PATH would supply it)"})
			}
		case OwnerShell:
			if registered(r.Command) {
				out = append(out, Finding{Check: "ownership", Name: r.Command,
					Detail: "intended owner is the shell, but a registered tool also claims the name (ambiguous ownership)"})
			}
			if providerHas(r.Command) {
				out = append(out, Finding{Check: "ownership", Name: r.Command,
					Detail: "ambiguous: dispositioned as shell-supplied but also pinned in the provider manifest"})
			}
		}
	}
	return out
}

// VerifyProviders is the provisioning/provenance gate: every pinned provider
// must resolve from the cache with its provenance verified. A platform the
// manifest does not declare is a FAILURE here, not a skip — a staged
// certification runtime that cannot supply all sixteen names is not the
// runtime it claims to be. (posix-providers check keeps its softer per-host
// semantics; this gate is the certification view.)
func VerifyProviders(r posixprovider.Resolver) []Finding {
	var out []Finding
	for _, e := range posixprovider.Entries() {
		st := r.Status(e.Command)
		switch {
		case !st.Supported:
			out = append(out, Finding{Check: "provider", Name: e.Command,
				Detail: fmt.Sprintf("%s %s is not declared for %s (manifest platforms: %s)",
					e.Command, e.Version, r.GOOS, strings.Join(e.Platforms, ","))})
		case st.Err != nil:
			out = append(out, Finding{Check: "provider", Name: e.Command, Detail: st.Err.Error()})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// runtime gate — the staged Profile C/D environment
// ---------------------------------------------------------------------------

// runtimeConfig is the staged environment under test: the shell the profile
// runs, and the staged tool directory its PATH is wired through.
type runtimeConfig struct {
	shell      string
	binDir     string
	sameTarget bool
}

// runShellFn is the probe seam. The default spawns the shell; hermetic tests
// substitute canned transcripts. Spawning here does not breach the no-shell-out
// rule: like env/xargs/watch, this tool's documented purpose IS running the
// operand it is given — the gate interrogates the shell named by --shell, and
// there is nothing else it could interrogate.
var runShellFn = runShellExec

func runShellExec(rc *tool.RunContext, shell string, args ...string) (string, error) {
	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	c := exec.CommandContext(ctx, shell, args...)
	c.Dir = rc.Dir
	if rc.Env == nil {
		c.Env = []string{}
	} else {
		c.Env = rc.Env
	}
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	if err := c.Run(); err != nil {
		return out.String(), fmt.Errorf("%s %s: %v (stderr: %s)",
			shell, strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// classifyScript asks the shell, in one spawn, how it resolves every name.
// `type -t` is a bash-family builtin and both target shells (GNU bash in
// Profile C, bashy in Profile D) are bash drop-ins; a shell that cannot answer
// fails the probe loudly, which is the fail-closed direction.
const classifyScript = `for n in "$@"; do t=$(type -t -- "$n" 2>/dev/null) || t=none; [ -n "$t" ] || t=none; printf '%s %s\n' "$n" "$t"; done`

// verifyRuntime checks the staged environment: POSIXLY_CORRECT in this very
// process and in shell children and grandchildren, POSIX mode in the shell,
// PATH ownership of every multicall-owned name, and the shell's effective
// classification of all 116 names.
func verifyRuntime(rc *tool.RunContext, spec []specRow, cfg runtimeConfig) []Finding {
	var out []Finding

	// The gate itself must be running inside the environment it certifies.
	// Presence-and-non-empty is the contract everywhere: POSIX utilities (GNU's
	// and this repo's alike) test whether POSIXLY_CORRECT is SET, and bash
	// rewrites the exported value to "y" on entering posix mode, so demanding
	// the literal "1" would reject a perfectly staged runtime.
	if rc.Getenv("POSIXLY_CORRECT") == "" {
		out = append(out, Finding{Check: "posixly-correct",
			Detail: "POSIXLY_CORRECT is not set in the gate's own environment"})
	}

	if fi, err := os.Stat(cfg.binDir); err != nil || !fi.IsDir() {
		out = append(out, Finding{Check: "bindir",
			Detail: fmt.Sprintf("staged tool directory %s is not a directory", cfg.binDir)})
		return out // every later check would just repeat this
	}

	// PATH ownership: every multicall-owned name (and sh) must resolve, via
	// the environment's own PATH, to an entry inside the staged directory.
	// Resolving anywhere else IS the host-fallback bug this gate exists to
	// reject; resolving nowhere is a name the runtime cannot supply.
	targets := map[string][]string{}
	for _, r := range spec {
		if !pathOwned(r) {
			continue
		}
		p := rc.ResolveCommand(r.Command)
		switch {
		case p == "":
			out = append(out, Finding{Check: "path-owner", Name: r.Command,
				Detail: "not resolvable on the staged PATH"})
		case !withinDir(cfg.binDir, p):
			out = append(out, Finding{Check: "path-owner", Name: r.Command,
				Detail: fmt.Sprintf("resolves to %s, outside the staged tool directory (host PATH fallback)", p)})
		case cfg.sameTarget && multicallOwned(r):
			t := p
			if resolved, err := filepath.EvalSymlinks(p); err == nil {
				t = resolved
			}
			targets[t] = append(targets[t], r.Command)
		}
	}
	if cfg.sameTarget && len(targets) > 1 {
		var parts []string
		for t, names := range targets {
			sort.Strings(names)
			parts = append(parts, fmt.Sprintf("%s <- %s", t, strings.Join(names, ",")))
		}
		sort.Strings(parts)
		out = append(out, Finding{Check: "path-target",
			Detail: "multicall-owned names resolve to more than one executable: " + strings.Join(parts, "; ")})
	}

	// Shell-effective ownership of all 116 names, in one spawn.
	args := []string{"-c", classifyScript, "posix-gate"}
	for _, r := range spec {
		args = append(args, r.Command)
	}
	if classes, err := runShellFn(rc, cfg.shell, args...); err != nil {
		out = append(out, Finding{Check: "shell-owner", Detail: "classification probe failed: " + err.Error()})
	} else {
		got := map[string]string{}
		for line := range strings.SplitSeq(classes, "\n") {
			if name, class, ok := strings.Cut(strings.TrimRight(line, "\r"), " "); ok {
				got[name] = class
			}
		}
		for _, r := range spec {
			want := expectedShellClass(r)
			switch g := got[r.Command]; g {
			case want:
			case "":
				out = append(out, Finding{Check: "shell-owner", Name: r.Command,
					Detail: "the shell reported no classification for this name"})
			default:
				out = append(out, Finding{Check: "shell-owner", Name: r.Command,
					Detail: fmt.Sprintf("shell resolves it as %q, intended owner requires %q", g, want)})
			}
		}
	}

	// The shell must be in POSIX mode, not merely be a POSIX-capable shell.
	if opts, err := runShellFn(rc, cfg.shell, "-c", "set -o"); err != nil {
		out = append(out, Finding{Check: "posix-mode", Detail: "set -o probe failed: " + err.Error()})
	} else if !posixModeOn(opts) {
		out = append(out, Finding{Check: "posix-mode",
			Detail: fmt.Sprintf("%s does not report `posix on` under `set -o`", cfg.shell)})
	}

	// POSIXLY_CORRECT must reach a shell child…
	if v, err := runShellFn(rc, cfg.shell, "-c", `printf '%s\n' "$POSIXLY_CORRECT"`); err != nil {
		out = append(out, Finding{Check: "posixly-correct", Detail: "shell environment probe failed: " + err.Error()})
	} else if strings.TrimSpace(v) == "" {
		out = append(out, Finding{Check: "posixly-correct",
			Detail: "POSIXLY_CORRECT is unset or empty inside the shell"})
	}

	// …and an exec'd grandchild: `env` here is whatever the staged PATH
	// supplies (verified above to be the staged applet), so this proves the
	// variable crosses a real process boundary, not just shell expansion.
	if dump, err := runShellFn(rc, cfg.shell, "-c", "exec env"); err != nil {
		out = append(out, Finding{Check: "posixly-correct-child", Detail: "child environment probe failed: " + err.Error()})
	} else if !hasNonEmptyEnvVar(dump, "POSIXLY_CORRECT") {
		out = append(out, Finding{Check: "posixly-correct-child",
			Detail: "POSIXLY_CORRECT is not in the environment of a process exec'd by the shell"})
	}

	return out
}

// posixModeOn scans `set -o` output for the posix option being on.
func posixModeOn(out string) bool {
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "posix" && f[len(f)-1] == "on" {
			return true
		}
	}
	return false
}

func hasNonEmptyEnvVar(dump, name string) bool {
	for line := range strings.SplitSeq(dump, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), name+"="); ok && v != "" {
			return true
		}
	}
	return false
}

// withinDir reports whether file sits inside dir. Only the DIRECTORY parts are
// resolved through symlinks (macOS /var -> /private/var must compare equal),
// never the file itself — a staged entry is routinely a symlink to the
// multicall elsewhere, and that is the intended layout, not an escape.
func withinDir(dir, file string) bool {
	d := resolvePath(dir)
	fd := resolvePath(filepath.Dir(file))
	rel, err := filepath.Rel(d, fd)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func resolvePath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
