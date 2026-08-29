// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package posixproviderscmd wires the pinned POSIX external providers into the
// multicall registry, and ships the one applet allowed to provision them.
//
// # What this fixes
//
// Profile C of the POSIX certification campaign is "GNU Bash + the Bashy Go
// coreutils". Ten POSIX-required commands are not implemented in Go
// (m4, man, ctags, ar, nm, strip, ex, vi, lp, localedef), and until
// this package existed they were absent from
// tool.Names() — so the shell adapter fell through to $PATH and the arm measured
// Ubuntu's binaries while reporting itself as bashy-only. Registering them here
// is what makes the harness's
// sut-wire.sh, which rebuilds /vsc/cushim from the multicall's OWN inventory,
// wire the provider rather than the host binary.
//
// # Two rules that must not be softened
//
//  1. NO SILENT FALLBACK. If a provider is not provisioned, the tool prints why
//     and exits 127. It never looks at $PATH. A fallback would restore exactly
//     the bug this package removes, and would restore it invisibly.
//
//  2. RUNNING NEVER BUILDS. Only `posix-providers build` downloads and
//     compiles. A provider invocation is a cache lookup (pkg/posixprovider),
//     because a build triggered inside a six-hour certification arm would inject
//     network and toolchain variance into measured evidence, and could hang the
//     arm outright.
//
// # Platform gating
//
// The manifest declares platforms per provider (man is linux,darwin only).
// The gate is enforced at RUN time, not at registration time: the name is
// registered on every platform so the multicall owns it everywhere and an
// unsupported host gets a loud "man 2.12.0 is not supported on windows" instead
// of a silent fall-through to whatever $PATH holds. It also keeps the registry
// — and therefore the measured inventory and the pkg/atlas coverage ratchet —
// the same shape on every platform.
//
// # Opt-out
//
// BASHY_POSIX_PROVIDERS=off unregisters all active providers, so plain
// bashy stays standalone-graceful on a machine with no provider cache. The `posix-providers`
// applet itself is always registered: it is how you get out of that state.
package posixproviderscmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/qiangli/coreutils/cmds/posixproviders/internal/ctagsfifo"
	"github.com/qiangli/coreutils/pkg/posixprovider"
	"github.com/qiangli/coreutils/tool"
)

// BuildScriptEnv names an explicit path to the build recipe, for a deployment
// where the coreutils checkout is not on disk.
const BuildScriptEnv = "POSIX_PROVIDER_BUILD"

// buildScriptRelPath is where the recipe lives inside a coreutils checkout.
var buildScriptRelPath = filepath.Join("tools", "posix-providers", "build.sh")

func init() {
	tool.Register(adminTool())
	if !posixprovider.Enabled() {
		return
	}
	for _, e := range posixprovider.DispatchEntries() {
		tool.Register(providerTool(e))
	}
}

// ---------------------------------------------------------------------------
// the active provider tools
// ---------------------------------------------------------------------------

func providerTool(e posixprovider.Entry) *tool.Tool {
	return &tool.Tool{
		Name: e.Command,
		Synopsis: fmt.Sprintf("%s %s (%s) — POSIX external provider, built locally from pinned upstream source.",
			e.Command, e.Version, e.License),
		Usage: fmt.Sprintf(`%s [arguments...]

Arguments are dispatched to the provisioned %s %s. For ctags only, an existing
POSIX FIFO output is generated through a private regular file and then streamed
to the unchanged FIFO; all other invocations retain direct argv passthrough.
Provision it with:  bashy posix-providers build %s`, e.Command, e.Command, e.Version, e.Command),
		Run: func(rc *tool.RunContext, args []string) int { return runProvider(e, rc, args) },
	}
}

// resolverFor builds the cache resolver for this invocation. BASHY_BIN_CACHE
// is read from the RunContext, not the process: the embedding shell owns the
// environment, and its value routinely differs from the process's. Without an
// override, posixprovider derives the cache from the authenticated OS account.
func resolverFor(rc *tool.RunContext) (posixprovider.Resolver, error) {
	return posixprovider.DefaultWithCacheOverride(rc.Getenv(posixprovider.CacheOverrideEnv))
}

