package getconfcmd

import (
	"strconv"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/execbudget"
)

func TestArgMaxMatchesShellExecBudget(t *testing.T) {
	out, stderr, code := runCmd(t, "ARG_MAX")
	value, err := strconv.Atoi(strings.TrimSpace(out))
	if code != 0 || stderr != "" || err != nil || value <= 0 || value > execbudget.BashyArgMax {
		t.Fatalf("getconf ARG_MAX = (%q, %q, %d), want positive integer <= %d", out, stderr, code, execbudget.BashyArgMax)
	}
}

func TestEffectiveArgMaxRespectsStricterHost(t *testing.T) {
	for _, tc := range []struct{ host, want int }{
		{execbudget.BashyArgMax / 2, execbudget.BashyArgMax / 2},
		{execbudget.BashyArgMax, execbudget.BashyArgMax},
		{2 * execbudget.BashyArgMax, execbudget.BashyArgMax},
	} {
		if got := effectiveArgMax(tc.host); got != tc.want {
			t.Errorf("effectiveArgMax(%d) = %d, want %d", tc.host, got, tc.want)
		}
	}
}
