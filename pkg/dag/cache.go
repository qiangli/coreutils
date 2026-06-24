// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package dag

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// Cache is the fingerprint store backing dag's incremental up-to-date skip —
// the agent-first answer to make's mtime prerequisites, but content-hashed
// (a touched-but-unchanged file does not force a rebuild). One JSON file per
// DAG document under ~/.cache/bashy/dag/, atomic tmp+rename writes.
type Cache struct {
	path   string
	Hashes map[string]string `json:"hashes"`
}

// LoadCache opens (or starts) the fingerprint cache for docPath. A read error
// yields an empty cache rather than failing — a missing/garbage cache just
// means "everything is out of date".
func LoadCache(docPath string) *Cache {
	c := &Cache{Hashes: map[string]string{}}
	abs, _ := filepath.Abs(docPath)
	dir, err := os.UserCacheDir()
	if err != nil {
		return c // no cache dir -> always-run cache
	}
	sum := sha256.Sum256([]byte(abs))
	c.path = filepath.Join(dir, "bashy", "dag", hex.EncodeToString(sum[:])+".json")
	if data, err := os.ReadFile(c.path); err == nil {
		_ = json.Unmarshal(data, c)
		if c.Hashes == nil {
			c.Hashes = map[string]string{}
		}
	}
	return c
}

// Fingerprint computes a node's content hash: its body + the hashes of its
// Sources/Inputs (file or directory, recursive) + its resolved deps'
// fingerprints (so an upstream change invalidates everything downstream).
// dir is the document directory; relative operands resolve against it. The
// deps map carries already-computed dependency fingerprints (callers walk in
// topological order, so a dep's fingerprint is ready before its dependent's).
func (c *Cache) Fingerprint(n *Node, dir string, depFPs map[string]string) string {
	h := sha256.New()
	io.WriteString(h, "body\x00"+n.Task.Body+"\x00")
	for _, d := range n.Deps {
		io.WriteString(h, "dep\x00"+d.Task.Name+"\x00"+depFPs[d.Task.Name]+"\x00")
	}
	paths := append(append([]string{}, n.Task.Sources...), n.Task.Inputs...)
	sort.Strings(paths)
	for _, p := range paths {
		io.WriteString(h, "src\x00"+p+"\x00"+hashPath(filepath.Join(dir, p))+"\x00")
	}
	return hex.EncodeToString(h.Sum(nil))
}

// UpToDate reports whether n can be skipped: it declares Generates, all of them
// exist, and its recorded fingerprint matches fp. A target with no Generates is
// never up-to-date (it is effectively phony — like make's no-output targets).
func (c *Cache) UpToDate(n *Node, dir, fp string) bool {
	if len(n.Task.Generates) == 0 {
		return false
	}
	if c.Hashes[n.Task.Name] != fp {
		return false
	}
	for _, g := range n.Task.Generates {
		if _, err := os.Stat(filepath.Join(dir, g)); err != nil {
			return false
		}
	}
	return true
}

// Record stores a node's fingerprint after a successful run.
func (c *Cache) Record(name, fp string) {
	if c.Hashes == nil {
		c.Hashes = map[string]string{}
	}
	c.Hashes[name] = fp
}

// Save atomically persists the cache. Best-effort: a write failure is ignored
// (the next run just recomputes).
func (c *Cache) Save() {
	if c.path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(c.path), 0o755)
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, c.path)
}

// hashPath hashes a file's content, or a directory's recursive file contents.
// A missing path hashes to a stable sentinel (so "absent" differs from "empty"
// and re-appearing the file invalidates).
func hashPath(p string) string {
	fi, err := os.Stat(p)
	if err != nil {
		return "absent"
	}
	h := sha256.New()
	if !fi.IsDir() {
		hashFile(h, p)
		return hex.EncodeToString(h.Sum(nil))
	}
	var files []string
	_ = filepath.Walk(p, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	for _, f := range files {
		io.WriteString(h, f+"\x00")
		hashFile(h, f)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func hashFile(h io.Writer, p string) {
	f, err := os.Open(p)
	if err != nil {
		io.WriteString(h, "err\x00")
		return
	}
	defer f.Close()
	_, _ = io.Copy(h, f)
	io.WriteString(h, "\x00")
}
