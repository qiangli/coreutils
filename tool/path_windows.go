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
	// On Windows, /foo is drive-relative: every Windows API (GetFullPathName,
	// CreateFileW, etc.) treats it as root-on-current-drive, e.g. C:\foo.
	// Go's filepath.IsAbs returns false for these by design (POSIX semantics).
	// We accept them as absolute so that rc.Path("/foo") returns \foo rather
	// than incorrectly joining it with the working directory.
	return len(p) > 0 && p[0] == '/' && p[1] != '/'
}

func normalizePath(p string) string {
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
	// If name already carries an extension, try the exact path.
	if filepath.Ext(name) != "" {
		return p
	}
	exts := pathextFromEnv(rc.Env)
	for _, e := range exts {
		if fp := p + e; isRegularFile(fp) {
			return fp
		}
	}
	return p
}

func isRegularFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}
