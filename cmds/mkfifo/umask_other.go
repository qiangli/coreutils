//go:build !unix

package mkfifocmd

import "github.com/qiangli/coreutils/tool"

func effectiveUmask(rc *tool.RunContext) uint32 {
	if rc.UmaskSet {
		return uint32(rc.Umask.Perm())
	}
	return 0
}
