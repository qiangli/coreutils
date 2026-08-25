// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package posixgatecmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/pkg/posixprovider"
	"github.com/qiangli/coreutils/tool"
)

// Finding is one rejection. An empty findings list is the ONLY passing state;
// every check either produces positive evidence or produces a Finding.
type Finding struct {
	Check  string // which gate rejected (count-drift, ownership, exec-identity, …)
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

// VerifyRegistry is the hermetic ownership gate: the generated projection
// against the live tool registry and the embedded provider manifest. It
// touches no cache, no network, and spawns nothing. optOutValue is the
// observed value of BASHY_POSIX_PROVIDERS — with the opt-out in effect the
// provider names are deliberately unregistered, which a certification runtime
// must treat as a hard failure, not a configuration choice.
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

// verifyInventory rejects count drift — on BOTH axes — and provider-set drift.
// The counts are compared against the hard pins, both directions: a name added
// anywhere without updating the pins is as much a failure as one removed.
func verifyInventory(spec []specRow, providerNames []string) []Finding {
	var out []Finding
	avail := map[Owner]int{}
	effective := map[string]int{}
	specProviders := map[string]bool{}
	for _, r := range spec {
		avail[r.Owner]++
		switch r.Effective {
		case SelShellEntry, SelShellBuiltin, SelShellKeyword:
			effective["shell"]++
		default:
			effective[string(r.Effective)]++
		}
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
	pin("availability go-applet", avail[OwnerGoApplet], pinAvailGoApplets)
	pin("availability shell", avail[OwnerShell], pinAvailShell)
	pin("availability provider", avail[OwnerProvider], pinProviders)
	pin("effective go-applet", effective["go_applet"], pinEffectiveGoApplets)
	pin("effective shell", effective["shell"], pinEffectiveShell)
	pin("effective provider", effective["external_provider"], pinProviders)

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

// runtimeConfig is the staged environment under test: the profile being
// certified, the shell NAME it runs (resolved through the staged PATH, never
// taken as a host path), the staged tool directory its PATH is wired through,
// the approved multicall executable every multicall-owned name must dispatch
// to, and the approved digests from the externally supplied build manifest.
// The digests are MANDATORY and never derived from the staged binaries: the
// staged binary hashing to itself proves nothing.
type runtimeConfig struct {
	profile      string // "C" or "D"
	manifestPath string // the approved build/run manifest (--manifest)
	shellName    string // resolved via the RunContext's staged PATH
	binDir       string
	multicall    string // path to the approved multicall executable
	shellSHA     string // approved staged-shell sha256, from the build manifest
	multicallSHA string // approved multicall sha256, from the build manifest
}

// runShellFn is the probe seam. The default spawns the shell; hermetic tests
// substitute canned transcripts. Spawning here does not breach the no-shell-out
// rule: like env/xargs/watch, this tool's documented purpose IS running the
// operand it is given — the gate interrogates the staged shell, and there is
// nothing else it could interrogate.
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

// shellVersionRe recognizes the version line of the two approved Profile C/D
// shells. Group 1 is the implementation, groups 2/3 the major/minor version,
// group 4 the build identifier. Observed forms:
//
//	GNU bash, version 5.3.8(1)-release (x86_64-pc-linux-gnu)   (Profile C)
//	bashy, GNU Bash 5.3 compatible, version 5.3.0(1)-bashy-dev (a0a0315)   (Profile D)
//
// The line is a CROSS-CHECK only: it is trivially forgeable, so the build
// identity is the manifest digest (verifyShellIdentity checks that first), and
// this parse merely requires the proven binary to also SAY the right thing.
var shellVersionRe = regexp.MustCompile(`^(GNU bash|bashy)\b.* version (\d+)\.(\d+)\S* \(([^()]+)\)\s*$`)

// verifyRuntime checks the staged environment: POSIXLY_CORRECT in this very
// process and in shell children and grandchildren, the approved multicall's
// identity behind every multicall-owned PATH entry, the staged shell's own
// identity/version/build, POSIX mode in the shell, and the shell's effective
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

	// The approved multicall: the gate does not trust directory membership, it
	// proves EXECUTABLE IDENTITY. Digest the approved binary once; every
	// multicall-owned name must dispatch (through any symlinks) to a target
	// with that exact digest, so a staged symlink to an arbitrary host tool
	// can never pass.
	approved, fs := approvedMulticallDigest(cfg)
	out = append(out, fs...)

	// PATH ownership + executable identity: every multicall-owned name (and
	// sh) must resolve, via the environment's own PATH, to an entry inside the
	// staged directory — and the entry's resolved target must BE the approved
	// multicall. Resolving anywhere else IS the host-fallback bug this gate
	// exists to reject; resolving nowhere is a name the runtime cannot supply.
	digests := map[string]string{}
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
		case approved != "" && multicallOwned(r):
			target := resolvePath(p)
			got, ok := digests[target]
			if !ok {
				sum, err := fileSHA256(target)
				if err != nil {
					out = append(out, Finding{Check: "exec-identity", Name: r.Command,
						Detail: fmt.Sprintf("cannot digest dispatch target %s: %v", target, err)})
					continue
				}
				got, digests[target] = sum, sum
			}
			if got != approved {
				out = append(out, Finding{Check: "exec-identity", Name: r.Command,
					Detail: fmt.Sprintf("dispatches to %s (sha256 %s), which is not the approved multicall (sha256 %s)",
						target, got, approved)})
			}
		}
	}

	// Provider provenance, bound to the staged wrapper's ACTUAL dispatch: the
	// cache the staged environment names is verified, and the (digest-proven)
	// approved multicall is then made to disclose, per provider, the exact
	// binary it would dispatch to — the two views must agree identically.
	out = append(out, verifyStagedProviders(rc, cfg, approved)...)

	// The interrogated shell is resolved through the staged PATH — never taken
	// as a host path — and must itself live in the staged directory and carry
	// the approved profile build (digest against the build manifest) plus the
	// matching identity/version/build line. Probing an unvalidated shell would
	// attribute every later answer to the wrong program, so the shell probes
	// only run once the shell's identity is proven.
	shellPath, fs := resolveShell(rc, cfg)
	out = append(out, fs...)
	if shellPath == "" {
		return out
	}
	if fs := verifyShellIdentity(rc, shellPath, cfg); len(fs) != 0 {
		return append(out, fs...)
	}

	// Shell-effective ownership of all 116 names, in one spawn, parsed
	// strictly: the transcript must carry exactly one well-formed row for each
	// of the 116 expected names — duplicates, extras, missing and malformed
	// rows are all rejections, not noise.
	args := []string{"-c", classifyScript, "posix-gate"}
	for _, r := range spec {
		args = append(args, r.Command)
	}
	if classes, err := runShellFn(rc, shellPath, args...); err != nil {
		out = append(out, Finding{Check: "shell-owner", Detail: "classification probe failed: " + err.Error()})
	} else {
		got, fs := parseTranscript(spec, classes)
		out = append(out, fs...)
		for _, r := range spec {
			g, ok := got[r.Command]
			if !ok {
				continue // already rejected as missing by parseTranscript
			}
			if want := expectedShellClass(r); g != want {
				out = append(out, Finding{Check: "shell-owner", Name: r.Command,
					Detail: fmt.Sprintf("shell resolves it as %q, intended owner requires %q", g, want)})
			}
		}
	}

	// The shell must be in POSIX mode, not merely be a POSIX-capable shell.
	if opts, err := runShellFn(rc, shellPath, "-c", "set -o"); err != nil {
		out = append(out, Finding{Check: "posix-mode", Detail: "set -o probe failed: " + err.Error()})
	} else if !posixModeOn(opts) {
		out = append(out, Finding{Check: "posix-mode",
			Detail: fmt.Sprintf("%s does not report `posix on` under `set -o`", shellPath)})
	}

	// POSIXLY_CORRECT must reach a shell child…
	if v, err := runShellFn(rc, shellPath, "-c", `printf '%s\n' "$POSIXLY_CORRECT"`); err != nil {
		out = append(out, Finding{Check: "posixly-correct", Detail: "shell environment probe failed: " + err.Error()})
	} else if strings.TrimSpace(v) == "" {
		out = append(out, Finding{Check: "posixly-correct",
			Detail: "POSIXLY_CORRECT is unset or empty inside the shell"})
	}

	// …and an exec'd grandchild: `env` here is whatever the staged PATH
	// supplies (verified above to be the approved multicall), so this proves
	// the variable crosses a real process boundary, not just shell expansion.
	if dump, err := runShellFn(rc, shellPath, "-c", "exec env"); err != nil {
		out = append(out, Finding{Check: "posixly-correct-child", Detail: "child environment probe failed: " + err.Error()})
	} else if !hasNonEmptyEnvVar(dump, "POSIXLY_CORRECT") {
		out = append(out, Finding{Check: "posixly-correct-child",
			Detail: "POSIXLY_CORRECT is not in the environment of a process exec'd by the shell"})
	}

	return out
}

