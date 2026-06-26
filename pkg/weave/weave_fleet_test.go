package weave

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestFleetRowDetectsMissingTool is the regression for the launch-failure
// class: a tool whose binary does not resolve on PATH MUST be reported not
// found and not available, so an orchestrator preflighting with `weave fleet`
// skips it instead of wasting a 0-second `weave start` (which is exactly the
// failure that motivated existence detection).
func TestFleetRowDetectsMissingTool(t *testing.T) {
	dir := t.TempDir()
	cache := map[string]fleetProbeEntry{}
	r, dirty := fleetRowFor(dir, "weave-bogus-tool-xyzzy-not-installed", time.Now(), true, cache)
	if r.Found {
		t.Fatalf("missing tool reported Found=true")
	}
	if r.Available {
		t.Fatalf("missing tool reported Available=true; an orchestrator would waste a launch on it")
	}
	if r.Probed {
		t.Fatalf("missing tool should not be capability-probed")
	}
	if dirty {
		t.Fatalf("missing tool should not dirty the probe cache")
	}
}

// TestFleetRowFindsInstalledTool checks the positive path: a real executable on
// PATH is Found + Available with its resolved path.
func TestFleetRowFindsInstalledTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("temp-exec + PATH shimming is fiddly on windows; existence logic is shared")
	}
	bindir := t.TempDir()
	tool := "weave-fake-tool"
	exe := filepath.Join(bindir, tool)
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho v1.2.3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bindir+string(os.PathListSeparator)+os.Getenv("PATH"))

	r, _ := fleetRowFor(t.TempDir(), tool, time.Now(), false, map[string]fleetProbeEntry{})
	if !r.Found || !r.Available {
		t.Fatalf("installed tool: Found=%v Available=%v, want both true", r.Found, r.Available)
	}
	if r.Path != exe {
		t.Fatalf("resolved path = %q, want %q", r.Path, exe)
	}
}
