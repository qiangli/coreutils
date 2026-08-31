//go:build darwin

package getconfcmd

import (
	"strconv"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

// Selector numbers from Darwin sys/unistd.h. Darwin numbers the selectors from
// 1 where Linux starts at 0, which is precisely the transcription error
// sysconf_test.go exists to catch. Darwin libc has no timestamp selector, so
// pcTimestampResolution is internal and handled before Pathconf.
const (
	scArgMax          = 1
	scChildMax        = 2
	scClkTck          = 3
	scNgroupsMax      = 4
	scOpenMax         = 5
	scPagesize        = 29
	scNprocessorsConf = 57
	scNprocessorsOnln = 58

	pcLinkMax             = 1
	pcMaxCanon            = 2
	pcMaxInput            = 3
	pcNameMax             = 4
	pcPathMax             = 5
	pcPipeBuf             = 6
	pcChownRestricted     = 7
	pcNoTrunc             = 8
	pcVdisable            = 9
	pc2Symlinks           = 15
	pcAllocSizeMin        = 16
	pcAsyncIO             = 17
	pcFilesizeBits        = 18
	pcPrioIO              = 19
	pcRecIncrXferSize     = 20
	pcRecMaxXferSize      = 21
	pcRecMinXferSize      = 22
	pcRecXferAlign        = 23
	pcSymlinkMax          = 24
	pcSyncIO              = 25
	pcTimestampResolution = 26
)

// Darwin has a real pathconf(2), so ask the kernel. A negative result with no
// error is POSIX's "no limit", which must print as "undefined" rather than -1.
func pathconfStr(rc *tool.RunContext, which int, path string) (string, bool, error) {
	p := path
	if rc != nil {
		p = rc.Path(path)
	}
	if which == pcTimestampResolution {
		var st unix.Statfs_t
		if err := unix.Statfs(p, &st); err != nil {
			return "", true, err
		}
		if resolution, ok := darwinTimestampResolution(unix.ByteSliceToString(st.Fstypename[:])); ok {
			return resolution, true, nil
		}
		return undefined, true, nil
	}
	v, err := unix.Pathconf(p, which)
	if err != nil {
		return "", true, err
	}
	if v < 0 {
		return undefined, true, nil
	}
	return strconv.Itoa(v), true, nil
}

// Apple documents APFS timestamp granularity as one nanosecond and HFS+ as
// one second. Unknown and remote filesystems stay undefined rather than
// inheriting the host volume's answer.
func darwinTimestampResolution(filesystem string) (string, bool) {
	switch filesystem {
	case "apfs":
		return "1", true
	case "hfs":
		return "1000000000", true
	default:
		return "", false
	}
}

// The POSIX revision this platform conforms to, as its own libc reports it.
const (
	posixVersion  = 200112
	posix2Version = 200112
)
