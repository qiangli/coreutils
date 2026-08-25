//go:build linux

package getconfcmd

import (
	"strconv"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
	"path/filepath"
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
	pc2Symlinks       = pcUndefined
	pcAllocSizeMin    = pcUndefined
	pcAsyncIO         = pcUndefined
	pcFilesizeBits    = pcUndefined
	pcPrioIO          = pcUndefined
	pcRecIncrXferSize = pcUndefined
	pcRecMaxXferSize  = pcUndefined
	pcRecMinXferSize  = pcUndefined
	pcRecXferAlign    = pcUndefined
	pcSymlinkMax      = pcUndefined
	pcSyncIO          = pcUndefined
)

// Linux has no pathconf(2). glibc's answers are libc policy, not kernel API;
// absent a libc adapter we only report the filesystem name limit that statfs
// actually exposes. Every other value is deliberately undefined rather than a
// POSIX minimum or a guessed glibc default.
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
			return strconv.FormatUint(uint64(st.Namelen), 10), true
		}
	}
	return undefined, true
}

// The POSIX revision this platform conforms to, as its own libc reports it.
const (
	posixVersion  = 200809
	posix2Version = 200809
)
