//go:build unix

package cpcmd

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func isSpecial(mode os.FileMode) bool {
	return mode&(os.ModeNamedPipe|os.ModeSocket|os.ModeDevice|os.ModeCharDevice) != 0
}

// copySpecial recreates a special node for recursive copies. GNU cp only
// opens the node and copies its stream when --copy-contents is requested.
func (c *copier) copySpecial(src, dst string, fi os.FileInfo) {
	// Link modes apply to every source type and never read the node.
	if c.link || c.symlink {
		c.copyFile(src, dst, fi)
		return
	}
	dp := c.path(dst)
	if di, err := os.Lstat(dp); err == nil {
		if os.SameFile(fi, di) {
			c.errf("'%s' and '%s' are the same file", src, dst)
			return
		}
		if di.IsDir() {
			c.errf("cannot overwrite directory '%s' with non-directory", dst)
			return
		}
		if c.noClobber {
			return
		}
		if c.update && !sourceNewer(c.path(src), dp) {
			return
		}
		if c.interactive && !c.confirm(dst) {
			return
		}
		if c.backup && !c.backupDest(dst) {
			return
		}
		if !c.backup {
			if err := os.Remove(dp); err != nil {
				c.errf("cannot remove '%s': %s", dst, reason(err))
				return
			}
		}
	}
	if parent := filepath.Dir(dst); parent != "." && parent != dst {
		if err := os.MkdirAll(c.path(parent), 0o777); err != nil {
			c.errf("cannot create directory '%s': %s", filepath.Dir(dst), reason(err))
			return
		}
	}

	mode := uint32(fi.Mode().Perm())
	var err error
	switch {
	case fi.Mode()&os.ModeNamedPipe != 0:
		err = unix.Mkfifo(dp, mode)
	case fi.Mode()&os.ModeSocket != 0:
		var ln *net.UnixListener
		ln, err = net.ListenUnix("unix", &net.UnixAddr{Name: dp, Net: "unix"})
		if err == nil {
			// ListenUnix otherwise removes a pathname socket on Close.
			ln.SetUnlinkOnClose(false)
			err = ln.Close()
		}
	case fi.Mode()&(os.ModeDevice|os.ModeCharDevice) != 0:
		rdev, ok := deviceNumber(fi)
		if !ok {
			err = fmt.Errorf("device metadata unavailable")
			break
		}
		kind := uint32(unix.S_IFBLK)
		if fi.Mode()&os.ModeCharDevice != 0 {
			kind = uint32(unix.S_IFCHR)
		}
		err = makeDeviceNode(dp, kind|mode, rdev)
	default:
		err = fmt.Errorf("unsupported special file type")
	}
	if err != nil {
		c.errf("cannot create special file '%s': %s", dst, reason(err))
		return
	}
	if c.preserve.any() {
		c.preserveAttrs(src, dst, fi)
	}
	c.debugf("copied special file '%s' -> '%s'", src, dst)
	c.verbosef("'%s' -> '%s'", src, dst)
}