func runProvider(e posixprovider.Entry, rc *tool.RunContext, args []string) int {
	rc.ExitSignal = 0
	providerArgs := args
	manKeyword := false
	if e.Command == "man" {
		var bad string
		manKeyword, providerArgs, bad = parseManArgs(args)
		if bad != "" {
			fmt.Fprintf(rc.Err, "man: unknown option %s\n", bad)
			return 2
		}
	}
	if e.Command == "localedef" {
		providerArgs = localedefProviderArgs(args)
	}

	r, err := resolverFor(rc)
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", e.Command, err)
		return 127
	}
	path, err := r.Resolve(e.Command)
	if err != nil {
		// 127 is "command not found": the honest status for a name this shell
		// owns but cannot supply. Never fall through to $PATH.
		fmt.Fprintf(rc.Err, "%s: %v\n", e.Command, err)
		return 127
	}
	if e.Command == "ctags" {
		return ctagsfifo.Run(rc, e.Command, path, args, execProviderFn)
	}
	providerName := e.Command
	if manKeyword {
		// Resolver verification for man includes this sibling's provenance and
		// digest. Derive it from the verified runtime path so a cache remains
		// valid after relocation; never search PATH or use an unverified helper.
		path = filepath.Join(filepath.Dir(path), "apropos")
		providerName = "apropos"
	}
	if attempted, code := execProviderDedicated(rc, providerName, path, providerArgs); attempted {
		return code
	}
	return execProviderFn(rc, providerName, path, providerArgs)
}

// localedefProviderArgs adapts the pinned GNU provider to the POSIX pathname
// meaning of an option-argument named "-". GNU localedef treats -f - and -i -
// as requests for built-in/default input, while POSIX specifies both option
// arguments as pathnames. In particular, a regular file literally named "-"
// in the current directory must be opened. Prefixing it with ./ removes GNU's
// sentinel interpretation without copying input or changing any other
// argument.
func localedefProviderArgs(args []string) []string {
	out := append([]string(nil), args...)
	for i := 0; i < len(out); i++ {
		switch {
		case out[i] == "-f" && i+1 < len(out):
			if out[i+1] == "-" {
				out[i+1] = "./-"
			}
			i++
		case out[i] == "-f-":
			out[i] = "-f./-"
		case out[i] == "-i" && i+1 < len(out):
			if out[i+1] == "-" {
				out[i+1] = "./-"
			}
			i++
		case out[i] == "-i-":
			out[i] = "-i./-"
		}
	}
	return out
}

// parseManArgs keeps the certification-facing provider on the POSIX
// man surface. Upstream man-db accepts many extensions (notably -h); accepting
// one changes a required invalid-option error into successful help output.
// POSIX specifies only -k, plus the Utility Syntax Guidelines' -- terminator.
// The returned argv has -k removed for direct dispatch to the verified apropos
// companion and inserts -- before every operand. That canonical boundary keeps
// extension-rich upstream programs from reinterpreting a post-operand -h/-k.
func parseManArgs(args []string) (keyword bool, providerArgs []string, bad string) {
	providerArgs = make([]string, 1, len(args)+1)
	providerArgs[0] = "--"
	options := true
	for _, arg := range args {
		if !options {
			providerArgs = append(providerArgs, arg)
			continue
		}
		if arg == "--" {
			options = false
			continue
		}
		if arg == "-k" {
			keyword = true
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			return false, nil, arg
		}
		options = false
		providerArgs = append(providerArgs, arg)
	}
	return keyword, providerArgs, ""
}

// execProviderFn is a seam so the exec path can be exercised without a real
// provider. Production value is execProvider.
var execProviderFn = execProvider

