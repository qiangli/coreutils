// Package perfbenchcmd is the DEV-ONLY measurement harness for the
// bashy-vs-GNU head-to-head: a cmdperf/hyperfine-style A/B timing runner
// (mode "run") plus a byte-identical GNU-fidelity differ (mode "conformance")
// plus a deterministic corpus generator (mode "gen"), over the complete
// coreutils 9.11 inventory (inventory.go).
//
// It is NOT part of the shipped userland:
//
//   - It is the ONE place os/exec is used outside the blessed set — it must
//     shell out to the real GNU binaries as the reference arm. That is legitimate
//     for a measurement instrument, but it means this package is DEV-ONLY and is
//     deliberately kept OUT of cmds/all, so the bare `coreutils` multicall binary
//     and the no-shell-out contract stay clean (same placement rule as cmds/graph).
//     A tiny dev host (cmd/perfbench, or bashy in the bench container) blank-imports
//     cmds/all + this package so the in-process arm can reach every tool.
//
// See docs/coreutils-fidelity-perf-harness-spec.md for the full design and the
// head-to-head run plan.
package perfbenchcmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"
)

func init() {
	tool.Register(&tool.Tool{
		Name:     "perfbench",
		Synopsis: "dev-only bashy-vs-GNU perf + fidelity measurement harness",
		Usage: `perfbench <mode> [flags]

modes:
  run           time workloads across arms (gnu, bashy-cold, bashy-inproc, bashy-warm)
  conformance   diff each command's output byte-for-byte against the GNU arm
  gen           generate the deterministic benchmark corpus
  list          print the complete coreutils 9.11 inventory + present/missing status

Reference arms are resolved from the environment (set by bench.Containerfile):
  GNU_PREFIX   dir holding the GNU coreutils 9.11 + bash 5.3 binaries (default /opt/gnu)
  BASHY_BIN    the bashy binary for the cold/warm arms (default: bashy on PATH)`,
		Run: run,
	})
}

func run(rc *tool.RunContext, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(rc.Err, "perfbench: missing mode (run|conformance|gen|list)")
		return 2
	}
	switch args[0] {
	case "run":
		return runPerf(rc, args[1:])
	case "conformance":
		return runConformance(rc, args[1:])
	case "gen":
		return runGen(rc, args[1:])
	case "list":
		return runList(rc, args[1:])
	default:
		fmt.Fprintf(rc.Err, "perfbench: unknown mode %q\n", args[0])
		return 2
	}
}

// runList prints the complete inventory grouped, with per-command status and a
// per-group tally — the "is coreutils complete" view.
func runList(rc *tool.RunContext, _ []string) int {
	groups := []Group{FileUtils, ShUtils, TextUtils}
	byGroup := ByGroup()
	var totPresent, tot int
	for _, g := range groups {
		var present, planned, delib int
		fmt.Fprintf(rc.Out, "== %s ==\n", g)
		for _, p := range byGroup[g] {
			st := StatusOf(p)
			mark := map[Status]string{StatusPresent: "✓", StatusPlanned: "○", StatusDeliberate: "✗"}[st]
			hist := ""
			if !p.Historical {
				hist = " (modern)"
			}
			fmt.Fprintf(rc.Out, "  %s %-10s%s\n", mark, p.Name, hist)
			switch st {
			case StatusPresent:
				present++
			case StatusPlanned:
				planned++
			default:
				delib++
			}
		}
		fmt.Fprintf(rc.Out, "  -- %s: %d present · %d planned · %d deliberate (of %d)\n\n",
			g, present, planned, delib, len(byGroup[g]))
		totPresent += present
		tot += len(byGroup[g])
	}
	fmt.Fprintf(rc.Out, "TOTAL: %d of %d GNU coreutils 9.11 programs present (✓)\n", totPresent, tot)
	return 0
}
