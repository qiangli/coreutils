// Portions adapted from https://github.com/u-root/u-root cmds/core/df/df.go (BSD-3-Clause).
// Changes: rewired to the tool framework; statfs via golang.org/x/sys/unix;
// octal mount-path unescaping; byte-precise totals (scaling done at print time).

//go:build linux

package dfcmd

import (
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func listMounts() ([]mountEntry, error) {
	buf, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, err
	}
	var out []mountEntry
	for _, ln := range strings.Split(string(buf), "\n") {
		f := strings.Fields(ln)
		if len(f) < 3 {
			continue
		}
		point := unescapeMount(f[1])
		var st unix.Statfs_t
		if err := unix.Statfs(point, &st); err != nil {
			continue // unreadable mount (permissions, stale) — skip, as GNU does
		}
		bs := uint64(st.Bsize)
		used := uint64(0)
		if st.Blocks > st.Bfree {
			used = (st.Blocks - st.Bfree) * bs
		}
		out = append(out, mountEntry{
			device: unescapeMount(f[0]),
			point:  point,
			fstype: f[2],
			total:  st.Blocks * bs,
			used:   used,
			avail:  uint64(st.Bavail) * bs,
			files:  st.Files,
			ifree:  st.Ffree,
		})
	}
	return out, nil
}

func syncFilesystems() { unix.Sync() }

// unescapeMount decodes the \ooo octal escapes /proc/mounts uses for
// spaces, tabs, newlines, and backslashes in mount paths.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) &&
			isOctal(s[i+1]) && isOctal(s[i+2]) && isOctal(s[i+3]) {
			b.WriteByte((s[i+1]-'0')<<6 | (s[i+2]-'0')<<3 | (s[i+3] - '0'))
			i += 3
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isOctal(c byte) bool { return c >= '0' && c <= '7' }