// approvedMulticallDigest establishes the identity every multicall-owned name
// is checked against: the staged --multicall executable must exist, be a
// regular file, and hash to EXACTLY the digest the approved build manifest
// records. The manifest pin is mandatory — a digest derived from the staged
// binary itself would only prove the binary equals itself, which is no root of
// trust at all. An empty return means no identity could be established, which
// the caller reports as findings and which leaves every per-name identity
// check unproven (fail-closed: the findings are the failure).
func approvedMulticallDigest(cfg runtimeConfig) (string, []Finding) {
	if !sha256Re.MatchString(cfg.multicallSHA) {
		// Unreachable through runRuntime (the manifest gate rejects first);
		// kept so a direct caller cannot run identity checks with no pin.
		return "", []Finding{{Check: "approved-multicall",
			Detail: "no approved multicall sha256 from the build manifest; identity cannot be established"}}
	}
	want := strings.ToLower(cfg.multicallSHA)
	target := resolvePath(cfg.multicall)
	fi, err := os.Stat(target)
	if err != nil || fi.IsDir() {
		return "", []Finding{{Check: "approved-multicall",
			Detail: fmt.Sprintf("approved multicall %s is not a file", cfg.multicall)}}
	}
	sum, err := fileSHA256(target)
	if err != nil {
		return "", []Finding{{Check: "approved-multicall",
			Detail: fmt.Sprintf("cannot digest approved multicall %s: %v", cfg.multicall, err)}}
	}
	if sum != want {
		return "", []Finding{{Check: "approved-multicall",
			Detail: fmt.Sprintf("%s hashes to sha256 %s, not the approved build manifest's %s",
				cfg.multicall, sum, want)}}
	}
	return sum, nil
}

