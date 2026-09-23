//go:build unix

package getconfcmd

import (
	"strconv"
	"testing"
)

func TestArgMaxUsesStricterOfHostAndBashy(t *testing.T) {
	hostText, _ := argMaxStr()
	host, err := strconv.Atoi(hostText)
	if err != nil || host <= 0 {
		t.Skipf("host ARG_MAX unavailable: %q", hostText)
	}
	got, stderr, code := runCmd(t, "ARG_MAX")
	want := strconv.Itoa(effectiveArgMax(host))
	if code != 0 || stderr != "" || got != want {
		t.Fatalf("getconf ARG_MAX = (%q, %q, %d), host=%d, want %s", got, stderr, code, host, want)
	}
}
