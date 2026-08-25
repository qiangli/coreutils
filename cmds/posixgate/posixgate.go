// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package posixgatecmd

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/qiangli/coreutils/pkg/posixprovider"
	"github.com/qiangli/coreutils/tool"
)

func init() {
	tool.Register(gateTool())
}

func gateTool() *tool.Tool {
	t := &tool.Tool{
		Name:     "posix-gate",
		Synopsis: "Fail-closed effective-owner gate for the 116 POSIX-required utility names (Profiles C/D).",
		Usage: `posix-gate <subcommand>

  spec                    print the canonical owner projection with its pinned
                          availability (86/14/16) and effective (78/22/16) splits
  registry                verify the live tool registry owns every name as intended
                          (hermetic: no cache, no network, nothing spawned)
  providers               verify every pinned external provider resolves from the
                          cache with its provenance intact
  runtime --profile C|D --manifest FILE --bindir DIR --multicall PATH [--shell NAME]
                          verify the staged runtime end to end: registry + providers
                          (bound to the staged wrapper's actual dispatch plan) +
                          approved executable identity — against the externally
                          supplied build manifest — behind every multicall-owned
                          name and the staged shell + the shell's effective
                          classification of all 116 names + POSIX mode +
                          POSIXLY_CORRECT reaching children and grandchildren

Every check is fail-closed: the gate proves the INTENDED owner — Go applet,
shell builtin/keyword/entry, or pinned provider — is selected for every name,
or it rejects, naming each name and cause. Count drift on either axis,
duplicate or ambiguous ownership, a missing provider pin or provenance record,
host PATH fallback, and a staged entry that is not the approved multicall are
all rejections. Identity is rooted in --manifest, the approved build/run
manifest (key<TAB>value rows: profile, shell_sha256, multicall_sha256) written
when the approved builds were produced — never in the staged binaries
themselves. Both approved shells report the stock "GNU bash, version 5.3…"
line and differ only in release flavor: Profile C certifies approved stock GNU
Bash 5.3 (-release) and rejects any -bashy- build; Profile D certifies Bashy
5.3, a GNU bash 5.3 build carrying the Bashy-specific -bashy-<revision>
release marker (e.g. 5.3.0(1)-bashy-dev), and rejects stock flavors. 5.2,
5.4, a wrong release flavor, or a manifest for the other profile all reject. --shell takes a command NAME (default sh)
resolved through the staged PATH — a host shell path is a usage error, not an
input. Run the runtime subcommand from INSIDE the staged environment, so the
PATH, BASHY_BIN_CACHE, and POSIXLY_CORRECT it validates are the ones the
runtime actually has.

Exit status: 0 every gate passed, 1 any rejection, 2 usage.`,
	}
	t.Run = runGate
	return t
}

func runGate(rc *tool.RunContext, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(rc.Err, "posix-gate: a subcommand is required (spec, registry, providers, runtime)")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "--help", "-h", "help":
		fmt.Fprintln(rc.Out, gateTool().Usage)
		return 0
	case "spec":
		return runSpec(rc, rest)
	case "registry":
		return runRegistry(rc, rest)
	case "providers":
		return runProviders(rc, rest)
	case "runtime":
		return runRuntime(rc, rest)
	default:
		fmt.Fprintf(rc.Err, "posix-gate: unknown subcommand %q (spec, registry, providers, runtime)\n", sub)
		return 2
	}
}

func runSpec(rc *tool.RunContext, args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(rc.Err, "posix-gate spec: takes no arguments")
		return 2
	}
	spec, err := loadSpec()
	if err != nil {
		fmt.Fprintf(rc.Err, "posix-gate: %v\n", err)
		return 1
	}
	avail := map[Owner]int{}
	effShell := 0
	for _, r := range spec {
		avail[r.Owner]++
		switch r.Effective {
		case SelShellEntry, SelShellBuiltin, SelShellKeyword:
			effShell++
		}
		pkg := r.GoPackage
		if pkg == "" {
			pkg = "-"
		}
		fmt.Fprintf(rc.Out, "%-10s %-17s %-14s %s\n", r.Command, r.Owner, r.Effective, pkg)
	}
	fmt.Fprintf(rc.Out, "total %d: availability %d go_applet, %d shell, %d external_provider (pinned %d/%d/%d)\n",
		len(spec), avail[OwnerGoApplet], avail[OwnerShell], avail[OwnerProvider],
		pinAvailGoApplets, pinAvailShell, pinProviders)
	fmt.Fprintf(rc.Out, "effective selection: %d go_applet, %d shell, %d external_provider (pinned %d/%d/%d)\n",
		len(spec)-effShell-avail[OwnerProvider], effShell, avail[OwnerProvider],
		pinEffectiveGoApplets, pinEffectiveShell, pinProviders)
	return 0
}

func runRegistry(rc *tool.RunContext, args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(rc.Err, "posix-gate registry: takes no arguments")
		return 2
	}
	return report(rc, "registry", VerifyRegistry(rc.Getenv(posixprovider.OptOutEnv)),
		fmt.Sprintf("%d names owned as intended: %d go applets, %d shell, %d pinned providers",
			pinTotal, pinAvailGoApplets, pinAvailShell, pinProviders))
}

