// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package dag

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/qiangli/coreutils/pkg/weavecli"
)

// Task is one target parsed from the markdown source.
type Task struct {
	Name string
	Desc string
	Lang string // interpreter tag from the fenced-code info string ("" => bash)
	Body string

	Requires  []string
	Inputs    []string
	Sources   []string
	Generates []string
	Env       []string // per-target KEY=VALUE overrides (make's target-specific vars)

	// P2 (parsed-and-ignored in P1, so a contract-bearing file parses today).
	Ensure  []string
	Effects []string

	Line int // 1-based source line of the heading, for error messages
}

// Document is a parsed DAG markdown file.
type Document struct {
	Path     string
	Name     string   // optional file-level frontmatter `name`
	Desc     string   // optional file-level frontmatter `description`
	Default  string   // optional frontmatter `default` — make's .DEFAULT_GOAL
	Includes []string // optional frontmatter `include` — files merged in (make's `include`)
	Tasks    []*Task
	Order    []string // task names in declaration order (deterministic listing)

	byName map[string]*Task
}

// Lookup returns the named task and whether it exists.
func (d *Document) Lookup(name string) (*Task, bool) {
	t, ok := d.byName[name]
	return t, ok
}

var metaKeys = map[string]bool{
	"requires": true, "inputs": true, "sources": true,
	"generates": true, "env": true, "ensure": true, "effects": true,
}

// Parse reads a DAG markdown document. The format:
//   - Optional YAML frontmatter (`---` … `---`) with `name`/`description`.
//   - If a `## Tasks` heading exists, targets are its `### name` children;
//     otherwise targets are top-level `## name` headings.
//   - Under each target heading: prose description, metadata lines
//     (`Requires:`/`Inputs:`/`Sources:`/`Generates:`/`Ensure:`/`Effects:` —
//     only these keys are metadata, every other line is description), and a
//     fenced code block whose info string selects the interpreter (default bash).
func Parse(r io.Reader, path string) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, errf(weavecli.ExitInvalidArg, "read %s: %v", path, err)
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	doc := &Document{Path: path, byName: map[string]*Task{}}

	start := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		j := 1
		for j < len(lines) && strings.TrimSpace(lines[j]) != "---" {
			if k, v, ok := frontmatterKV(lines[j]); ok {
				switch k {
				case "name":
					doc.Name = v
				case "description":
					doc.Desc = v
				case "default", "default_goal":
					doc.Default = v
				case "include", "includes":
					doc.Includes = append(doc.Includes, splitList(v)...)
				}
			}
			j++
		}
		if j < len(lines) {
			j++ // consume closing ---
		}
		start = j
	}

	nested := false
	for _, ln := range lines[start:] {
		if lvl, text, ok := parseHeading(ln); ok && lvl == 2 && strings.EqualFold(text, "Tasks") {
			nested = true
			break
		}
	}
	taskLevel := 2
	if nested {
		taskLevel = 3
	}

	var (
		cur      *Task
		curLines []string
		inTasks  = !nested
		perr     *Error
	)
	flush := func() {
		if cur == nil {
			return
		}
		cur.absorb(curLines)
		if _, dup := doc.byName[cur.Name]; dup {
			if perr == nil {
				perr = errf(weavecli.ExitInvalidArg, "duplicate target %q (line %d)", cur.Name, cur.Line)
			}
		} else {
			doc.byName[cur.Name] = cur
			doc.Tasks = append(doc.Tasks, cur)
			doc.Order = append(doc.Order, cur.Name)
		}
		cur, curLines = nil, nil
	}

	for idx := start; idx < len(lines); idx++ {
		ln := lines[idx]
		if lvl, text, ok := parseHeading(ln); ok {
			if nested {
				if lvl == 2 && strings.EqualFold(text, "Tasks") {
					flush()
					inTasks = true
					continue
				}
				if lvl <= 2 {
					flush()
					inTasks = false
					continue
				}
			}
			if lvl == taskLevel && inTasks {
				flush()
				cur = &Task{Name: text, Line: idx + 1}
				continue
			}
			if !nested && lvl <= taskLevel {
				flush() // a level-1 title (or sibling level-2) ends the current target
				continue
			}
			if cur != nil {
				curLines = append(curLines, ln) // deeper heading inside a body region
			}
			continue
		}
		if cur != nil {
			curLines = append(curLines, ln)
		}
	}
	flush()

	if perr != nil {
		return nil, perr
	}
	return doc, nil
}

// ParseFile parses the DAG markdown at path, resolving any frontmatter
// `include:` files (paths relative to the including file). Like make's
// `include`, included targets are merged in; the including file (and an
// earlier-listed include) wins a name collision. A file is parsed at most once,
// so diamonds and include cycles terminate safely.
func ParseFile(path string) (*Document, error) {
	return parseFileSeen(path, map[string]bool{})
}

func parseFileSeen(path string, seen map[string]bool) (*Document, error) {
	abs, _ := filepath.Abs(path)
	seen[abs] = true
	f, err := os.Open(path)
	if err != nil {
		return nil, errf(weavecli.ExitInvalidArg, "%v", err)
	}
	doc, err := Parse(f, path)
	f.Close()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	for _, inc := range doc.Includes {
		incPath := filepath.Join(dir, inc)
		if cabs, _ := filepath.Abs(incPath); seen[cabs] {
			continue // already merged (dedupe + cycle break)
		}
		child, err := parseFileSeen(incPath, seen)
		if err != nil {
			return nil, err
		}
		doc.mergeFrom(child)
	}
	return doc, nil
}

