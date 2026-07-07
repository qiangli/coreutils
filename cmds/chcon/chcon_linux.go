//go:build linux

package chconcmd

import (
	"golang.org/x/sys/unix"

	"github.com/qiangli/coreutils/tool"
)

func applyContext(rc *tool.RunContext, context string, files []string) int {
	exit := 0
	for _, name := range files {
		if err := unix.Setxattr(rc.Path(name), "security.selinux", []byte(context), 0); err != nil {
			reportChconErr(rc, name, context, err)
			exit = 1
		}
	}
	return exit
}
