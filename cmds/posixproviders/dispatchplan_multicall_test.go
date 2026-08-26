// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Built-multicall integration test for the dispatch-plan introspection
// surface. posix-gate trusts `posix-providers dispatch-plan` run through the
// digest-proven approved multicall BINARY, not through an in-process Tool.Run
// — so this test builds the real cmd/coreutils multicall and interrogates it
// as a subprocess, the exact shape the gate's probe takes. Hermetic: the
// provider cache is a provisioned temp directory, nothing downloads, and the
// only toolchain requirement is the `go` command the test run itself needed.
package posixproviderscmd

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/posixprovider"
)

// buildMulticall compiles cmd/coreutils into a temp dir and returns the
// binary's path. Building is the expensive step, so callers share one build.
func buildMulticall(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("short mode: skipping the built-multicall subprocess test")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("no go toolchain on PATH: %v", err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root not found at %s: %v", root, err)
	}
	bin := filepath.Join(t.TempDir(), "coreutils")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command(goTool, "build", "-o", bin, "./cmd/coreutils")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/coreutils: %v\n%s", err, out)
	}
	return bin
}

// runDispatchPlanSubprocess runs `<multicall> posix-providers dispatch-plan`
// against cacheRoot, with the two provider-relevant environment variables
// pinned so the host's own cache and opt-out cannot leak in.
func runDispatchPlanSubprocess(t *testing.T, bin, cacheRoot string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, "posix-providers", "dispatch-plan")
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "BASHY_BIN_CACHE=") || strings.HasPrefix(kv, posixprovider.OptOutEnv+"=") {
			continue
		}
		cmd.Env = append(cmd.Env, kv)
	}
	cmd.Env = append(cmd.Env, "BASHY_BIN_CACHE="+cacheRoot)
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("running %s: %v", bin, err)
		}
		code = ee.ExitCode()
	}
	return code, out.String(), errb.String()
}

// TestDispatchPlanThroughBuiltMulticall pins the introspection surface end to
// end, against THIS host's manifest view: a provisioned cache yields exactly
// one strict TSV row per provider declared for runtime.GOOS, each matching
// the identity an independent in-process resolver verifies, and every
// undeclared provider is a loud FAIL (exit 1) — never a silently shorter
// plan. A tampered binary and an empty cache each fail loudly too. This is
// the gate's provider-dispatch-binding probe run for real.
func TestDispatchPlanThroughBuiltMulticall(t *testing.T) {
	bin := buildMulticall(t)

	root := t.TempDir()
	bodies := map[string]string{}
	declared := map[string]bool{}
	for _, e := range posixprovider.DispatchEntries() {
		body := "#!/bin/sh\n# provisioned " + e.Command + "\nexit 0\n"
		provision(t, root, e.Command, body)
		bodies[e.Command] = body
		declared[e.Command] = e.SupportsGOOS(runtime.GOOS)
	}

	// Fully provisioned cache: one verified row per declared provider,
	// FAIL + exit 1 for any provider the manifest does not declare here.
	wantCode, wantRows := 0, 0
	for _, ok := range declared {
		if ok {
			wantRows++
		} else {
			wantCode = 1
		}
	}
	code, stdout, stderr := runDispatchPlanSubprocess(t, bin, root)
	if code != wantCode {
		t.Fatalf("provisioned plan: exit = %d, want %d; stderr = %q", code, wantCode, stderr)
	}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if stdout == "" {
		lines = nil
	}
	if len(lines) != wantRows {
		t.Fatalf("plan has %d rows, want %d:\n%s", len(lines), wantRows, stdout)
	}
	r := posixprovider.Resolver{CacheRoot: root, GOOS: runtime.GOOS}
	seen := map[string]bool{}
	for _, line := range lines {
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			t.Fatalf("malformed plan row %q", line)
		}
		if seen[f[0]] {
			t.Fatalf("duplicate plan row for %s", f[0])
		}
		seen[f[0]] = true
		if !declared[f[0]] {
			t.Fatalf("plan row for %s, which the manifest does not declare for %s", f[0], runtime.GOOS)
		}
		id, err := r.VerifiedIdentity(f[0])
		if err != nil {
			t.Fatalf("independent resolution of %s: %v", f[0], err)
		}
		sum := sha256.Sum256([]byte(bodies[f[0]]))
		if f[1] != id.Version || f[2] != id.Path || f[3] != id.BuiltSHA256 || f[3] != hex.EncodeToString(sum[:]) {
			t.Errorf("row for %s = %q, want version %s, path %s, sha %s",
				f[0], line, id.Version, id.Path, id.BuiltSHA256)
		}
	}
	for name, ok := range declared {
		if !ok && !strings.Contains(stderr, "FAIL "+name) {
			t.Errorf("undeclared provider %s not FAILed on stderr: %q", name, stderr)
		}
	}

	// A tampered binary no longer matches its provenance: the plan must FAIL
	// that provider, disclose no row for it, and exit non-zero. make is
	// declared for every platform, so this leg runs everywhere.
	if !declared["make"] {
		t.Fatalf("manifest no longer declares make for %s; pick another tamper target", runtime.GOOS)
	}
	e, _ := posixprovider.Lookup("make")
	tampered := filepath.Join(root, e.Command, e.Version, e.Command)
	if err := os.WriteFile(tampered, []byte("#!/bin/sh\n# tampered\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = runDispatchPlanSubprocess(t, bin, root)
	if code != 1 || !strings.Contains(stderr, "FAIL make") ||
		!strings.Contains(stderr, "no verifiable dispatch target") {
		t.Errorf("tampered make: exit = %d, stderr = %q", code, stderr)
	}
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "make\t") {
			t.Errorf("tampered make still disclosed as a dispatch target: %q", line)
		}
	}

	// An empty cache is fourteen loud failures, never an empty plan.
	code, stdout, stderr = runDispatchPlanSubprocess(t, bin, t.TempDir())
	if code != 1 || strings.TrimSpace(stdout) != "" ||
		!strings.Contains(stderr, "no verifiable dispatch target") {
		t.Errorf("empty cache: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}