// execProvider runs the provider with full argv passthrough and faithful exit
// status.
//
// argv[0] is the plain COMMAND NAME, not the cache path. That is not cosmetic:
// GNU binutils derive their diagnostic prefix from argv[0] (a cert arm diffs
// those strings), and vim decides between vi mode and ex mode by
// looking at argv[0] — an `ex` invoked as the cache path would come up in vi
// mode and hang a scripted edit.
func execProvider(rc *tool.RunContext, name, path string, args []string) int {
	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	c := exec.CommandContext(ctx, path)
	// Overriding Args after CommandContext is the documented way to set argv[0]
	// independently of the executable path.
	c.Args = append([]string{name}, args...)
	c.Dir = rc.Dir
	if rc.Env == nil {
		c.Env = []string{}
	} else {
		c.Env = rc.Env
	}
	c.Stdin, c.Stdout, c.Stderr = rc.In, rc.Out, rc.Err

	if err := c.Start(); err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", name, err)
		return 126
	}
	if err := c.Wait(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			if code := ee.ExitCode(); code >= 0 {
				return code
			}
			// Terminated by a signal. Report the safe 128+N to every caller and
			// record the raw signal so a standalone process boundary
			// (multicall.Main) can re-raise it and inherit the exact wait
			// status, the way an execve-replacing wrapper would.
			if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				sig := int(ws.Signal())
				rc.ExitSignal = sig
				return 128 + sig
			}
			return 1
		}
		fmt.Fprintf(rc.Err, "%s: %v\n", name, err)
		return 126
	}
	return 0
}

// ---------------------------------------------------------------------------
// posix-providers — the provisioning applet
// ---------------------------------------------------------------------------

func adminTool() *tool.Tool {
	active := strings.Join(posixprovider.DispatchNames(), ", ")
	activeCount := len(posixprovider.DispatchNames())
	t := &tool.Tool{
		Name:     "posix-providers",
		Synopsis: fmt.Sprintf("Provision and inspect the %d pinned POSIX external providers.", activeCount),
		Usage: fmt.Sprintf(`posix-providers <subcommand>

  list                 show every pinned provider and whether it is provisioned
  check [all|<cmd>]    verify provisioning + provenance; non-zero if any is unusable
  dispatch-plan        print, one TSV row per active provider, the exact binary
                       THIS invocation would dispatch to: command, version,
                       resolved path, verified built sha256 — the introspection
                       surface posix-gate compares against its own resolution
  build [all|<cmd>]    fetch pinned upstream SOURCE, verify sha256, build locally

build is the ONLY path that downloads or compiles. Running a provider never
does: provisioning is a prepare-time activity, running is a test-time one, and
fusing them would put network and toolchain variance inside measured evidence.

Active external providers (%d): %s

Go-only replacements, never external providers: bc, ed, make, patch, mail, mailx, talk.

Providers are built locally from pinned upstream source; most are copyleft and
lp is Apache-2.0. Their binaries are never redistributed. Set
BASHY_POSIX_PROVIDERS=off to unregister the provider names entirely.`, activeCount, active),
	}
	t.Run = runAdmin
	return t
}

func runAdmin(rc *tool.RunContext, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(rc.Err, "posix-providers: a subcommand is required (list, check, dispatch-plan, build)")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "--help", "-h", "help":
		fmt.Fprintln(rc.Out, adminTool().Usage)
		return 0
	case "list":
		return runList(rc, rest)
	case "check":
		return runCheck(rc, rest)
	case "dispatch-plan":
		return runDispatchPlan(rc, rest)
	case "build":
		return runBuild(rc, rest)
	default:
		fmt.Fprintf(rc.Err, "posix-providers: unknown subcommand %q (list, check, dispatch-plan, build)\n", sub)
		return 2
	}
}

