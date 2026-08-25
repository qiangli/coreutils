//go:build linux

package getconfcmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	// Linux has no pathconf system call. These private selectors only route
	// values that the adapter can derive from Linux kernel interfaces.
	pc2Symlinks           = 100
	pcAllocSizeMin        = 101
	pcAsyncIO             = pcUndefined
	pcFilesizeBits        = pcUndefined
	pcPrioIO              = pcUndefined
	pcRecIncrXferSize     = pcUndefined
	pcRecMaxXferSize      = pcUndefined
	pcRecMinXferSize      = 102
	pcRecXferAlign        = 103
	pcSymlinkMax          = pcUndefined
	pcSyncIO              = pcUndefined
	pcTimestampResolution = 104
)

// Linux has no pathconf(2). glibc's answers are libc policy, not kernel API;
// absent a libc adapter we report only values supplied by statfs, Linux UAPI
// invariants, or the mounted filesystem type. Every other value is deliberately
// undefined rather than a POSIX minimum or a guessed glibc default.
func pathconfStr(rc *tool.RunContext, which int, path string) (string, bool, error) {
	p := path
	if !filepath.IsAbs(p) && rc != nil && rc.Dir != "" {
		p = filepath.Join(rc.Dir, p)
	}
	var st unix.Statfs_t
	if err := unix.Statfs(p, &st); err != nil {
		return "", true, err
	}
	switch which {
	case pcNameMax:
		if st.Namelen > 0 {
			return strconv.FormatUint(uint64(st.Namelen), 10), true, nil
		}
	case pcMaxCanon, pcMaxInput:
		return "255", true, nil
	case pcPathMax, pcPipeBuf:
		return "4096", true, nil
	case pcChownRestricted, pcNoTrunc, pc2Symlinks:
		return "1", true, nil
	case pcVdisable:
		return "0", true, nil
	case pcAllocSizeMin, pcRecMinXferSize, pcRecXferAlign:
		if st.Bsize > 0 {
			return strconv.FormatInt(int64(st.Bsize), 10), true, nil
		}
	case pcTimestampResolution:
		if linuxNanosecondFilesystem(p) {
			return "1", true, nil
		}
	}
	return undefined, true, nil
}

func linuxNanosecondFilesystem(path string) bool {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	clean := filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		clean = resolved
	}
	bestLen, bestType := -1, ""
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) != 2 {
			continue
		}
		left, right := strings.Fields(parts[0]), strings.Fields(parts[1])
		if len(left) < 5 || len(right) < 1 {
			continue
		}
		mountpoint := decodeMountInfoPath(left[4])
		if clean != mountpoint && !strings.HasPrefix(clean, strings.TrimSuffix(mountpoint, "/")+"/") {
			continue
		}
		if len(mountpoint) > bestLen {
			bestLen, bestType = len(mountpoint), right[0]
		}
	}
	switch bestType {
	case "btrfs", "ext4", "overlay", "tmpfs", "xfs":
		return true
	}
	return false
}

func decodeMountInfoPath(value string) string {
	r := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return r.Replace(value)
}

// The POSIX revision this platform conforms to, as its own libc reports it.
const (
	posixVersion  = 200809
	posix2Version = 200809
)
