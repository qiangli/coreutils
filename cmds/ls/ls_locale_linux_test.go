//go:build linux

package lscmd

import (
	"testing"

	corectype "github.com/qiangli/coreutils/pkg/ctype"
)

func TestLsQuestionMarkUsesGermanSingleByteCType(t *testing.T) {
	dir := t.TempDir()
	name := "\xe4"
	write(t, dir, name, "")
	open := func(localeName string) (ctypeProvider, error) {
		return corectype.Open(localeName)
	}
	out, errOut, code := runLsLocale(t, dir,
		[]string{"LC_COLLATE=C", "LC_CTYPE=de_DE.ISO-8859-1"},
		noLsCollator, open, "-1q", "--sort=none")
	if code != 0 || errOut != "" || out != name+"\n" {
		t.Fatalf("ls -q Latin-1 = (%q, %q, %d), want raw printable byte", out, errOut, code)
	}
}
