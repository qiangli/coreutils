package tool

import (
	"path/filepath"
	"runtime"
	"strings"

	"mvdan.cc/sh/v3/pathconv"
)

// The shell-spelling resolvers below are the Windows path layer written
// against an explicit mount table and windows flag, so the conversion an
// applet applies to an operand is testable on every host. The build-tagged
// entry points (normalizePath, joinPath, toOSPath) pass
// pathconv.CurrentMounts() and windows=true.
//
// The rule is one sentence: an operand in the shell's spelling resolves
// exactly as bashy's interpreter resolves it. That is pathconv.ToOS — the
// SAME converter the sh engine runs — which applies the BASHY_ROOT mount
// table (/tmp -> %TEMP%, /bin and /usr/bin -> root\usr\bin, /etc ->
// root\etc, / -> root), the MSYS and WSL drive forms (/c/x, /mnt/c/x),
// /dev/null -> NUL, the drive-relative fallback onto dir's volume, and the
// Cygwin/MSYS encoding of the characters NTFS refuses in a filename
// (: * ? " < > | -> U+F000+c), so `touch 'x*x'` creates the file the shell's
// own redirection `>'x*x'` would. Before this the tool layer recognized only
// the drive forms and /tmp locally, so /tmp/x under a private fixture TEMP
// became C:\tmp\x, /bin/sh could not be stat'ed at all, and x*x was refused
// by CreateFile.

// shellAbsMode converts operand — absolute in the shell's spelling, or
// relative when the caller has no directory to join it onto — into the
// host's native form. dir supplies the volume for a drive-relative /foo and
// may itself be in the shell's spelling. A trailing separator survives
// (RunContext.Path treats it as semantic). Off windows mode the operand is
// returned unchanged.
func shellAbsMode(m *pathconv.Mounts, dir, operand string, windows bool) string {
	if !windows || operand == "" {
		return operand
	}
	if isSlashByte(operand[0]) && len(operand) >= 2 && isSlashByte(operand[1]) {
		// Device and UNC-prefixed paths (\\.\pipe\x, //server/share) are
		// native already; only the separators are normalized.
		return toBackslash(operand)
	}
	var out string
	if pathconv.IsAbsMode(operand, true) {
		out = pathconv.ToOSMountsMode(m, shellDirMode(m, dir, true), operand, true)
	} else {
		// A relative operand with nothing to join onto: encode the NTFS
		// specials and spell the separators natively, exactly as the
		// absolute case does for the remainder under a mount.
		out = pathconv.EncodeSpecialMode(toBackslash(operand), true)
	}
	if hasTrailingSlashByte(operand) && !hasTrailingSlashByte(out) {
		out += `\`
	}
	return out
}

// shellJoinMode resolves a relative operand under dir the way the
// interpreter's own JoinAbs does: dir is converted first (it is in the
// shell's spelling after a cd, /tmp/x or /c/Users/x — joined raw it would
// become the drive-relative \tmp\x and then C:\tmp\x), the NTFS specials in
// operand are encoded before filepath.Join can see a colon that is not the
// drive's, and the result is cleaned like every native path.
func shellJoinMode(m *pathconv.Mounts, dir, operand string, windows bool) string {
	if !windows {
		return filepath.Join(dir, operand)
	}
	nativeDir := shellDirMode(m, dir, true)
	enc := pathconv.EncodeSpecialMode(toBackslash(operand), true)
	if runtime.GOOS == "windows" {
		return filepath.Join(nativeDir, enc)
	}
	// Host-independent spelling of the same join for the tests: no Clean
	// pass exists for a foreign separator, so the pieces are concatenated.
	switch {
	case nativeDir == "":
		return enc
	case enc == "":
		return nativeDir
	case hasTrailingSlashByte(nativeDir):
		return nativeDir + enc
	}
	return nativeDir + `\` + enc
}

// shellDirMode is the invocation directory in native form; an empty dir
// stays empty so ToOS falls back to the system drive.
func shellDirMode(m *pathconv.Mounts, dir string, windows bool) string {
	if !windows || dir == "" {
		return dir
	}
	return pathconv.ToOSMountsMode(m, "", dir, true)
}

// displayNameMode spells a directory-entry or operand name the way the
// shell would print it: the U+F000-encoded NTFS specials become the
// characters they stand for, so a file the shell created as `x*x` is listed
// as x*x and named that way in diagnostics. Off windows mode it is the
// identity.
func displayNameMode(name string, windows bool) string {
	return pathconv.DecodeSpecialMode(name, windows)
}

func isSlashByte(c byte) bool { return c == '/' || c == '\\' }

// hasTrailingSlashByte is hasTrailingPathSeparator for a Windows spelling
// on any host: both separators count.
func hasTrailingSlashByte(p string) bool { return p != "" && isSlashByte(p[len(p)-1]) }

func toBackslash(p string) string { return strings.ReplaceAll(p, "/", `\`) }
