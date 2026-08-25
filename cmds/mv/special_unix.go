//go:build unix

package mvcmd

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

func copySpecialNode(dp string, fi os.FileInfo) error {
	mode := uint32(fi.Mode().Perm())
	switch {
	case fi.Mode()&os.ModeNamedPipe != 0:
		return unix.Mkfifo(dp, mode)
	case fi.Mode()&os.ModeSocket != 0:
		ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: dp, Net: "unix"})
		if err == nil {
			ln.SetUnlinkOnClose(false)
			return ln.Close()
		}
		return err
	case fi.Mode()&(os.ModeDevice|os.ModeCharDevice) != 0:
		rdev, ok := deviceNumber(fi)
		if !ok {
			return fmt.Errorf("device metadata unavailable")
		}
		kind := uint32(unix.S_IFBLK)
		if fi.Mode()&os.ModeCharDevice != 0 {
			kind = uint32(unix.S_IFCHR)
		}
		return makeDeviceNode(dp, kind|mode, rdev)
	}
	return fmt.Errorf("unsupported special file type")
}
