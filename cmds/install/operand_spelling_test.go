package installcmd

// Story #682 (S245.5f): an applet rebuilds its destination operand from the
// target directory and the source's basename. That rebuilt operand must stay
// in the SPELLING the caller used — on Windows pathconv's mount lookup is
// defined on the POSIX spelling, so a filepath.Join that Cleans /tmp/d into
// \tmp\d drops out of the /tmp mount and lands on C:\tmp ("The system
// cannot find the path specified" on the bash-5.3 fixture runner).
//
// tool.SetOperandWindows pins the operand-spelling mode so the Windows
// behavior is asserted here on a Unix developer host, and
// tool.ResolveOperandMode answers where the operand would land against an
// injected mount table.

import (
	"testing"

	"mvdan.cc/sh/v3/pathconv"

	"github.com/qiangli/coreutils/tool"
)

// The fixture runner's layout: BASHY_ROOT beside a private TEMP.
const (
	operandFixtureRoot = `C:\T\bash53-1\root`
	operandFixtureTmp  = `C:\T\bash53-1\tmp`
)

func operandFixtureMounts() *pathconv.Mounts {
	return pathconv.NewMounts(operandFixtureRoot, nil, operandFixtureTmp)
}

func TestDestForKeepsOperandSpelling(t *testing.T) {
	defer tool.SetOperandWindows(true)()

	dst := destFor("/tmp/execdir-4440", "/usr/bin/bash.exe")
	if want := "/tmp/execdir-4440/bash.exe"; dst != want {
		t.Fatalf("destFor = %q; want %q", dst, want)
	}
	// ...and that operand still resolves under the mounted /tmp.
	got := tool.ResolveOperandMode(operandFixtureMounts(), "", dst, true)
	if want := operandFixtureTmp + `\execdir-4440\bash.exe`; got != want {
		t.Errorf("resolved %q = %q; want %q", dst, got, want)
	}
	// A destination already in the native spelling gets the native join —
	// the half of the rule that is observable on every host.
	if got, want := destFor(`C:\d`, "/usr/bin/bash.exe"), `C:\d\bash.exe`; got != want {
		t.Errorf("destFor(native) = %q; want %q", got, want)
	}
}
