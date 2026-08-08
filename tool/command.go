package tool

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveCommand resolves name the way a shell's execvp(3) does, against the
// invocation's environment (rc.Env) and working directory (rc.Dir) — never the
// host process's. It is the single shared PATH resolver for wrapper commands
// (xargs, nice, time, …) that run an external COMMAND operand.
//
// Semantics (POSIX execvp / shell):
//
//   - A name containing a path separator is taken relative to the working
//     directory; it is never searched in PATH. PATHEXT suffixes are tried on
//     Windows.
//   - Otherwise each PATH component is searched in order. An empty component
//     names the current directory, exactly as execvp(3) specifies, so
//     "PATH=:/bin" and "PATH=/bin:" both include the working directory.
//   - An UNSET PATH (no "PATH=" entry) falls back to the platform default
//     search path, while an explicitly empty PATH ("PATH=") is a single
//     zero-length component — the working directory only. The two are distinct.
//
// Resolution returns the first executable match. If only non-executable matches
// exist, the first is returned anyway so the caller can attempt to exec it and
// report POSIX status 126 (found but not executable) rather than 127 (not
// found); "" is returned only when no file of that name exists anywhere in the
// search path. Every directory and the current-directory element is resolved
// through rc.Path, so a relative PATH entry or a "." component is interpreted
// relative to rc.Dir, not the embedding process's working directory.
func (rc *RunContext) ResolveCommand(name string) string {
	if strings.ContainsAny(name, `/\`) {
		p := rc.ResolveExecutable(name) // rc.Path on POSIX; PATHEXT on Windows
		if pathExists(p) {
			return p
		}
		return ""
	}

	pathVal, present := envValue(rc.Env, "PATH")
	if !present {
		pathVal = defaultCommandPath()
	}
	dirs := filepath.SplitList(pathVal)
	if len(dirs) == 0 {
		// An empty PATH is one zero-length element (the working directory),
		// not "nowhere to look"; filepath.SplitList("") returns no elements.
		dirs = []string{""}
	}

	var nonExecutable string
	for _, dir := range dirs {
		if dir == "" {
			dir = "." // empty component = current directory (execvp(3))
		}
		// ResolveExecutable is rc.Path on POSIX (joins rc.Dir) and applies
		// PATHEXT on Windows, so a relative entry is relative to rc.Dir and a
		// ".bat"/".exe" sibling is found when the bare name is given.
		cand := rc.ResolveExecutable(filepath.Join(dir, name))
		if isExecutableFile(cand) {
			return cand
		}
		if nonExecutable == "" && pathExists(cand) {
			nonExecutable = cand // first found-but-not-executable match (126)
		}
	}
	return nonExecutable
}

// envValue returns the last assignment to name in environ order and whether it
// was present at all (so an unset PATH is distinguished from an empty one).
func envValue(environ []string, name string) (string, bool) {
	prefix := name + "="
	for i := len(environ) - 1; i >= 0; i-- {
		if strings.HasPrefix(environ[i], prefix) {
			return environ[i][len(prefix):], true
		}
	}
	return "", false
}

// isExecutableFile reports whether path is a regular file that is executable
// (any existing file counts as executable on Windows). It mirrors the check the
// wrapper commands applied in their private lookCommand copies.
func isExecutableFile(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	return pathIsExecutableBit(fi)
}

// pathExists reports whether any file system entry exists at path (a
// found-but-not-executable match, which yields POSIX 126 rather than 127).
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
