//go:build darwin

package getconfcmd

import (
	"path/filepath"
	"strconv"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

// Selector numbers from Darwin sys/unistd.h. Darwin numbers the selectors from
// 1 where Linux starts at 0, which is precisely the transcription error
// sysconf_test.go exists to catch.
const (
	scArgMax          = 1
	scChildMax        = 2
	scClkTck          = 3
	scNgroupsMax      = 4
	scOpenMax         = 5
	scPagesize        = 29
	scNprocessorsConf = 57
	scNprocessorsOnln = 58

	pcLinkMax         = 1
	pcMaxCanon        = 2
	pcMaxInput        = 3
	pcNameMax         = 4
	pcPathMax         = 5
	pcPipeBuf         = 6
	pcChownRestricted = 7
	pcNoTrunc         = 8
	pcVdisable        = 9
)

// Darwin has a real pathconf(2), so ask the kernel. A negative result with no
// error is POSIX's "no limit", which must print as "undefined" rather than -1.
func pathconfStr(rc *tool.RunContext, which int, path string) (string, bool) {
	p := path
	if !filepath.IsAbs(p) && rc != nil && rc.Dir != "" {
		p = filepath.Join(rc.Dir, p)
	}
	v, err := unix.Pathconf(p, which)
	if err != nil || v < 0 {
		return undefined, true
	}
	return strconv.Itoa(v), true
}

// The POSIX revision this platform conforms to, as its own libc reports it.
const (
	posixVersion  = 200112
	posix2Version = 200112
)
