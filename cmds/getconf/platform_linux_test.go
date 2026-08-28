//go:build linux

package getconfcmd

import (
	"strconv"
	"testing"
)

func TestLinuxAdvertisesBundledLocaledef(t *testing.T) {
	want := strconv.FormatInt(posix2Version, 10)
	for _, name := range []string{"_POSIX2_LOCALEDEF", "POSIX2_LOCALEDEF"} {
		got, ok := systemValue(name)
		if !ok || got != want {
			t.Errorf("systemValue(%q) = (%q, %v), want (%q, true)", name, got, ok, want)
		}
	}
}
