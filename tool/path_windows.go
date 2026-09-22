//go:build windows

package tool

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/pathconv"
)

// pathLengthLimit for Windows is the \\?\ namespace bound (~32767
// UTF-16 units): the Go runtime rewrites long absolute paths into that
// namespace itself, so a joined absolute string stays usable far past
// MAX_PATH and the relative fallback in RunContext.Path is effectively
// never needed here.
var pathLengthLimit = 32000

// isAbsPath is pathconv.IsAbsMode: a native C:\x, and a leading slash or
// backslash in the shell's spelling (/tmp/x, /usr/bin, /c/x, \foo — all of
// which filepath.IsAbs rejects). A UNC or device path (//server/share,
// \\.\pipe\x) is absolute too and passes through normalizePath untouched.
func isAbsPath(p string) bool {
	return pathconv.IsAbsMode(p, true)
}

// normalizePath converts a shell-style path into a real Windows path. It is
// the tool entry point (RunContext.Path resolves operands through it), so
// the operand resolves exactly as bashy's interpreter resolves it:
// shellAbsMode delegates to pathconv.ToOS with the process mount table
// (BASHY_ROOT: /tmp -> %TEMP%, /bin -> root\usr\bin, /etc -> root\etc,
// / -> root), the MSYS and WSL drive forms, /dev/null -> NUL and the
// U+F000 encoding of the characters NTFS refuses. Without a mount table a
// drive-less /foo lands on C: (the interpreter's own fallback), not the
// process's current drive.
func normalizePath(p string) string {
	return normalizePathIn("", p)
}

// normalizePathIn is normalizePath with the invocation directory supplying
// the volume for a drive-relative /foo — the interpreter resolves such an
// operand against its cwd's drive, so an applet must too.
func normalizePathIn(dir, p string) string {
	return shellAbsMode(pathconv.CurrentMounts(), dir, p, true)
}

// joinPath resolves a relative operand under the invocation directory. The
// directory may be in the shell's spelling (bashy's in-process applets get
// hc.Dir as /tmp/x or /c/Users/x): it is converted first, so the join never
// produces the drive-relative \tmp\x that CreateFile resolves as C:\tmp\x.
func joinPath(dir, operand string) string {
	return shellJoinMode(pathconv.CurrentMounts(), dir, operand, true)
}

// displayName decodes the U+F000 NTFS specials in a name read back from
// the filesystem (ls output, diagnostics) so it prints as the shell spelled
// it.
func displayName(name string) string {
	return displayNameMode(name, true)
}

func pathextFromEnv(env []string) []string {
	var raw string
	for i := len(env) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(env[i], "="); ok && strings.EqualFold(k, "PATHEXT") {
			raw = v
			break
		}
	}
	if raw == "" {
		raw = ".COM;.EXE;.BAT;.CMD"
	}
	var exts []string
	for _, e := range strings.Split(raw, ";") {
		if e == "" {
			continue
		}
		if e[0] != '.' {
			e = "." + e
		}
		exts = append(exts, e)
	}
	return exts
}

func resolveExecutable(rc *RunContext, name string) string {
	p := rc.Path(name)
	if filepath.Ext(name) != "" {
		return p
	}
	exts := pathextFromEnv(rc.Env)
	dir, base := filepath.Dir(p), filepath.Base(p)
	// Resolve to the ACTUAL directory entry (case-preserved): Windows' FS is
	// case-insensitive, so "myprog"+PATHEXT ".BAT" matches a real "myprog.bat" —
	// return the file's own case, not the PATHEXT spelling, in PATHEXT priority.
	if ents, err := os.ReadDir(dir); err == nil {
		for _, e := range exts {
			want := base + e
			for _, ent := range ents {
				if ent.Type().IsRegular() && strings.EqualFold(ent.Name(), want) {
					return filepath.Join(dir, ent.Name())
				}
			}
		}
	}
	return p
}

func isRegularFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// defaultCommandPath is the search path used when PATH is unset. Windows has no
// execvp(3)/_CS_PATH convention, so the process's own PATH is the only sensible
// fallback (matching CreateProcess callers that defer to the environment).
func defaultCommandPath() string {
	return os.Getenv("PATH")
}

// pathIsExecutableBit: Windows has no Unix exec bit; any regular file is a
// candidate to run (the CreateProcess PATHEXT lookup decides the rest).
func pathIsExecutableBit(_ fs.FileInfo) bool { return true }

func systemDrive() string {
	sd := os.Getenv("SystemDrive")
	if sd == "" {
		sd = "C:"
	}
	return sd + `\`
}

// toOSPath converts a shell path (localFS layer) to a Windows path through
// the same resolver as normalizePath. The drive-less "/foo -> SystemDrive"
// mapping survives as ToOS's volume fallback with SystemDrive as the
// directory, so the toOSPath<->fromOSPath round-trip still holds; under a
// BASHY_ROOT mount table "/" is the root directory instead, as it is for
// the interpreter.
func toOSPath(p string) string {
	return shellAbsMode(pathconv.CurrentMounts(), systemDrive(), p, true)
}

// fromOSPath converts a native path back to the shell's spelling via the
// shared converter: C:\Users\x -> /c/Users/x (the MSYS drive form bashy
// speaks; cygpath -u prints the same shape under MSYS2), \foo -> /foo, UNC
// and relative paths are slash-converted only. The former SystemDrive
// stripping (C:\foo -> /foo) was lossy — /foo does not name a drive, and
// D:\foo could not round-trip at all.
func fromOSPath(p string) string {
	return pathconv.FromOS(p)
}
