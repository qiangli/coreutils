package getconfcmd

import "mvdan.cc/sh/v3/execbudget"

// Bashy cannot promise more than either its own launch budget or the host's
// exec limit. Callers still receive the real host value when it is stricter.
func effectiveArgMax(host int) int { return min(host, execbudget.BashyArgMax) }
