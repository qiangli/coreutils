// Local search (P0b) — the other half of the find-things primitive. A unifying
// FRONT over what bashy already has: a content/filename scan (grep+find, no new
// index — a scan is not an index) and kb facts (the kb Go API). ast (treesitter)
// and graph are a documented follow-up; an agent can call those verbs directly
// today.
package search

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/pkg/kb"
	"github.com/qiangli/coreutils/pkg/scope"
)

// LocalResult is one local hit, in a uniform shape across domains.
type LocalResult struct {
	Kind string `json:"kind"` // "content" | "file" | "kb"
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
	Text string `json:"text,omitempty"`
}

// LocalOptions configures a local search.
type LocalOptions struct {
	Dir        string // root to scan (default: cwd)
	MaxResults int    // default 40
	Domain     string // "" | "content" | "files" | "kb" ; "" = content + kb
}

// skipDirs are never descended into.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".bashy": true,
	"dist": true, "build": true, "target": true, ".cache": true, ".venv": true,
}

// Local runs a local search and returns unified results.
func Local(query string, opt LocalOptions) ([]LocalResult, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, errors.New("search: empty local query")
	}
	max := opt.MaxResults
	if max <= 0 {
		max = 40
	}
	dir := opt.Dir
	if strings.TrimSpace(dir) == "" {
		dir, _ = os.Getwd()
	}

	var out []LocalResult
	switch strings.ToLower(strings.TrimSpace(opt.Domain)) {
	case "kb":
		return searchKB(q, max)
	case "files":
		return scanTree(dir, q, max, true)
	case "content":
		return scanTree(dir, q, max, false)
	default: // content + kb
		out, _ = scanTree(dir, q, max, false)
		if len(out) < max {
			if kbHits, _ := searchKB(q, max-len(out)); len(kbHits) > 0 {
				out = append(out, kbHits...)
			}
		}
		return out, nil
	}
}

// scanTree walks dir matching file CONTENT (or file NAMES when byName), skipping
// the usual noise and binary files. Case-insensitive substring — a scan, not a
// regex index.
func scanTree(dir, query string, max int, byName bool) ([]LocalResult, error) {
	needle := strings.ToLower(query)
	var out []LocalResult
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || len(out) >= max {
			if len(out) >= max {
				return filepath.SkipAll
			}
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if byName {
			if strings.Contains(strings.ToLower(d.Name()), needle) {
				out = append(out, LocalResult{Kind: "file", Path: rel(dir, path)})
			}
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil || info.Size() > 2<<20 { // skip files > 2 MB
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil || bytes.IndexByte(data[:min(len(data), 512)], 0) >= 0 {
			return nil // unreadable or binary
		}
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		ln := 0
		for sc.Scan() {
			ln++
			line := sc.Text()
			if strings.Contains(strings.ToLower(line), needle) {
				out = append(out, LocalResult{Kind: "content", Path: rel(dir, path), Line: ln, Text: strings.TrimSpace(line)})
				if len(out) >= max {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	return out, err
}

// searchKB runs the kb term search over the active scope's pages.
func searchKB(query string, max int) ([]LocalResult, error) {
	sc, err := scope.Resolve(scope.Options{
		RepoSub: kb.RepoSub,
		HostDir: func() (string, error) { return kb.DefaultDir(), nil },
	})
	if err != nil {
		return nil, nil // no scope resolvable — kb simply contributes nothing
	}
	store := kb.Open(sc.Dir())
	pages, err := store.List()
	if err != nil || len(pages) == 0 {
		return nil, nil
	}
	hits := kb.Search(pages, kb.Query{Terms: kb.Terms(query), K: max})
	out := make([]LocalResult, 0, len(hits))
	for _, h := range hits {
		out = append(out, LocalResult{Kind: "kb", Path: h.Page.Slug + " — " + h.Page.Title})
	}
	return out, nil
}

func rel(base, path string) string {
	if r, err := filepath.Rel(base, path); err == nil {
		return r
	}
	return path
}