// runMulticallFn is the trusted-introspection seam: it runs the APPROVED
// multicall (already digest-proven against the build manifest) with the staged
// environment. Hermetic tests substitute canned dispatch plans; production
// execs the binary. A separate seam from runShellFn on purpose — the shell
// probe and the multicall probe interrogate different programs, and a test
// must be able to model them independently.
var runMulticallFn = runShellExec

// verifyStagedProviders binds provider provenance to the STAGED wrapper's
// dispatch target, in two mutually checking halves.
//
// First, the cache: the staged wrapper resolves its cache from the environment
// the shell hands it, so the certification claim is only checkable when that
// environment names the cache explicitly. BASHY_BIN_CACHE absent from the
// staged environment is a rejection, not a fall-back to the gate process's own
// default cache — verifying a cache the wrapper may never consult would
// attribute provenance to the wrong binaries. Every pinned provider must then
// resolve from that cache with provenance intact.
//
// Second, the dispatch: a verified cache SITTING THERE proves nothing about
// what the wrapper actually runs. The gate makes the approved multicall
// disclose its own dispatch plan (`posix-providers dispatch-plan`, run with
// the staged environment) and requires the observed resolved executable,
// version, and built digest for every provider to equal the gate's
// independently verified identity for the same name. A valid-but-unused cache
// alongside a wrapper that would dispatch anything else fails here. The probe
// only runs once the multicall's own identity is digest-proven (approved !=
// ""): an unproven binary's answers about itself are worthless, and the
// missing identity is already a rejection.
func verifyStagedProviders(rc *tool.RunContext, cfg runtimeConfig, approved string) []Finding {
	root := strings.TrimSpace(rc.Getenv("BASHY_BIN_CACHE"))
	if root == "" {
		return []Finding{{Check: "provider-cache",
			Detail: "BASHY_BIN_CACHE is not set in the staged environment, so provider provenance cannot be bound to the staged wrapper's dispatch target"}}
	}
	r := posixprovider.Resolver{CacheRoot: root, GOOS: gateGOOS}
	out := VerifyProviders(r)
	if approved == "" {
		return out
	}

	plan, err := runMulticallFn(rc, cfg.multicall, "posix-providers", "dispatch-plan")
	if err != nil {
		return append(out, Finding{Check: "provider-dispatch",
			Detail: "the approved multicall could not disclose its dispatch plan: " + err.Error()})
	}
	rows, fs := parseDispatchPlan(plan)
	out = append(out, fs...)
	for _, e := range posixprovider.Entries() {
		row, ok := rows[e.Command]
		if !ok {
			continue // already rejected as missing by parseDispatchPlan
		}
		id, err := r.VerifiedIdentity(e.Command)
		if err != nil {
			continue // this provider already rejected by VerifyProviders above
		}
		if row.version != id.Version || resolvePath(row.path) != resolvePath(id.Path) ||
			!strings.EqualFold(row.builtSHA, id.BuiltSHA256) {
			out = append(out, Finding{Check: "provider-dispatch", Name: e.Command,
				Detail: fmt.Sprintf("staged wrapper would dispatch %s at %s (built sha256 %s), but the verified cache identity is %s at %s (built sha256 %s)",
					row.version, row.path, row.builtSHA, id.Version, id.Path, id.BuiltSHA256)})
		}
	}
	return out
}

