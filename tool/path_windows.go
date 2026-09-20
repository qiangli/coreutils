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

func isAbsPath(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	if p == "" {
		return false
	}
	// A leading slash OR backslash is a drive-relative absolute path on Windows
	// (we map it onto the system drive); a doubled separator (UNC) is not.
	if p[0] == '/' || p[0] == '\\' {
		return len(p) < 2 || (p[1] != '/' && p[1] != '\\')
	}
	return false
}

// normalizePath converts a shell-style path into a real Windows path. It is the
// tool entry point (RunContext.Path resolves operands through it), so every
// shell spelling the shared pathconv package recognizes is honored here:
// /c/foo and /mnt/c/foo -> C:\foo, /dev/null -> NUL, /tmp -> the host temp
// directory. A drive-less path is just slash-converted (a leading "/" stays
// drive-relative, as before).
func normalizePath(p string) string {
	if native, ok := shellSpecialPath(p); ok {
		return native
	}
	return filepath.FromSlash(p)
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

// toOSPath converts a shell path (localFS layer) to a Windows path. It honors
// every shell spelling the shared pathconv package recognizes (MSYS /c/…,
// WSL /mnt/c/…, /dev/null, /tmp), then the legacy drive-less
// "/foo -> SystemDrive" mapping (kept so the toOSPath<->fromOSPath
// round-trip holds).
func toOSPath(p string) string {
	if native, ok := shellSpecialPath(p); ok {
		return native
	}
	if len(p) > 0 && p[0] == '/' && (len(p) < 2 || p[1] != '/') {
		return systemDrive() + filepath.FromSlash(p[1:])
	}
	return filepath.FromSlash(p)
}

// shellSpecialPath converts the shell-spelling forms recognized by the shared
// mvdan.cc/sh/v3/pathconv package — the SAME converter bashy's sh interpreter
// runs, so a path names the same file inside and outside a script: the
// MSYS/Git-Bash drive form (/c/…, also \c\… — bashy hands scripts /c/… for
// $HOME, $TEMP and pwd, and an applet that runs filepath.FromSlash on an
// operand before resolving it turns that into \c\foo; both must still mean
// C:\foo, never the drive-relative C:\c\foo), the WSL mount form (/mnt/c/…,
// forward-slash spelling only, exactly as pathconv defines it), and the two
// POSIX pseudo-operands /dev/null (-> NUL) and /tmp[/…] (-> the host temp
// directory). ok=false means p is none of those and the caller applies its
// own drive-less rule (normalizePath keeps a bare /foo drive-relative;
// toOSPath maps it onto SystemDrive).
//
// The drive form is assembled with FromSlash rather than routed through
// pathconv.ToOS wholesale: ToOS runs filepath.Clean, and RunContext.Path
// treats a trailing separator (and every ".." component a caller kept) as
// semantic, not cosmetic.
func shellSpecialPath(p string) (string, bool) {
	if drive, rest, ok := pathconv.DrivePath(p); ok {
		return string(drive) + ":" + filepath.FromSlash(rest), true
	}
	// The pseudo-operand MAPPINGS (which device, which directory) live in
	// pathconv.ToOS; only the recognition gate is local, so the drive-less
	// fallbacks above are not subjected to ToOS's volume-prepend rule.
	if p == "/dev/null" {
		return pathconv.ToOS("", p), true
	}
	if strings.HasPrefix(p, "/tmp") && (len(p) == 4 || p[4] == '/') {
		out := pathconv.ToOS("", p)
		if hasTrailingPathSeparator(p) && !hasTrailingPathSeparator(out) {
			out += `\`
		}
		return out, true
	}
	return "", false
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
