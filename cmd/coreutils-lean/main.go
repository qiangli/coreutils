// coreutils-lean is the lean multicall binary: the same busybox-style dispatch
// as cmd/coreutils, but linking only the standard Unix utilities registered by
// [github.com/qiangli/coreutils/cmds/lean] instead of the full agent-extended
// userland.
//
// It exists for the hot helper path — the place where a shell script resolves
// every utility through one multicall binary that is mmap'd and demand-paged on
// each invocation. The full cmd/coreutils binary is ~64 MB because it links
// agent extensions (browser, fetch, jq, …) whose dependency trees dominate the
// page footprint; this binary is ~5x smaller, so a cheap command like `true` or
// `expr` starts ~2.5x faster. See cmds/lean for the exact, tested inventory and
// the exclusion rationale.
//
// Anything not in the lean inventory fails closed with exit 2 and the same
// "not a supported command" diagnostic the full binary emits — never a silent
// approximation. To get the full command set, build/use cmd/coreutils instead.
package main

import "github.com/qiangli/coreutils/multicall"

// cmds/lean registers the lean inventory via init().
import _ "github.com/qiangli/coreutils/cmds/lean"

func main() {
	multicall.Main("coreutils")
}