// planRow is one observed dispatch-plan disclosure.
type planRow struct {
	version  string
	path     string
	builtSHA string
}

// parseDispatchPlan strictly parses the wrapper's dispatch-plan transcript:
// exactly one well-formed `command version path built_sha256` TSV row per
// pinned provider, sixteen in total. Duplicates, extras, missing names,
// malformed rows, and digests that are not 64 hex characters are each a
// Finding — a plan the gate cannot fully account for must not certify
// anything.
func parseDispatchPlan(plan string) (map[string]planRow, []Finding) {
	rows := map[string]planRow{}
	var out []Finding
	for i, line := range strings.Split(strings.TrimSuffix(plan, "\n"), "\n") {
		line = strings.TrimRight(line, "\r")
		f := strings.Split(line, "\t")
		switch {
		case len(f) != 4 || f[0] == "" || f[1] == "" || f[2] == "" || !sha256Re.MatchString(f[3]):
			out = append(out, Finding{Check: "provider-dispatch",
				Detail: fmt.Sprintf("malformed dispatch-plan row %d: %q", i+1, line)})
		case !posixprovider.Has(f[0]):
			out = append(out, Finding{Check: "provider-dispatch", Name: f[0],
				Detail: "dispatch-plan row for a name outside the sixteen pinned providers"})
		default:
			if _, dup := rows[f[0]]; dup {
				out = append(out, Finding{Check: "provider-dispatch", Name: f[0],
					Detail: "duplicate dispatch-plan row"})
				continue
			}
			rows[f[0]] = planRow{version: f[1], path: f[2], builtSHA: strings.ToLower(f[3])}
		}
	}
	for _, e := range posixprovider.Entries() {
		if _, ok := rows[e.Command]; !ok {
			out = append(out, Finding{Check: "provider-dispatch", Name: e.Command,
				Detail: "no dispatch-plan row for this provider"})
		}
	}
	if len(rows) != pinProviders {
		out = append(out, Finding{Check: "provider-dispatch",
			Detail: fmt.Sprintf("dispatch plan accounts for %d pinned providers, want exactly %d", len(rows), pinProviders)})
	}
	return rows, out
}

// resolveShell resolves the interrogated shell BY NAME through the
// RunContext's staged PATH and requires the result to live inside the staged
// tool directory. A shell picked up from the host PATH would make every probe
// answer about the wrong program, which is exactly the substitution this gate
// exists to reject.
func resolveShell(rc *tool.RunContext, cfg runtimeConfig) (string, []Finding) {
	p := rc.ResolveCommand(cfg.shellName)
	if p == "" {
		return "", []Finding{{Check: "shell-path", Name: cfg.shellName,
			Detail: "shell is not resolvable on the staged PATH"}}
	}
	if !withinDir(cfg.binDir, p) {
		return "", []Finding{{Check: "shell-path", Name: cfg.shellName,
			Detail: fmt.Sprintf("resolves to %s, outside the staged tool directory (host PATH shell)", p)}}
	}
	return p, nil
}

