//go:build !unix

package chmodcmd

import "github.com/qiangli/coreutils/tool"

// readOnlyHost: no POSIX mode bits exist here, only the read-only
// attribute; hostMode projects the computed bits onto it (see
// readOnlyProjection). chmod never refuses on this platform.
const readOnlyHost = true

// effectiveUmask returns the invoking shell's virtual mask when the command
// is embedded. A standalone invocation has no process mask to snapshot on
// this platform; the conventional 022 (what Cygwin and MSYS report for a
// fresh shell) is used so an omitted-who clause behaves as it would there.
func effectiveUmask(rc *tool.RunContext) uint32 {
	if rc.UmaskSet {
		return uint32(rc.Umask.Perm())
	}
	return 0o022
}
