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
	return len(p) > 0 && p[0] == '/' && (len(p) < 2 || p[1] != '/')
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

func systemDrive() string {
	sd := os.Getenv("SystemDrive")
	if sd == "" {
		sd = "C:"
	}
	return sd + `\`
}

func toOSPath(p string) string {
	if len(p) > 0 && p[0] == '/' && (len(p) < 2 || p[1] != '/') {
		return systemDrive() + filepath.FromSlash(p[1:])
	}
	return normalizePath(p)
}

func fromOSPath(p string) string {
	drv := systemDrive()
	drvLen := len(drv)
	if len(p) >= drvLen && strings.EqualFold(p[:drvLen], drv) {
		if rest := p[drvLen:]; rest != "" {
			return "/" + filepath.ToSlash(rest)
		}
		return "/"
	}
	if len(p) > 0 && p[0] == '\\' && (len(p) < 2 || p[1] != '\\') {
		return "/" + filepath.ToSlash(p[1:])
	}
	return filepath.ToSlash(p)
}
