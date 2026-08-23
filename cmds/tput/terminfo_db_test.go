package tputcmd

import (
	"os"
	"os/exec"
	"testing"
)

// The capability name tables are index-order transcriptions of a binary
// format: a wrong entry yields a plausible value under the wrong name, which
// no self-consistent test can catch. So when a system database and a reference
// tput are present, compare against them.
//
// This test is a CROSS-CHECK, not a dependency: it skips cleanly wherever the
// database or the reference binary is missing (Windows, a scratch container),
// and every other test in the package drives fixtures built in-process.
func TestAgreesWithSystemTput(t *testing.T) {
	sys, err := exec.LookPath("tput")
	if err != nil {
		t.Skip("no system tput to compare against")
	}
	getenv := func(k string) string { return os.Getenv(k) }

	cases := []struct {
		capName string
		args    []string
	}{
		{"bel", nil},
		{"clear", nil},
		{"el", nil},
		{"ed", nil},
		{"home", nil},
		{"sgr0", nil},
		{"smso", nil},
		{"rmso", nil},
		{"smul", nil},
		{"kcuu1", nil},
		{"kf1", nil},
		{"acsc", nil},
		{"cup", []string{"5", "10"}},
		{"cub", []string{"3"}},
		{"hpa", []string{"12"}},
		{"setaf", []string{"3"}},
		{"setaf", []string{"200"}},
		{"setab", []string{"1"}},
		{"sgr", []string{"0", "0", "0", "0", "0", "1", "0", "0", "0"}},
	}

	compared := 0
	for _, term := range []string{"xterm", "xterm-256color", "vt100", "ansi", "linux", "screen"} {
		e, err := loadEntry(getenv, term)
		if err != nil || e.source == "(built-in)" {
			continue // not installed here; the fallback is not the reference
		}
		for _, c := range cases {
			s, ok := e.strs[c.capName]
			if !ok {
				continue
			}
			got, err := instantiate(s, c.args)
			if err != nil {
				t.Errorf("%s %s %v: %v", term, c.capName, c.args, err)
				continue
			}
			cmd := exec.Command(sys, append([]string{"-T", term, c.capName}, c.args...)...)
			out, err := cmd.Output()
			if err != nil {
				continue // an older reference may not know this capability
			}
			compared++
			if string(out) != got {
				t.Errorf("%s %s %v:\n ours %q\n theirs %q\n entry %q",
					term, c.capName, c.args, got, string(out), s)
			}
		}
	}
	if compared == 0 {
		t.Skip("no terminal type resolved from a system database")
	}
	t.Logf("compared %d capability instantiations against %s", compared, sys)
}
