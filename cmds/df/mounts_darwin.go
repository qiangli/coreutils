//go:build darwin

package dfcmd

import "golang.org/x/sys/unix"

func listMounts() ([]mountEntry, error) {
	n, err := unix.Getfsstat(nil, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}
	buf := make([]unix.Statfs_t, n)
	n, err = unix.Getfsstat(buf, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}
	var out []mountEntry
	for i := 0; i < n; i++ {
		st := &buf[i]
		bs := uint64(st.Bsize)
		out = append(out, mountEntry{
			device: unix.ByteSliceToString(st.Mntfromname[:]),
			point:  unix.ByteSliceToString(st.Mntonname[:]),
			total:  st.Blocks * bs,
			used:   (st.Blocks - st.Bfree) * bs,
			avail:  uint64(st.Bavail) * bs,
		})
	}
	return out, nil
}
