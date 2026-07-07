// Portions adapted from https://github.com/synseqack/aict tools/df/df_windows.go (MIT).
// Changes: rewired to the tool framework and golang.org/x/sys/windows;
// GNU plain-text output only (no structured output modes).

//go:build windows

package dfcmd

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// listMounts enumerates fixed logical drives via GetLogicalDrives,
// sizing each with GetDiskFreeSpaceEx. "Available" honors per-user
// quotas (free bytes available to the caller), like GNU's f_bavail.
func listMounts() ([]mountEntry, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, err
	}
	var out []mountEntry
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p, perr := windows.UTF16PtrFromString(root)
		if perr != nil {
			continue
		}
		if windows.GetDriveType(p) != windows.DRIVE_FIXED {
			continue
		}
		var availCaller, total, totalFree uint64
		if err := windows.GetDiskFreeSpaceEx(p, &availCaller, &total, &totalFree); err != nil {
			continue
		}
		out = append(out, mountEntry{
			device: root[:2],
			point:  root,
			fstype: "fixed",
			total:  total,
			used:   total - totalFree,
			avail:  availCaller,
		})
	}
	return out, nil
}

func syncFilesystems() {}

func mountForFile(path string, mounts []mountEntry) (int, bool) {
	vol := filepath.VolumeName(path)
	if vol == "" {
		return 0, false
	}
	want := strings.ToUpper(vol)
	for i, m := range mounts {
		if strings.ToUpper(m.device) == want {
			return i, true
		}
	}
	return 0, false
}
