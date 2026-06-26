//go:build !windows

package tool

import "path/filepath"

func isAbsPath(p string) bool {
	return filepath.IsAbs(p)
}

func normalizePath(p string) string {
	return p
}

func pathextFromEnv(_ []string) []string {
	return nil
}

func resolveExecutable(rc *RunContext, name string) string {
	return rc.Path(name)
}
