//go:build !windows

package tool

import (
	"path/filepath"
	"runtime"
)

// pathLengthLimit is the longest path string, in bytes and excluding
// the terminating NUL, that a single system call accepts (PATH_MAX - 1;
// see RunContext.Path for why it matters). Under-estimating merely
// switches an over-limit join to the equivalent relative form earlier;
// over-estimating would leave resolvable operands failing with
// ENAMETOOLONG — so unknown platforms get the conservative value.
var pathLengthLimit = func() int {
	switch runtime.GOOS {
	case "linux":
		return 4095 // PATH_MAX 4096, including the NUL
	default:
		return 1023 // darwin/BSDs 1024 including the NUL; AIX 1023 without
	}
}()

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

func toOSPath(p string) string { return p }

func fromOSPath(p string) string { return p }

func systemDrive() string { return "/" }
