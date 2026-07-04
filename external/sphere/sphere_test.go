package sphere

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveOutpostEnvAndMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exec-bit check is unix-shaped")
	}
	// A non-existent $OUTPOST_BIN is a clear error, not a silent fallthrough.
	t.Setenv("OUTPOST_BIN", filepath.Join(t.TempDir(), "nope"))
	if _, err := resolveOutpost(); err == nil {
		t.Error("expected error for missing $OUTPOST_BIN")
	}

	// A real executable at $OUTPOST_BIN resolves.
	bin := filepath.Join(t.TempDir(), "outpost")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OUTPOST_BIN", bin)
	got, err := resolveOutpost()
	if err != nil || got != bin {
		t.Errorf("resolveOutpost() = %q, %v; want %q", got, err, bin)
	}
}

func TestSubVerbs(t *testing.T) {
	for _, v := range []string{"mesh", "shard", "pool", "peers"} {
		if _, ok := subVerbs[v]; !ok {
			t.Errorf("sphere is missing sub-verb %q", v)
		}
	}
	if _, ok := subVerbs["cluster"]; ok {
		t.Error("cluster is tier 5 — must not be a sphere sub-verb")
	}
}
