//go:build !unix

package chgrpcmd

import (
	"fmt"
	"os"

	"github.com/qiangli/coreutils/tool"
)

// This platform has no POSIX uid/gid ownership: there is no group for a
// file to belong to, so there is nothing a group change could do and
// nothing it could undo. chgrp is therefore a SUCCESSFUL NO-OP here — it
// verifies that each operand exists, diagnoses the ones that do not
// exactly as the Unix leg does, and otherwise exits 0 with no output.
//
// This is the same decision chmod's read-only projection records (Story
// #682, Sprint 245.5c) and for the same reason: refusing outright turns
// every script that touches ownership before doing its real work into a
// failed script, and on Windows that is most of them. bash's own
// test.tests chgrps a scratch file and then tests -g on it; a hard error
// derailed the whole fixture over an operation that has no meaning on the
// platform. Cygwin and MSYS answer the same way.
//
// The deviation is deliberate and bounded: the operand check is kept, so
// `chgrp g missing` still fails with the GNU diagnostic and exit 1. What
// is dropped is only the part of the command that cannot exist here. It is
// NOT a silent approximation of ownership — no ownership is invented, read
// back, or reported. -v/-c print nothing because nothing changed and
// nothing was retained; there is no ownership to name.
func apply(rc *tool.RunContext, _ string, o options) int {
	exit := 0
	for _, name := range o.files {
		if _, err := os.Lstat(rc.Path(name)); err != nil {
			// -f suppresses the message, not the failure — the same rule
			// the Unix leg applies to an unreachable operand.
			if !o.silent {
				fmt.Fprintf(rc.Err, "chgrp: cannot access '%s': %s\n", name, tool.SysErrString(err))
			}
			exit = 1
		}
	}
	return exit
}

// parseFromSpec accepts any --from spec: there are no ids to compare it
// against, so it can never exclude an operand.
func parseFromSpec(string) (int, int, error) { return -1, -1, nil }

// statFile backs --reference. The file must exist — that check is real on
// every platform — but the ids it would supply do not.
func statFile(rc *tool.RunContext, name string) (*refFileInfo, error) {
	if _, err := os.Stat(rc.Path(name)); err != nil {
		return nil, tool.SysErr(err)
	}
	return &refFileInfo{}, nil
}

type refFileInfo struct{}

func (*refFileInfo) ids() (uid, gid int) { return -1, -1 }

func statusError(rc *tool.RunContext, format string, a ...any) int {
	fmt.Fprintf(rc.Err, "chgrp: "+format+"\n", a...)
	return 1
}
