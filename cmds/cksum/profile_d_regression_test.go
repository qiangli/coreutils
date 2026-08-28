package cksumcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCKSumStopsLegacyAndOptionParsingAtFirstOperand(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"first", "-x", "--algorithm", "--"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, errb, code := runTool(t, dir, "", "first", "-x", "--algorithm", "--")
	if code != 0 || errb != "" {
		t.Fatalf("cksum option-looking operands = (%q, %q, %d)", out, errb, code)
	}
	for _, name := range []string{"first", "-x", "--algorithm", "--"} {
		if !strings.Contains(out, " "+name+"\n") {
			t.Errorf("checksum output missing operand %q: %q", name, out)
		}
	}
}

func TestRewriteLegacyAliasesSkipsValuesAndStopsAtOperand(t *testing.T) {
	got := rewriteLegacyAlgorithmAliases([]string{"--alg", "-r", "-s", "first", "-r"})
	want := []string{"--alg", "-r", "--algorithm=sysv", "first", "-r"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("rewrite = %q, want %q", got, want)
	}
}