// runDispatchPlan is the trusted-introspection surface: for every active
// provider it prints the binary THIS invocation — this executable, this
// environment, this cache — would dispatch to, as one strict TSV row:
//
//	command<TAB>version<TAB>resolved path<TAB>verified built sha256
//
// posix-gate runs it through the digest-verified approved multicall and
// compares each row against its own independent resolution, which binds
// provider provenance to the staged wrapper's ACTUAL dispatch target rather
// than to a cache the wrapper might never consult. Any provider that cannot
// produce a verified identity is a loud FAIL and a non-zero exit — a plan
// with holes must not read as a plan.
func runDispatchPlan(rc *tool.RunContext, args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(rc.Err, "posix-providers dispatch-plan: takes no arguments")
		return 2
	}
	r, err := resolverFor(rc)
	if err != nil {
		fmt.Fprintf(rc.Err, "posix-providers: %v\n", err)
		return 1
	}
	bad := 0
	for _, e := range posixprovider.DispatchEntries() {
		id, err := r.VerifiedIdentity(e.Command)
		if err != nil {
			fmt.Fprintf(rc.Err, "FAIL %s: %v\n", e.Command, err)
			bad++
			continue
		}
		fmt.Fprintf(rc.Out, "%s\t%s\t%s\t%s\n", id.Command, id.Version, id.Path, id.BuiltSHA256)
	}
	if bad > 0 {
		fmt.Fprintf(rc.Err, "posix-providers: %d provider(s) have no verifiable dispatch target\n", bad)
		return 1
	}
	return 0
}

func runList(rc *tool.RunContext, args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(rc.Err, "posix-providers list: takes no arguments")
		return 2
	}
	r, err := resolverFor(rc)
	if err != nil {
		fmt.Fprintf(rc.Err, "posix-providers: %v\n", err)
		return 1
	}
	fmt.Fprintf(rc.Out, "%-8s %-10s %-9s %-24s %s\n", "COMMAND", "VERSION", "LICENSE", "PLATFORMS", "STATE")
	for _, e := range posixprovider.Entries() {
		st := r.Status(e.Command)
		state := "provisioned"
		switch {
		case !st.Supported:
			state = "unsupported on " + r.GOOS
		case st.Err != nil:
			state = shortReason(st.Err)
		}
		fmt.Fprintf(rc.Out, "%-8s %-10s %-9s %-24s %s\n",
			e.Command, e.Version, e.License, strings.Join(e.Platforms, ","), state)
	}
	return 0
}

func shortReason(err error) string {
	switch {
	case errors.Is(err, posixprovider.ErrNotProvisioned):
		return "not provisioned"
	case errors.Is(err, posixprovider.ErrProvenance):
		return "PROVENANCE MISMATCH"
	default:
		return "unusable"
	}
}

func runCheck(rc *tool.RunContext, args []string) int {
	names, code := selectNames(rc, args, "check")
	if code != 0 {
		return code
	}
	r, err := resolverFor(rc)
	if err != nil {
		fmt.Fprintf(rc.Err, "posix-providers: %v\n", err)
		return 1
	}
	bad := 0
	for _, n := range names {
		st := r.Status(n)
		switch {
		case !st.Supported:
			// Not a failure: the manifest never promised this platform.
			fmt.Fprintf(rc.Out, "SKIP %s (not declared for %s)\n", n, r.GOOS)
		case st.Err != nil:
			fmt.Fprintf(rc.Err, "FAIL %s: %v\n", n, st.Err)
			bad++
		default:
			fmt.Fprintf(rc.Out, "PASS %s %s -> %s\n", n, st.Entry.Version, st.Path)
		}
	}
	if bad > 0 {
		fmt.Fprintf(rc.Err, "posix-providers: %d provider(s) unusable\n", bad)
		return 1
	}
	return 0
}

