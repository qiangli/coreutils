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

  spec                    print the canonical owner inventory and its pinned counts
  registry                verify the live tool registry owns every name as intended
                          (hermetic: no cache, no network, nothing spawned)
  providers               verify every pinned external provider resolves from the
                          cache with its provenance intact
  runtime --shell PATH --bindir DIR [--same-target]
                          verify the staged runtime end to end: registry + providers
                          + PATH ownership of every multicall-owned name + the
                          shell's effective classification of all 116 names + POSIX
                          mode + POSIXLY_CORRECT reaching children and grandchildren

Every check is fail-closed: the gate proves the INTENDED owner — Go applet,
shell builtin/keyword/entry, or pinned provider — is selected for every name,
or it rejects, naming each name and cause. Count drift, duplicate or ambiguous
ownership, a missing provider pin or provenance record, and host PATH fallback
are all rejections. Run the runtime subcommand from INSIDE the staged
environment, so the PATH and POSIXLY_CORRECT it validates are the ones the
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
	counts := map[Owner]int{}
	for _, r := range spec {
		counts[r.Owner]++
		pkg := r.GoPackage
		if pkg == "" {
			pkg = "-"
		}
		fmt.Fprintf(rc.Out, "%-10s %-17s %s\n", r.Command, r.Owner, pkg)
	}
	fmt.Fprintf(rc.Out, "total %d: %d go_applet, %d shell, %d external_provider (pinned %d/%d/%d/%d)\n",
		len(spec), counts[OwnerGoApplet], counts[OwnerShell], counts[OwnerProvider],
		pinTotal, pinGoApplets, pinShell, pinProviders)
	return 0
}

func runRegistry(rc *tool.RunContext, args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(rc.Err, "posix-gate registry: takes no arguments")
		return 2
	}
	return report(rc, "registry", VerifyRegistry(rc.Getenv(posixprovider.OptOutEnv)),
		fmt.Sprintf("%d names owned as intended: %d go applets, %d shell, %d pinned providers",
			pinTotal, pinGoApplets, pinShell, pinProviders))
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

	// The runtime verdict INCLUDES the registry and provider gates: a staged
	// environment whose PATH is right but whose registry is wrong (or whose
	// providers are unattributable) has not selected the intended owners.
	findings := VerifyRegistry(rc.Getenv(posixprovider.OptOutEnv))
	if r, err := resolverFor(rc); err != nil {
		findings = append(findings, Finding{Check: "provider", Detail: err.Error()})
	} else {
		findings = append(findings, VerifyProviders(r)...)
	}
	findings = append(findings, verifyRuntime(rc, spec, cfg)...)

	return report(rc, "runtime", findings,
		fmt.Sprintf("staged runtime selects the intended owner for all %d names (shell %s, bindir %s)",
			pinTotal, cfg.shell, cfg.binDir))
}

func parseRuntimeFlags(rc *tool.RunContext, args []string) (runtimeConfig, int) {
	var cfg runtimeConfig
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
		case "--shell":
			v, ok := take()
			if !ok || v == "" {
				return usage("--shell requires the staged shell's path")
			}
			cfg.shell = v
		case "--bindir":
			v, ok := take()
			if !ok || v == "" {
				return usage("--bindir requires the staged tool directory")
			}
			cfg.binDir = rc.Path(v)
		case "--same-target":
			if eq {
				return usage("--same-target takes no value")
			}
			cfg.sameTarget = true
		default:
			return usage(fmt.Sprintf("unknown option %q", arg))
		}
	}
	if cfg.shell == "" || cfg.binDir == "" {
		return usage("both --shell and --bindir are required")
	}
	return cfg, 0
}

// gateGOOS is the platform the provider manifest is gated against. A seam so
// tests can exercise the full-pass and platform-refusal paths on any host;
// production is always the running GOOS.
var gateGOOS = runtime.GOOS

// resolverFor mirrors cmds/posixproviders: $BASHY_BIN_CACHE is read from the
// RunContext, never the process — the embedding shell owns the environment.
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
