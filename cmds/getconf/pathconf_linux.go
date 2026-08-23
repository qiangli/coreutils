//go:build linux

package getconfcmd

import (
	"path/filepath"
	"strconv"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

// Selector numbers from glibc bits/confname.h. They are userspace ABI and
// cannot be renumbered; sysconf_test.go still cross-checks them against the
// host's own getconf so a transcription error cannot survive.
const (
	scArgMax          = 0
	scChildMax        = 1
	scClkTck          = 2
	scNgroupsMax      = 3
	scOpenMax         = 4
	scPagesize        = 30
	scNprocessorsConf = 83
	scNprocessorsOnln = 84

	pcLinkMax         = 0
	pcMaxCanon        = 1
	pcMaxInput        = 2
	pcNameMax         = 3
	pcPathMax         = 4
	pcPipeBuf         = 5
	pcChownRestricted = 6
	pcNoTrunc         = 7
	pcVdisable        = 8
)

// Linux has no pathconf(2) — glibc computes it from fixed limits plus what the
// filesystem reports. Do the same rather than pretend a syscall exists: the
// only value that genuinely varies per filesystem is NAME_MAX, which statfs
// supplies. The path is still stat'ed first so a missing operand is an error
// rather than a silently fabricated constant.
func pathconfStr(rc *tool.RunContext, which int, path string) (string, bool) {
	p := path
	if !filepath.IsAbs(p) && rc != nil && rc.Dir != "" {
		p = filepath.Join(rc.Dir, p)
	}
	var st unix.Statfs_t
	if err := unix.Statfs(p, &st); err != nil {
		return undefined, true
	}
	switch which {
	case pcNameMax:
		if st.Namelen > 0 {
			return strconv.FormatInt(int64(st.Namelen), 10), true
		}
		return "255", true
	case pcPathMax:
		return "4096", true
	case pcPipeBuf:
		return "4096", true
	case pcLinkMax:
		return "127", true
	case pcMaxCanon, pcMaxInput:
		return "255", true
	case pcChownRestricted, pcNoTrunc:
		return "1", true
	case pcVdisable:
		return "0", true
	}
	return undefined, true
}

// The POSIX revision this platform conforms to, as its own libc reports it.
const (
	posixVersion  = 200809
	posix2Version = 200809
)
