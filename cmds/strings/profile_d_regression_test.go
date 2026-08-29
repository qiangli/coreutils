package stringscmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStringsPOSIXStopsOptionParsingAtFirstOperand(t *testing.T) {
	for _, optionLikeName := range []string{"-n", "--"} {
		t.Run(optionLikeName, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "first"), []byte("abcde\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, optionLikeName), []byte("defgh\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			out, errOut, code := runToolEnv(t, dir, []string{"POSIXLY_CORRECT=1", "LC_ALL=C"}, "", "first", optionLikeName)
			if code != 0 || errOut != "" || out != "abcde\ndefgh\n" {
				t.Fatalf("post-operand %q = (%q, %q, %d), want both pathname results", optionLikeName, out, errOut, code)
			}
		})
	}
}

func TestStringsDefaultModeRetainsPermutedOptions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "input"), []byte("four\nfives\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errOut, code := runToolEnv(t, dir, []string{"LC_ALL=C"}, "", "input", "-n", "5")
	if code != 0 || errOut != "" || out != "fives\n" {
		t.Fatalf("default permuted -n = (%q, %q, %d), want GNU option permutation", out, errOut, code)
	}
}