// verifyShellIdentity proves the resolved shell is THE approved build for the
// profile being certified. The build identity comes first and is a digest, not
// a string: the staged shell's resolved target must hash to exactly the
// shell_sha256 the externally supplied build manifest records — a --version
// line or target triplet is forgeable and is never accepted as identity on its
// own. Only a digest-proven shell is then cross-checked against what it
// reports: the profile's approved implementation (stock GNU Bash for Profile
// C, Bashy for Profile D), exactly version 5.3 (5.2 and 5.4 both reject), and
// a non-empty build identifier. An unidentified or mismatched shell must not
// be allowed to answer the classification and POSIX-mode probes.
func verifyShellIdentity(rc *tool.RunContext, shellPath string, cfg runtimeConfig) []Finding {
	prof, ok := profiles[cfg.profile]
	if !ok || !sha256Re.MatchString(cfg.shellSHA) {
		// Unreachable through runRuntime (the manifest gate rejects first);
		// kept so a direct caller cannot probe a shell with no root of trust.
		return []Finding{{Check: "shell-build",
			Detail: "no approved profile/shell sha256 from the build manifest; shell identity cannot be established"}}
	}
	sum, err := fileSHA256(resolvePath(shellPath))
	if err != nil {
		return []Finding{{Check: "shell-build",
			Detail: fmt.Sprintf("cannot digest staged shell %s: %v", shellPath, err)}}
	}
	if want := strings.ToLower(cfg.shellSHA); sum != want {
		return []Finding{{Check: "shell-build",
			Detail: fmt.Sprintf("%s hashes to sha256 %s, not the approved profile %s shell build %s from the build manifest",
				shellPath, sum, cfg.profile, want)}}
	}

	v, err := runShellFn(rc, shellPath, "--version")
	if err != nil {
		return []Finding{{Check: "shell-identity",
			Detail: "version probe failed: " + err.Error()}}
	}
	first, _, _ := strings.Cut(v, "\n")
	first = strings.TrimRight(first, "\r")
	m := shellVersionRe.FindStringSubmatch(first)
	if m == nil {
		return []Finding{{Check: "shell-identity",
			Detail: fmt.Sprintf("%s reports %q, which does not identify an approved Profile C/D shell (GNU bash or bashy, with version and build)", shellPath, first)}}
	}
	if m[1] != prof.impl {
		return []Finding{{Check: "shell-identity",
			Detail: fmt.Sprintf("%s identifies as %s %s.%s; profile %s requires %s",
				shellPath, m[1], m[2], m[3], cfg.profile, prof.human)}}
	}
	major, errMaj := strconv.Atoi(m[2])
	minor, errMin := strconv.Atoi(m[3])
	if errMaj != nil || errMin != nil || major != approvedShellMajor || minor != approvedShellMinor {
		return []Finding{{Check: "shell-identity",
			Detail: fmt.Sprintf("%s is %s %s.%s (build %s); profile %s requires %s — exactly version %d.%d",
				shellPath, m[1], m[2], m[3], m[4], cfg.profile, prof.human, approvedShellMajor, approvedShellMinor)}}
	}
	if strings.TrimSpace(m[4]) == "" {
		return []Finding{{Check: "shell-identity",
			Detail: fmt.Sprintf("%s (%s %s.%s) reports no build identifier", shellPath, m[1], m[2], m[3])}}
	}
	return nil
}

// transcriptClasses is the closed vocabulary a `type -t` probe can emit (with
// classifyScript's "none" substitution for an unresolvable name). Anything
// else is a malformed row, not a new classification.
var transcriptClasses = map[string]bool{
	"alias": true, "builtin": true, "file": true, "function": true,
	"keyword": true, "none": true,
}

// parseTranscript strictly parses the classification transcript: exactly one
// well-formed `name class` row per expected name, 116 unique names in total.
// Duplicate rows, extra names, malformed rows, and missing names are each a
// Finding — a transcript the gate cannot fully account for must not certify
// anything.
func parseTranscript(spec []specRow, transcript string) (map[string]string, []Finding) {
	expected := map[string]bool{}
	for _, r := range spec {
		expected[r.Command] = true
	}
	got := map[string]string{}
	var out []Finding
	lines := strings.Split(strings.TrimSuffix(transcript, "\n"), "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")
		name, class, ok := strings.Cut(line, " ")
		switch {
		case !ok || name == "" || class == "" || strings.Contains(class, " ") || !transcriptClasses[class]:
			out = append(out, Finding{Check: "transcript",
				Detail: fmt.Sprintf("malformed classification row %d: %q", i+1, line)})
		case !expected[name]:
			out = append(out, Finding{Check: "transcript", Name: name,
				Detail: "classification row for a name outside the 116-name inventory"})
		case got[name] != "":
			out = append(out, Finding{Check: "transcript", Name: name,
				Detail: "duplicate classification row"})
		default:
			got[name] = class
		}
	}
	for _, r := range spec {
		if got[r.Command] == "" {
			out = append(out, Finding{Check: "transcript", Name: r.Command,
				Detail: "no classification row for this name"})
		}
	}
	if len(got) != pinTotal {
		out = append(out, Finding{Check: "transcript",
			Detail: fmt.Sprintf("transcript accounts for %d unique expected names, want exactly %d", len(got), pinTotal)})
	}
	return got, out
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
// multicall elsewhere, and that is the intended layout, not an escape. The
// LINK TARGET's identity is proven separately, by digest, against the
// approved multicall.
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

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
