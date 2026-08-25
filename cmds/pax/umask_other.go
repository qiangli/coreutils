//go:build !unix

package paxcmd

import (
	"os"

	"github.com/qiangli/coreutils/tool"
)

func invocationUmask(rc *tool.RunContext) os.FileMode {
	if rc.UmaskSet {
		return rc.Umask.Perm()
	}
	return 0
}
