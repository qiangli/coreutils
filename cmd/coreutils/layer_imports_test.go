package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// allowlist records packages from the banned set that are still transitively
// imported today.
//
// This is a shrink-only allowlist: every entry in this map MUST still be
// imported by the cmd/coreutils binary. If an entry is no longer imported, the
// test fails so that the entry is promptly removed from this allowlist.
var allowlist = map[string]string{
	// Removing story: S4 6e4bb2e13140
	"mvdan.cc/sh/v3/polyglot": "S4 6e4bb2e13140",
}

// isBanned returns whether pkg matches any of the layer-boundary banned package
// rules:
//   - github.com/qiangli/yoke/...
//   - bashsharp
//   - mvdan.cc/sh/v3/{polyglot,gosource,lower,grammar}
func isBanned(pkg string) (string, bool) {
	if pkg == "github.com/qiangli/yoke" || strings.HasPrefix(pkg, "github.com/qiangli/yoke/") {
		return "github.com/qiangli/yoke/...", true
	}
	if strings.Contains(pkg, "bashsharp") {
		return "bashsharp", true
	}
	for _, prefix := range []string{
		"mvdan.cc/sh/v3/polyglot",
		"mvdan.cc/sh/v3/gosource",
		"mvdan.cc/sh/v3/lower",
		"mvdan.cc/sh/v3/grammar",
	} {
		if pkg == prefix || strings.HasPrefix(pkg, prefix+"/") {
			return "mvdan.cc/sh/v3/{polyglot,gosource,lower,grammar}", true
		}
	}
	return "", false
}

// TestLayerImports verifies that the cmd/coreutils binary does not transitively
// import yoke, bashsharp, or banned mvdan.cc/sh packages.
func TestLayerImports(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("skipping test: go toolchain not found on PATH: %v", err)
	}

	cmd := exec.Command(goTool, "list", "-deps")
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GOWORK=") {
			env = append(env, e)
		}
	}
	cmd.Env = append(env, "GOWORK=off")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps failed: %v\nOutput:\n%s", err, string(out))
	}

	imported := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		pkg := strings.TrimSpace(line)
		if pkg != "" {
			imported[pkg] = true
		}
	}

	// Shrink-only ratchet: every allowlisted entry must still be imported.
	for allowedPkg := range allowlist {
		if _, banned := isBanned(allowedPkg); !banned {
			t.Errorf("allowlist entry %q is not in the banned set", allowedPkg)
		}
		if !imported[allowedPkg] {
			t.Errorf("shrink-only allowlist violation: %q is no longer imported; remove it from allowlist", allowedPkg)
		}
	}

	// Enforce that no banned package is imported unless explicitly allowlisted.
	for pkg := range imported {
		rule, banned := isBanned(pkg)
		if !banned {
			continue
		}
		if _, allowed := allowlist[pkg]; !allowed {
			t.Errorf("banned transitive dependency imported: %q (violates %s)", pkg, rule)
		}
	}
}