// mergeFrom adds child's targets that aren't already defined (this document and
// earlier includes win name collisions).
func (d *Document) mergeFrom(child *Document) {
	for _, name := range child.Order {
		if _, exists := d.byName[name]; exists {
			continue
		}
		t := child.byName[name]
		d.byName[name] = t
		d.Tasks = append(d.Tasks, t)
		d.Order = append(d.Order, name)
	}
}

// Discover locates the task file in dir: DAG.md if present, else the first
// (lexical) *.md containing a `## Tasks` section.
func Discover(dir string) (string, error) {
	if fi, err := os.Stat(filepath.Join(dir, "DAG.md")); err == nil && !fi.IsDir() {
		return filepath.Join(dir, "DAG.md"), nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", errf(weavecli.ExitInvalidArg, "%v", err)
	}
	var mds []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			mds = append(mds, e.Name())
		}
	}
	sort.Strings(mds)
	for _, name := range mds {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if hasTasksSection(data) {
			return filepath.Join(dir, name), nil
		}
	}
	return "", errf(weavecli.ExitInvalidArg,
		"no DAG.md or *.md with a '## Tasks' section in %s", dir)
}

func hasTasksSection(data []byte) bool {
	for _, ln := range strings.Split(string(data), "\n") {
		if lvl, text, ok := parseHeading(ln); ok && lvl == 2 && strings.EqualFold(text, "Tasks") {
			return true
		}
	}
	return false
}

// absorb splits a target's raw lines into description, metadata, and body.
func (t *Task) absorb(lines []string) {
	var desc, body []string
	inBody, bodyDone := false, false
	fenceCh := ""
	for _, ln := range lines {
		if bodyDone {
			break
		}
		if inBody {
			if strings.TrimSpace(ln) == fenceCh {
				inBody, bodyDone = false, true
				continue
			}
			body = append(body, ln)
			continue
		}
		if ch, _, ok := fenceOpen(ln); ok {
			inBody, fenceCh = true, ch
			_, lang, _ := fenceOpen(ln)
			t.Lang = lang
			continue
		}
		if k, v, ok := parseMeta(ln); ok {
			switch k {
			case "requires":
				t.Requires = append(t.Requires, splitList(v)...)
			case "inputs":
				t.Inputs = append(t.Inputs, splitList(v)...)
			case "sources":
				t.Sources = append(t.Sources, splitList(v)...)
			case "generates":
				t.Generates = append(t.Generates, splitList(v)...)
			case "env":
				t.Env = append(t.Env, splitList(v)...)
			case "ensure":
				if s := strings.TrimSpace(v); s != "" {
					t.Ensure = append(t.Ensure, s)
				}
			case "effects":
				t.Effects = append(t.Effects, splitList(v)...)
			}
			continue
		}
		desc = append(desc, ln)
	}
	t.Body = strings.Join(body, "\n")
	if t.Body != "" {
		t.Body += "\n"
	}
	t.Desc = strings.TrimSpace(strings.Join(desc, "\n"))
}

// parseHeading parses an ATX heading ("##  Name"), returning the level (1-6)
// and trimmed text. A `#` run must be followed by a space and non-empty text.
func parseHeading(line string) (level int, text string, ok bool) {
	s := strings.TrimRight(line, " \t")
	i := 0
	for i < len(s) && s[i] == '#' {
		i++
	}
	if i == 0 || i > 6 || i >= len(s) || s[i] != ' ' {
		return 0, "", false
	}
	text = strings.TrimSpace(s[i:])
	if text == "" {
		return 0, "", false
	}
	return i, text, true
}

// fenceOpen reports an opening code fence, the fence marker ("```" or "~~~"),
// and the trimmed info string (interpreter tag).
func fenceOpen(line string) (marker, info string, ok bool) {
	s := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(s, "```"):
		return "```", strings.TrimSpace(strings.TrimPrefix(s, "```")), true
	case strings.HasPrefix(s, "~~~"):
		return "~~~", strings.TrimSpace(strings.TrimPrefix(s, "~~~")), true
	}
	return "", "", false
}

// parseMeta recognizes a metadata line "Key: value" where Key is one of the
// known metadata keys (case-insensitive). Non-metadata "x: y" prose lines are
// left for the description, which avoids treating a prose colon as metadata.
func parseMeta(line string) (key, value string, ok bool) {
	i := strings.IndexByte(line, ':')
	if i <= 0 {
		return "", "", false
	}
	k := strings.ToLower(strings.TrimSpace(line[:i]))
	if !metaKeys[k] {
		return "", "", false
	}
	return k, strings.TrimSpace(line[i+1:]), true
}

func frontmatterKV(line string) (key, value string, ok bool) {
	i := strings.IndexByte(line, ':')
	if i <= 0 {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(line[:i])),
		strings.Trim(strings.TrimSpace(line[i+1:]), `"'`), true
}

func splitList(v string) []string {
	return strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
}
