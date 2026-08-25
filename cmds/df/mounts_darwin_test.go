//go:build darwin

package dfcmd

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestAPFSVolumeAllocatedSpace(t *testing.T) {
	mounts, err := listMounts()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range mounts {
		if m.fstype != "apfs" {
			continue
		}
		used, err := volumeSpaceUsed(m.point)
		if err != nil {
			t.Fatalf("volumeSpaceUsed(%q): %v", m.point, err)
		}
		if used != m.used || used > m.total {
			t.Fatalf("APFS %q: attr used=%d row used=%d total=%d", m.point, used, m.used, m.total)
		}

		var st unix.Statfs_t
		if err := unix.Statfs(m.point, &st); err != nil {
			t.Fatal(err)
		}
		statfsUsed := (st.Blocks - st.Bfree) * uint64(st.Bsize)
		if used != statfsUsed {
			// This is the load-bearing APFS case: the volume attribute excludes
			// reserved/purgeable bytes that statfs would mislabel as files.
			return
		}
	}
	t.Skip("no APFS mount exposing a statfs/volume allocation difference")
}
