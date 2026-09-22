package lncmd

import (
	"io"
	"os"
	"strings"
)

// symlinkFallback is what ln -s does on a host that refused to create the
// symbolic link for lack of privilege. It is decided by
// chooseSymlinkFallback, a pure function, so the policy is tested on every
// host even though only Windows ever takes it.
type symlinkFallback int

const (
	// fallbackNone: the symlink error stands.
	fallbackNone symlinkFallback = iota
	// fallbackHardLink: the source and the destination share a volume, so
	// a hard link names the same file with no privilege at all.
	fallbackHardLink
	// fallbackCopy: different volumes; a copy is the closest thing to a
	// link the host allows.
	fallbackCopy
)

// chooseSymlinkFallback: on Windows CreateSymbolicLink needs
// SeCreateSymbolicLinkPrivilege (administrators, or Developer Mode) and
// fails with ERROR_PRIVILEGE_NOT_HELD otherwise — GitHub's runners are
// admin, an ordinary user's shell is not. The bash-5.3 corpus does
// `ln -s $THIS_SH /tmp/bash` and then RUNS /tmp/bash (execscript), so a
// refusal takes the whole fixture with it. When the refusal is the
// privilege one and the source is a regular file, fall back to a hard link
// on the same volume (NTFS hard links need no privilege and behave
// identically for reading and executing), else to a copy; any other error
// (destination exists, source missing, a directory source) is reported as
// the symlink failure it is. Unix never gets here: symlink(2) has no
// privilege gate.
func chooseSymlinkFallback(privilegeDenied, regularSource, sameVolume bool) symlinkFallback {
	if !privilegeDenied || !regularSource {
		return fallbackNone
	}
	if sameVolume {
		return fallbackHardLink
	}
	return fallbackCopy
}

// sameWindowsVolume reports whether two native Windows paths are on the
// same volume: the same drive letter (case-insensitively), or the same UNC
// \\server\share. A path with no volume is drive-relative, i.e. on the
// current drive; two such paths match each other, and neither matches a
// lettered path (the current drive is unknown here, so the safe answer is
// "different", which costs a copy instead of a failed link).
func sameWindowsVolume(a, b string) bool {
	return strings.EqualFold(windowsVolume(a), windowsVolume(b))
}

// windowsVolume is filepath.VolumeName for a Windows spelling on any host:
// "C:" for C:\x or C:/x, "\\server\share" for a UNC path, "" otherwise.
func windowsVolume(p string) string {
	if len(p) >= 2 && p[1] == ':' && isASCIILetter(p[0]) {
		return p[:2]
	}
	if len(p) >= 2 && isSlashByte(p[0]) && isSlashByte(p[1]) {
		// \\server\share: two components after the double separator.
		rest := p[2:]
		i := strings.IndexFunc(rest, isSlashRune)
		if i <= 0 {
			return ""
		}
		j := strings.IndexFunc(rest[i+1:], isSlashRune)
		if j < 0 {
			if len(rest[i+1:]) == 0 {
				return ""
			}
			return `\\` + strings.ReplaceAll(rest, "/", `\`)
		}
		if j == 0 {
			return ""
		}
		return `\\` + strings.ReplaceAll(rest[:i+1+j], "/", `\`)
	}
	return ""
}

func isASCIILetter(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }
func isSlashByte(c byte) bool   { return c == '/' || c == '\\' }
func isSlashRune(r rune) bool   { return r == '/' || r == '\\' }

// copyRegularFile is the fallbackCopy action: the source's bytes, created
// exclusively so an existing destination (which the caller has already
// dealt with under -f/-b/-i) is never clobbered by the fallback.
func copyRegularFile(srcPath, destPath string) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(destPath)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(destPath)
		return err
	}
	return nil
}
