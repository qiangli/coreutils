// Package ignore is the opt-in "agentic" path filter shared by grep and find:
// under the --agentic flag they skip well-known noise directories and paths
// matched by the repo's .gitignore, so a recursive search over a codebase does
// not drag an agent through node_modules/.git/vendor/dist. It is OPT-IN only —
// default search behavior is never changed, and results under --agentic are a
// clearly-announced subset of the classic results (see each tool's transparency
// line). The exact-name noise-dir floor never hides a real source file; the
// .gitignore layer adds project-specific fidelity via a vetted matcher.
package ignore

import (
	"os"
	"path/filepath"

	gitignore "github.com/sabhiram/go-gitignore"
)

// noiseDirs are directory basenames almost always excluded from a codebase
// search: VCS metadata, dependency trees, build outputs, caches. Exact-name
// matched, so a real source file is never hidden by this floor.
var noiseDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"build": true, "target": true, ".venv": true, "venv": true,
	"__pycache__": true, ".cache": true, ".next": true, ".tox": true,
	".mypy_cache": true, ".pytest_cache": true, ".gradle": true,
	".idea": true, ".vscode": true, ".terraform": true,
}

// Matcher decides whether a path should be skipped under --agentic.
// A nil *Matcher skips nothing, so callers can hold one unconditionally.
type Matcher struct {
	gi     *gitignore.GitIgnore // root .gitignore, may be nil
	root   string               // repo root (abs) for relative matching
	hidden int                  // count of entries skipped
}

// New builds a Matcher for an absolute start directory: it locates the git repo
// root (nearest ancestor containing .git) and compiles that root's .gitignore if
// present. The noise-dir denylist always applies, repo or not.
func New(dir string) *Matcher {
	m := &Matcher{}
	if root, ok := repoRoot(dir); ok {
		m.root = root
		if gi, err := gitignore.CompileIgnoreFile(filepath.Join(root, ".gitignore")); err == nil {
			m.gi = gi
		}
	}
	return m
}

// Skip reports whether the entry at absolute path (a directory when isDir)
// should be excluded, counting each exclusion. Callers prune directories
// (filepath.SkipDir) and drop files when this returns true. Safe to call on a
// nil Matcher (returns false).
func (m *Matcher) Skip(path string, isDir bool) bool {
	if m == nil {
		return false
	}
	if isDir && noiseDirs[filepath.Base(path)] {
		m.hidden++
		return true
	}
	if m.gi != nil && m.root != "" {
		if rel, err := filepath.Rel(m.root, path); err == nil && rel != "." && m.gi.MatchesPath(rel) {
			m.hidden++
			return true
		}
	}
	return false
}

// Hidden returns how many entries were skipped (for the transparency line).
func (m *Matcher) Hidden() int {
	if m == nil {
		return 0
	}
	return m.hidden
}

// repoRoot returns the nearest ancestor of dir that contains a .git entry.
func repoRoot(dir string) (string, bool) {
	d := dir
	for {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", false
		}
		d = parent
	}
}
