//go:build darwin

package getconfcmd

import (
	"testing"

	"golang.org/x/sys/unix"
)

// APFS records file timestamps at nanosecond granularity. getconf's pathname
// query must report that filesystem property even though Darwin libc does not
// expose _PC_TIMESTAMP_RESOLUTION through pathconf(2).
func TestDarwinAPFSTimestampResolution(t *testing.T) {
	dir := t.TempDir()
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		t.Fatal(err)
	}
	if fs := unix.ByteSliceToString(st.Fstypename[:]); fs != "apfs" {
		t.Skipf("public APFS reducer requires APFS, got %q", fs)
	}
	got, stderr, code := runCmd(t, "_POSIX_TIMESTAMP_RESOLUTION", dir)
	if code != 0 || stderr != "" || got != "1" {
		t.Fatalf("APFS timestamp resolution = (%q, %q, %d), want (1, empty, 0)", got, stderr, code)
	}
}

func TestDarwinTimestampResolutionIsFilesystemSpecific(t *testing.T) {
	for _, tc := range []struct {
		filesystem string
		want       string
		ok         bool
	}{
		{filesystem: "apfs", want: "1", ok: true},
		{filesystem: "hfs", want: "1000000000", ok: true},
		{filesystem: "nfs"},
		{filesystem: "smbfs"},
	} {
		got, ok := darwinTimestampResolution(tc.filesystem)
		if got != tc.want || ok != tc.ok {
			t.Errorf("darwinTimestampResolution(%q) = (%q, %v), want (%q, %v)",
				tc.filesystem, got, ok, tc.want, tc.ok)
		}
	}
}
