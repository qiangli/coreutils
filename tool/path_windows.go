//go:build windows

package tool

import (
	"os"
	"path/filepath"
	"strings"
)

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
// tool entry point (RunContext.Path resolves operands through it), so the
// MSYS/Git-Bash drive convention is honored here: /c/foo -> C:\foo, a bare
// drive-less /foo -> <SystemDrive>\foo, everything else slash-converted.
func normalizePath(p string) string {
	if drive, rest, ok := msysDriveSplit(p); ok {
		return drive + ":" + filepath.FromSlash(rest)
	}
	if len(p) > 0 && p[0] == '/' && (len(p) < 2 || p[1] != '/') {
		return systemDrive() + filepath.FromSlash(p[1:])
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

func systemDrive() string {
	sd := os.Getenv("SystemDrive")
	if sd == "" {
		sd = "C:"
	}
	return sd + `\`
}

// toOSPath converts a shell path to a Windows path (now identical to
// normalizePath, which honors the MSYS drive convention).
func toOSPath(p string) string { return normalizePath(p) }

// msysDriveSplit recognizes the MSYS/Git-Bash drive convention "/c" or "/c/...":
// a leading slash, one ASCII letter, then end-of-string or another slash. It
// returns the UPPERCASE drive letter and the remainder beginning with a slash
// ("/c" -> "C","/"; "/c/Users" -> "C","/Users"). This is the standard way every
// Windows dev tool spells C:\ as a POSIX path, so a node's scripts are portable.
func msysDriveSplit(p string) (drive, rest string, ok bool) {
	if len(p) >= 2 && p[0] == '/' && isASCIILetter(p[1]) && (len(p) == 2 || p[2] == '/') {
		r := p[2:]
		if r == "" {
			r = "/"
		}
		return string(p[1] &^ 0x20), r, true // &^0x20 = ASCII upper
	}
	return "", "", false
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func fromOSPath(p string) string {
	// C:\foo -> /c/foo (any drive letter; reverse of the MSYS convention).
	if len(p) >= 2 && isASCIILetter(p[0]) && p[1] == ':' {
		return "/" + string(p[0]|0x20) + filepath.ToSlash(p[2:]) // |0x20 = ASCII lower
	}
	if len(p) > 0 && p[0] == '\\' && (len(p) < 2 || p[1] != '\\') {
		return "/" + filepath.ToSlash(p[1:])
	}
	return filepath.ToSlash(p)
}
