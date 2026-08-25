//go:build darwin

package dfcmd

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

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
		point := unix.ByteSliceToString(st.Mntonname[:])
		// APFS Getfsstat snapshots can report f_bfree as f_bavail and thereby
		// count reserved/purgeable space as allocated. Refresh each selected
		// mount by pathname, as native df does for an operand.
		var current unix.Statfs_t
		if err := unix.Statfs(point, &current); err == nil {
			st = &current
		}
		bs := uint64(st.Bsize)
		fstype := unix.ByteSliceToString(st.Fstypename[:])
		used := uint64(0)
		if apfsUsed, err := volumeSpaceUsed(point); err == nil {
			used = apfsUsed
		} else if fstype == "apfs" {
			return nil, fmt.Errorf("APFS allocated-space query for %s: %w", point, err)
		} else if st.Blocks > st.Bfree {
			used = (st.Blocks - st.Bfree) * bs
		}
		out = append(out, mountEntry{
			device:         unix.ByteSliceToString(st.Mntfromname[:]),
			point:          point,
			fstype:         fstype,
			total:          st.Blocks * bs,
			used:           used,
			avail:          spaceFromBlocks(uint64(st.Bavail), st.Blocks, bs),
			files:          st.Files,
			ifree:          st.Ffree,
			fileSlotsKnown: true,
		})
	}
	return out, nil
}

// volumeSpaceUsed asks Darwin's volume-attribute API for the allocated byte
// count used by native df on APFS. statfs f_blocks-f_bfree includes space that
// APFS considers reserved or purgeable and can materially overstate files'
// allocated space. APFS query failure is fatal rather than knowingly emit an
// inflated value; other filesystems may fall back to their statfs result.
func volumeSpaceUsed(path string) (uint64, error) {
	p, err := unix.BytePtrFromString(path)
	if err != nil {
		return 0, err
	}
	attrs := unix.Attrlist{
		Bitmapcount: unix.ATTR_BIT_MAP_COUNT,
		Volattr:     unix.ATTR_VOL_SPACEUSED,
	}
	var result [12]byte // uint32 returned length followed by off_t space-used
	_, _, errno := unix.Syscall6(
		unix.SYS_GETATTRLIST,
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&attrs)),
		uintptr(unsafe.Pointer(&result[0])),
		uintptr(len(result)),
		unix.FSOPT_REPORT_FULLSIZE,
		0,
	)
	if errno != 0 {
		return 0, errno
	}
	if binary.LittleEndian.Uint32(result[:4]) < uint32(len(result)) {
		return 0, unix.EIO
	}
	return binary.LittleEndian.Uint64(result[4:]), nil
}

func syncFilesystems() { unix.Sync() }
