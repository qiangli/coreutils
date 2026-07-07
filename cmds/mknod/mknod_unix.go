//go:build unix

package mknodcmd

import "golang.org/x/sys/unix"

func makeNode(path string, kind nodeKind, mode, major, minor uint32) error {
	switch kind {
	case nodeFIFO:
		return unix.Mkfifo(path, mode)
	case nodeBlock:
		return unix.Mknod(path, unix.S_IFBLK|mode, int(unix.Mkdev(major, minor)))
	case nodeChar:
		return unix.Mknod(path, unix.S_IFCHR|mode, int(unix.Mkdev(major, minor)))
	default:
		panic("mknod: unknown node kind")
	}
}
