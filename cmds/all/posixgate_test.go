// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// This is the shipped-inventory half of the POSIX owner gate: cmds/posixgate's
// own tests exercise the checks against synthetic registries (its package
// cannot import cmds/all without a cycle), and THIS test runs the same gate
// against the real assembled registry. A registered name lost from cmds/all, a
// shell-owned name gaining a registered tool, or provider/manifest drift fails
// here at `go test` time — before any certification arm measures it.
package all_test

import (
	"os"
	"testing"

	_ "github.com/qiangli/coreutils/cmds/all"
	posixgatecmd "github.com/qiangli/coreutils/cmds/posixgate"
	"github.com/qiangli/coreutils/pkg/posixprovider"
)

func TestProfileCDRegistrySelectsIntendedOwners(t *testing.T) {
	// The provider opt-out (BASHY_POSIX_PROVIDERS=off) unregisters the fifteen
	// provider names, so the assembled registry cannot own them. That is a
	// FAILURE of this certification gate, never a skip: a skipped gate reads
	// as green in a test log, and green while unmeasured is exactly the silent
	// substitution the gate exists to reject. VerifyRegistry is handed the
	// observed value and produces the opt-out rejection itself.
	for _, f := range posixgatecmd.VerifyRegistry(os.Getenv(posixprovider.OptOutEnv)) {
		t.Errorf("%s", f)
	}
}