func runBuild(rc *tool.RunContext, args []string) int {
	names, code := selectNames(rc, args, "build")
	if code != 0 {
		return code
	}
	script, err := findBuildScript(rc)
	if err != nil {
		fmt.Fprintf(rc.Err, "posix-providers: %v\n", err)
		return 1
	}
	cacheRoot, err := buildCacheRoot(rc)
	if err != nil {
		fmt.Fprintf(rc.Err, "posix-providers: %v\n", err)
		return 1
	}

	// Hand the recipe the pins the RESOLVER will check against. The manifest is
	// embedded in the binary, so a stale copy in the checkout cannot make the two
	// disagree. An empty path leaves the recipe on its own default.
	manifest := materializeManifest()
	if manifest != "" {
		defer os.Remove(manifest)
	}

	failed := 0
	for _, n := range names {
		e, _ := posixprovider.Lookup(n)
		if !e.SupportsGOOS(runtime.GOOS) {
			fmt.Fprintf(rc.Out, "SKIP %s (not declared for %s)\n", n, runtime.GOOS)
			continue
		}
		if code := runBuildScript(rc, script, cacheRoot, manifest, n); code != 0 {
			fmt.Fprintf(rc.Err, "posix-providers: build %s failed (exit %d)\n", n, code)
			failed++
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}

// runBuildScript invokes the recipe. Shelling out here is a BUILD STEP, not a
// utility implementation: the "never shell out" rule governs the coreutils
// themselves (cat never execs /bin/cat), and compiling an external provider is
// categorically not that. The recipe keeps the sha256-before-extraction
// property; nothing about it is reimplemented here.
func runBuildScript(rc *tool.RunContext, script, cacheRoot, manifest, name string) int {
	if runtime.GOOS == "windows" {
		fmt.Fprintf(rc.Err, "posix-providers: build needs a POSIX shell; run it under bashy or a unix host\n")
		return 1
	}
	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	c := exec.CommandContext(ctx, script, name)
	c.Dir = rc.Dir
	env := append([]string{}, rc.Env...)
	env = append(env, "BASHY_BIN_CACHE="+cacheRoot)
	if manifest != "" {
		env = append(env, "POSIX_PROVIDER_MANIFEST="+manifest)
	}
	c.Env = env
	c.Stdin, c.Stdout, c.Stderr = rc.In, rc.Out, rc.Err
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			if code := ee.ExitCode(); code >= 0 {
				return code
			}
			return 1
		}
		fmt.Fprintf(rc.Err, "posix-providers: %v\n", err)
		return 1
	}
	return 0
}

// materializeManifest writes the embedded manifest to a temp file so the recipe
// reads the same pins the resolver enforces. On any failure it returns "", which
// leaves the recipe on its own default (the canonical file in a checkout) rather
// than failing a build over a temp-file problem.
func materializeManifest() string {
	f, err := os.CreateTemp("", "posix-provider-manifest-*.tsv")
	if err != nil {
		return ""
	}
	if _, err := f.WriteString(posixprovider.ManifestText()); err != nil {
		f.Close()
		os.Remove(f.Name())
		return ""
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return ""
	}
	return f.Name()
}

func buildCacheRoot(rc *tool.RunContext) (string, error) {
	return posixprovider.CacheRoot(rc.Getenv(posixprovider.CacheOverrideEnv))
}

// selectNames resolves the "all | <cmd> …" operand form.
func selectNames(rc *tool.RunContext, args []string, verb string) ([]string, int) {
	if len(args) == 0 || (len(args) == 1 && args[0] == "all") {
		return posixprovider.Names(), 0
	}
	for _, a := range args {
		if !posixprovider.Has(a) {
			fmt.Fprintf(rc.Err, "posix-providers %s: %q is not a pinned provider (see `posix-providers list`)\n", verb, a)
			return nil, 2
		}
	}
	return args, 0
}

// findBuildScript locates tools/posix-providers/build.sh. The recipe is a file
// in the source tree rather than an embedded blob, so a binary installed away
// from its checkout must be told where it is.
func findBuildScript(rc *tool.RunContext) (string, error) {
	if p := strings.TrimSpace(rc.Getenv(BuildScriptEnv)); p != "" {
		if fileExists(p) {
			return p, nil
		}
		return "", fmt.Errorf("%s=%s does not exist", BuildScriptEnv, p)
	}
	var tried []string
	for _, start := range candidateRoots(rc) {
		if start == "" {
			continue
		}
		if p, ok := walkUpFor(start, buildScriptRelPath); ok {
			return p, nil
		}
		tried = append(tried, start)
	}
	return "", fmt.Errorf("build recipe %s not found (looked upward from %s); "+
		"run from a coreutils checkout or set %s",
		buildScriptRelPath, strings.Join(tried, ", "), BuildScriptEnv)
}

func candidateRoots(rc *tool.RunContext) []string {
	roots := []string{rc.Dir}
	if exe, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(exe))
	}
	return roots
}

func walkUpFor(start, rel string) (string, bool) {
	dir := start
	for {
		candidate := filepath.Join(dir, rel)
		if fileExists(candidate) {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
