//go:build windows

package lncmd

import (
	"errors"
	"os"
	"syscall"

	"mvdan.cc/sh/v3/pathconv"

	"github.com/qiangli/coreutils/tool"
)

// createSymlink creates the symbolic link with CreateSymbolicLink and, when
// the host refuses for lack of SeCreateSymbolicLinkPrivilege (an ordinary
// user's shell without Developer Mode), falls back the way
// chooseSymlinkFallback documents: a hard link when srcPath and destPath
// share a volume, a copy otherwise. srcPath is the resolved source (what
// the link would point at), used only by the fallbacks.
//
// The link CONTENT is spelled natively: the operand arrives in the shell's
// spelling (/d/a/bashy/bin/bash.exe, /tmp/x) and stored verbatim it would
// be a drive-relative \d\a\… that resolves nowhere; a relative operand
// keeps its shape with native separators and the NTFS-special encoding, as
// the shell's own redirection would spell it.
func createSymlink(linkTarget, destPath, srcPath string) error {
	err := os.Symlink(nativeLinkContent(linkTarget), destPath)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.ERROR_PRIVILEGE_NOT_HELD) {
		return err
	}
	fi, statErr := os.Stat(srcPath)
	regular := statErr == nil && fi.Mode().IsRegular()
	switch chooseSymlinkFallback(true, regular, sameWindowsVolume(srcPath, destPath)) {
	case fallbackHardLink:
		if os.Link(srcPath, destPath) == nil {
			return nil
		}
		// A hard link can still fail (FAT volumes have none); the copy is
		// the last resort on the same volume too.
		fallthrough
	case fallbackCopy:
		if copyRegularFile(srcPath, destPath) == nil {
			return nil
		}
	}
	return err
}

func nativeLinkContent(target string) string {
	if native, ok := tool.NativeAbs(target); ok {
		return native
	}
	return pathconv.EncodeSpecialMode(toBackslash(target), true)
}

func toBackslash(p string) string {
	out := []byte(p)
	for i, c := range out {
		if c == '/' {
			out[i] = '\\'
		}
	}
	return string(out)
}
