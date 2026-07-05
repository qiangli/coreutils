package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Source is one ring's skill supply.
type Source interface {
	Ring() Ring
	Names() ([]string, error)
	Body(name string) ([]byte, bool)      // SKILL.md
	File(name, rel string) ([]byte, bool) // reference.md, skill.dhnt, …
	Files(name string) ([]string, error)  // every file in the skill folder (rel paths)
}

// EmbedSource wraps an fs.FS whose top-level directories are skills
// (the shape of bashy's embedded skills FS).
func EmbedSource(fsys fs.FS, ring Ring) Source { return fsSource{fsys: fsys, ring: ring} }

type fsSource struct {
	fsys fs.FS
	ring Ring
}

func (s fsSource) Ring() Ring { return s.ring }

func (s fsSource) Names() ([]string, error) {
	entries, err := fs.ReadDir(s.fsys, ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if _, err := fs.Stat(s.fsys, e.Name()+"/SKILL.md"); err == nil {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func (s fsSource) Body(name string) ([]byte, bool) { return s.File(name, "SKILL.md") }

func (s fsSource) Files(name string) ([]string, error) {
	name = strings.Trim(name, "/")
	if name == "" || strings.Contains(name, "/") {
		return nil, fmt.Errorf("skills: bad skill name %q", name)
	}
	var out []string
	err := fs.WalkDir(s.fsys, name, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(name, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func (s fsSource) File(name, rel string) ([]byte, bool) {
	name = strings.Trim(name, "/")
	if name == "" || strings.Contains(name, "/") || strings.Contains(rel, "..") {
		return nil, false
	}
	data, err := fs.ReadFile(s.fsys, name+"/"+rel)
	if err != nil {
		return nil, false
	}
	return data, true
}

// DirSource serves the host-local ring from a directory of skill
// folders. A missing directory is an empty source, not an error.
func DirSource(dir string) Source { return dirSource{dir: dir, ring: RingLocal} }

// SharedDirSource serves a shared catalog directory — any git clone or
// synced folder of skill folders — as a read-only ring between embedded
// and local. This is the standalone sharing path ("cloud as a thin
// replaceable relay"): a team's skills repo cloned to disk IS a catalog;
// no control plane required. Local installs/learning still shadow it.
func SharedDirSource(dir string) Source { return dirSource{dir: dir, ring: RingShared} }

type dirSource struct {
	dir  string
	ring Ring
}

func (s dirSource) Ring() Ring { return s.ring }

func (s dirSource) Names() ([]string, error) {
	if _, err := os.Stat(s.dir); err != nil {
		return nil, nil
	}
	return fsSource{fsys: os.DirFS(s.dir), ring: s.ring}.Names()
}

func (s dirSource) Body(name string) ([]byte, bool) { return s.File(name, "SKILL.md") }

func (s dirSource) File(name, rel string) ([]byte, bool) {
	if _, err := os.Stat(s.dir); err != nil {
		return nil, false
	}
	return fsSource{fsys: os.DirFS(s.dir), ring: s.ring}.File(name, rel)
}

func (s dirSource) Files(name string) ([]string, error) {
	if _, err := os.Stat(s.dir); err != nil {
		return nil, nil
	}
	return fsSource{fsys: os.DirFS(s.dir), ring: s.ring}.Files(name)
}

// Listing is one catalog row: the skill plus its applicability verdict
// at this host's coordinate.
type Listing struct {
	Skill
	Verdict Verdict
	Shadows bool // this row hides a same-named skill in a lower ring
	Warning string
}

// Catalog merges sources; on a name collision the LAST source wins
// (ring order: embedded first, local last — local shadows embedded).
type Catalog struct{ Sources []Source }

// Get returns a skill's parsed entry and its winning source.
func (c *Catalog) Get(name string) (Skill, Source, bool) {
	for i := len(c.Sources) - 1; i >= 0; i-- {
		src := c.Sources[i]
		body, ok := src.Body(name)
		if !ok {
			continue
		}
		return c.parse(name, body, src), src, true
	}
	return Skill{}, nil, false
}

// List returns every row with verdicts attached. Filtering is the
// caller's business — `--all`, JSON views, and future consumers choose
// their own cut.
func (c *Catalog) List(ps *ProbeSet) ([]Listing, error) {
	byName := map[string]int{}
	var rows []Listing
	for _, src := range c.Sources {
		names, err := src.Names()
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			body, ok := src.Body(n)
			if !ok {
				continue
			}
			row := Listing{Skill: c.parse(n, body, src)}
			if i, dup := byName[row.Name]; dup {
				row.Shadows = true
				rows[i] = row
				continue
			}
			byName[row.Name] = len(rows)
			rows = append(rows, row)
		}
	}
	for i := range rows {
		rows[i].Verdict = verdictOf(rows[i].Skill, ps)
		if rows[i].RequiresErr != "" {
			rows[i].Warning = "requires unparsable: " + rows[i].RequiresErr
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows, nil
}

func (c *Catalog) parse(dirName string, body []byte, src Source) Skill {
	sk, err := ParseFrontmatter(body)
	if err != nil {
		// Degrade, never hide: directory name, no description, rung-0.
		sk = Skill{RequiresErr: err.Error(), Meta: map[string]string{}}
	}
	if sk.Name == "" {
		sk.Name = dirName
	}
	sk.Ring = src.Ring()
	if ds, ok := src.(dirSource); ok {
		sk.Dir = filepath.Join(ds.dir, dirName)
	}
	if canon, ok := src.File(dirName, "skill.dhnt"); ok {
		sk.HasDhnt = true
		sk.Dhnt = parseDhntInfo(canon)
	}
	return sk
}

// verdictOf applies the pinned gating table: structured requires gate
// exactly; compatibility free text alone flags but never filters; no
// info at all is applicable (rung-0 permissive).
func verdictOf(sk Skill, ps *ProbeSet) Verdict {
	if sk.Requires != nil {
		return sk.Requires.Eval(ps)
	}
	v := Verdict{Applicable: true}
	if sk.Compatibility != "" || sk.RequiresErr != "" {
		v.Unchecked = sk.Compatibility
		if v.Unchecked == "" {
			v.Unchecked = "requires unparsable"
		}
	}
	return v
}

// KeyProbes returns the probe names a skill's context key is computed
// over: {os, arch} ∪ the requires-referenced subset (pinned rule).
func KeyProbes(sk Skill) []string {
	names := []string{"os", "arch"}
	if sk.Requires != nil {
		for _, p := range sk.Requires.ProbeRefs() {
			if p != "os" && p != "arch" {
				names = append(names, p)
			}
		}
	}
	return names
}