func runProviders(rc *tool.RunContext, args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(rc.Err, "posix-gate providers: takes no arguments")
		return 2
	}
	r, err := resolverFor(rc)
	if err != nil {
		fmt.Fprintf(rc.Err, "posix-gate: %v\n", err)
		return 1
	}
	return report(rc, "providers", VerifyProviders(r),
		fmt.Sprintf("%d providers provisioned, provenance verified", pinProviders))
}

func runRuntime(rc *tool.RunContext, args []string) int {
	cfg, code := parseRuntimeFlags(rc, args)
	if code != 0 {
		return code
	}
	spec, err := loadSpec()
	if err != nil {
		fmt.Fprintf(rc.Err, "posix-gate: %v\n", err)
		return 1
	}

	// The externally supplied build manifest is the root of trust for every
	// identity check below. Without a valid one there is nothing meaningful
	// left to verify — running the later gates against unpinned binaries would
	// dress an unrooted measurement up as a partial verdict.
	man, mfs := loadBuildManifest(cfg.manifestPath, cfg.profile)
	if len(mfs) != 0 {
		return report(rc, "runtime", mfs, "")
	}
	cfg.shellSHA, cfg.multicallSHA = man.shellSHA, man.multicallSHA

	// The runtime verdict INCLUDES the registry and provider gates: a staged
	// environment whose PATH is right but whose registry is wrong (or whose
	// providers are unattributable) has not selected the intended owners.
	findings := VerifyRegistry(rc.Getenv(posixprovider.OptOutEnv))
	findings = append(findings, verifyRuntime(rc, spec, cfg)...)

	return report(rc, "runtime", findings,
		fmt.Sprintf("staged profile %s runtime selects the intended owner for all %d names (shell %s, bindir %s, multicall %s)",
			cfg.profile, pinTotal, cfg.shellName, cfg.binDir, cfg.multicall))
}

func parseRuntimeFlags(rc *tool.RunContext, args []string) (runtimeConfig, int) {
	cfg := runtimeConfig{shellName: "sh"}
	usage := func(msg string) (runtimeConfig, int) {
		fmt.Fprintf(rc.Err, "posix-gate runtime: %s\n", msg)
		return cfg, 2
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, val, eq := strings.Cut(arg, "=")
		take := func() (string, bool) {
			if eq {
				return val, true
			}
			if i+1 < len(args) {
				i++
				return args[i], true
			}
			return "", false
		}
		switch name {
		case "--profile":
			v, ok := take()
			if !ok || !profileKnown(v) {
				return usage("--profile requires the profile being certified: C (stock GNU Bash 5.3) or D (Bashy 5.3)")
			}
			cfg.profile = v
		case "--manifest":
			v, ok := take()
			if !ok || v == "" {
				return usage("--manifest requires the approved build/run manifest's path")
			}
			cfg.manifestPath = rc.Path(v)
		case "--shell":
			v, ok := take()
			if !ok || v == "" {
				return usage("--shell requires the staged shell's command name")
			}
			if strings.ContainsAny(v, `/\`) {
				return usage(fmt.Sprintf("--shell takes a command NAME resolved through the staged PATH, not a path (%q)", v))
			}
			cfg.shellName = v
		case "--bindir":
			v, ok := take()
			if !ok || v == "" {
				return usage("--bindir requires the staged tool directory")
			}
			cfg.binDir = rc.Path(v)
		case "--multicall":
			v, ok := take()
			if !ok || v == "" {
				return usage("--multicall requires the approved multicall executable's path")
			}
			cfg.multicall = rc.Path(v)
		default:
			return usage(fmt.Sprintf("unknown option %q", arg))
		}
	}
	if cfg.profile == "" || cfg.manifestPath == "" || cfg.binDir == "" || cfg.multicall == "" {
		return usage("--profile, --manifest, --bindir, and --multicall are all required (identity of the staged executables is rooted in the approved build manifest, never optional)")
	}
	return cfg, 0
}

// gateGOOS is the platform the provider manifest is gated against. A seam so
// tests can exercise the full-pass and platform-refusal paths on any host;
// production is always the running GOOS.
var gateGOOS = runtime.GOOS

// resolverFor mirrors cmds/posixproviders: $BASHY_BIN_CACHE is read from the
// RunContext, never the process — the embedding shell owns the environment.
// Only the standalone `providers` subcommand takes the per-host default cache;
// the runtime gate demands an explicit staged cache (verifyStagedProviders).
func resolverFor(rc *tool.RunContext) (posixprovider.Resolver, error) {
	if root := strings.TrimSpace(rc.Getenv("BASHY_BIN_CACHE")); root != "" {
		return posixprovider.Resolver{CacheRoot: root, GOOS: gateGOOS}, nil
	}
	r, err := posixprovider.Default()
	if err != nil {
		return r, err
	}
	r.GOOS = gateGOOS
	return r, nil
}

// report prints each rejection to stderr and one PASS/FAIL verdict line, so a
// harness log carries both the verdict and every cause.
func report(rc *tool.RunContext, gate string, findings []Finding, passDetail string) int {
	if len(findings) == 0 {
		fmt.Fprintf(rc.Out, "posix-gate %s: PASS (%s)\n", gate, passDetail)
		return 0
	}
	for _, f := range findings {
		fmt.Fprintf(rc.Err, "FAIL %s\n", f)
	}
	fmt.Fprintf(rc.Err, "posix-gate %s: FAIL (%d rejection(s))\n", gate, len(findings))
	return 1
}
