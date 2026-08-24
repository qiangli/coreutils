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

// runtimeConfig is the staged environment under test: the shell NAME the
// profile runs (resolved through the staged PATH, never taken as a host
// path), the staged tool directory its PATH is wired through, and the
// approved multicall executable every multicall-owned name must dispatch to.
type runtimeConfig struct {
	shellName    string // resolved via the RunContext's staged PATH
	binDir       string
	multicall    string // path to the approved multicall executable
	multicallSHA string // optional externally pinned sha256 of that executable
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
// shells. Group 1 is the identity, groups 2/3 the major/minor version, group 4
// the build identifier. Observed forms:
//
//	GNU bash, version 5.2.32(1)-release (x86_64-pc-linux-gnu)   (Profile C)
//	bashy, GNU Bash 5.3 compatible, version 5.3.0(1)-bashy-dev (a0a0315)   (Profile D)
var shellVersionRe = regexp.MustCompile(`^(GNU bash|bashy)\b.* version (\d+)\.(\d+)\S* \(([^()]+)\)\s*$`)

// minShellMajor: both approved profiles run a bash-5-family shell; anything
// older is not the certified configuration.
const minShellMajor = 5

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

	// The interrogated shell is resolved through the staged PATH — never taken
	// as a host path — and must itself live in the staged directory and carry
	// an approved Profile C/D identity/version/build. Probing an unvalidated
	// shell would attribute every later answer to the wrong program, so the
	// shell probes only run once the shell's identity is proven.
	shellPath, fs := resolveShell(rc, cfg)
	out = append(out, fs...)
	if shellPath == "" {
		return out
	}
	if fs := verifyShellIdentity(rc, shellPath); len(fs) != 0 {
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
// is checked against: the sha256 of the approved multicall executable, which
// must exist, be a regular executable file, and — when the caller pinned a
// digest externally — hash to exactly that pin. An empty return means no
// identity could be established, which the caller reports as findings and
// which leaves every per-name identity check unproven (fail-closed: the
// findings are the failure).
func approvedMulticallDigest(cfg runtimeConfig) (string, []Finding) {
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
	if cfg.multicallSHA != "" && !strings.EqualFold(cfg.multicallSHA, sum) {
		return "", []Finding{{Check: "approved-multicall",
			Detail: fmt.Sprintf("%s hashes to sha256 %s, not the approved digest %s",
				cfg.multicall, sum, strings.ToLower(cfg.multicallSHA))}}
	}
	return sum, nil
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

// verifyShellIdentity proves the resolved shell is an approved Profile C/D
// shell: its --version line must identify GNU bash (Profile C) or bashy
// (Profile D), at a bash-5-family version, with a non-empty build identifier.
// An unrecognizable answer is a rejection — an unidentified shell must not be
// allowed to answer the classification and POSIX-mode probes.
func verifyShellIdentity(rc *tool.RunContext, shellPath string) []Finding {
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
	major, err := strconv.Atoi(m[2])
	if err != nil || major < minShellMajor {
		return []Finding{{Check: "shell-identity",
			Detail: fmt.Sprintf("%s is %s %s.%s (build %s); approved profiles require a bash-%d-family shell",
				shellPath, m[1], m[2], m[3], m[4], minShellMajor)}}
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
