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
	"testing"

	_ "github.com/qiangli/coreutils/cmds/all"
	posixgatecmd "github.com/qiangli/coreutils/cmds/posixgate"
	"github.com/qiangli/coreutils/pkg/posixprovider"
)

func TestProfileCDRegistrySelectsIntendedOwners(t *testing.T) {
	if !posixprovider.Enabled() {
		t.Skip("BASHY_POSIX_PROVIDERS=off in this process: provider names are deliberately unregistered")
	}
	for _, f := range posixgatecmd.VerifyRegistry("") {
		t.Errorf("%s", f)
	}
}
