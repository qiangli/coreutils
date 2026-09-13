package kb

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Links become real at READ TIME. A record's body is a cache of prose, never a
// store: the graph between pages is parsed out of the markdown every time it is
// consumed, and the body is never rewritten to "materialise" a link (that is
// resolve-at-consumption — docs/kb-self-organization.md). This file is the pure
// parser + resolver both `kb backlinks`/`kb doctor` and `bashy todo show
// --links` call; it reads scope/bus/git and stdlib only, so pkg/kb stays an
// import leaf and cannot reach pkg/issue or pkg/todo.
//
// The four link spellings, and the two relative-markdown forms:
//
//	[[slug]]        a bare kb slug
//	[[kb:slug]]     an explicit kb slug
//	[[todo:<id>]]   a todo/issue id (matched by id or unique prefix, git-style)
//	[[sprint:<n>]]  a sprint card (classified, never resolved here — kb cannot
//	                see the sprint store while staying a leaf)
//	[text](pages/x.md)        → kb slug x
//	[text](../todo/<id>-….md) → todo id

// LinkKind is the namespace a parsed link points into.
type LinkKind string

const (
	LinkKB     LinkKind = "kb"
	LinkTodo   LinkKind = "todo"
	LinkSprint LinkKind = "sprint"
)

// Link is one reference parsed out of a record body.
type Link struct {
	Kind   LinkKind `json:"kind"`
	Target string   `json:"target"` // slug (kb), id (todo), number (sprint)
	Raw    string   `json:"raw"`    // the literal text matched, for reporting
}

// Ref is the canonical address a link resolves to, e.g. "kb:never-pkill".
func (l Link) Ref() string { return string(l.Kind) + ":" + l.Target }

// LinkNode is a record in the link graph — a kb page or a todo/issue — viewed as
// both a potential link source (Body) and a potential target (Ref). The page
// pointer is set for kb nodes so doctor can read form/description/status without
// a second load; todo nodes carry only the generic fields kb can read without
// importing pkg/issue.
type LinkNode struct {
	Kind  LinkKind
	ID    string // kb slug | todo id
	Title string
	Body  string

	page *Page // set for kb nodes only
}

// Ref is the canonical address another record would cite this node by.
func (n LinkNode) Ref() string { return string(n.Kind) + ":" + n.ID }

// KBNode wraps a kb page as a link-graph node.
func KBNode(p *Page) LinkNode {
	return LinkNode{Kind: LinkKB, ID: p.Slug, Title: p.Title, Body: p.Body, page: p}
}

// KBNodes wraps every page as a link-graph node.
func KBNodes(pages []*Page) []LinkNode {
	out := make([]LinkNode, 0, len(pages))
	for _, p := range pages {
		out = append(out, KBNode(p))
	}
	return out
}

// TodoNode builds a link-graph node for a todo/issue record. pkg/todo (which
// may import kb) calls this with its own issues; kb builds them itself from disk
// via TodoNodesFromDir so it never imports pkg/issue.
func TodoNode(id, title, body string) LinkNode {
	return LinkNode{Kind: LinkTodo, ID: strings.TrimSpace(id), Title: title, Body: body}
}

var (
	wikiRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	mdRe   = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
)

// ParseLinks extracts every resolvable link from a markdown body, de-duplicated
// by (kind, target) so one page citing the same page twice reports once.
func ParseLinks(body string) []Link {
	var out []Link
	seen := map[string]bool{}
	add := func(l Link, ok bool) {
		if !ok {
			return
		}
		key := string(l.Kind) + "\x00" + l.Target
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, l)
	}
	for _, m := range wikiRe.FindAllStringSubmatch(body, -1) {
		add(classifyWiki(m[1], m[0]))
	}
	for _, m := range mdRe.FindAllStringSubmatch(body, -1) {
		add(classifyRel(m[1]))
	}
	return out
}

// classifyWiki turns the inside of a [[…]] into a Link. A known scheme prefix
// (kb:/todo:/sprint:) selects the namespace; a bare token is a kb slug.
func classifyWiki(inner, raw string) (Link, bool) {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return Link{}, false
	}
	if scheme, rest, ok := strings.Cut(inner, ":"); ok {
		rest = strings.TrimSpace(rest)
		switch scheme {
		case "kb":
			return Link{Kind: LinkKB, Target: rest, Raw: raw}, rest != ""
		case "todo":
			return Link{Kind: LinkTodo, Target: strings.TrimPrefix(rest, "#"), Raw: raw}, rest != ""
		case "sprint":
			return Link{Kind: LinkSprint, Target: strings.TrimPrefix(rest, "#"), Raw: raw}, rest != ""
		}
	}
	// A bare [[token]] is a kb slug.
	return Link{Kind: LinkKB, Target: inner, Raw: raw}, true
}

// classifyRel turns a relative markdown target into a Link. Only repo-relative
// .md paths under pages/ (kb) or todo/ resolve; external URLs and anchors are
// ignored.
func classifyRel(raw string) (Link, bool) {
	u := strings.TrimSpace(raw)
	if u == "" || strings.Contains(u, "://") || strings.HasPrefix(u, "#") || strings.HasPrefix(u, "mailto:") {
		return Link{}, false
	}
	if !strings.HasSuffix(u, ".md") {
		return Link{}, false
	}
	clean := path.Clean(u)
	stem := strings.TrimSuffix(path.Base(clean), ".md")
	dir := path.Dir(clean)
	switch {
	case strings.Contains(dir, "todo"):
		// todo files are "<id>-<slug>.md"; the id is the hex prefix.
		id := stem
		if i := strings.IndexByte(stem, '-'); i > 0 {
			id = stem[:i]
		}
		return Link{Kind: LinkTodo, Target: id, Raw: raw}, id != ""
	case strings.Contains(dir, "pages") || strings.Contains(dir, "kb"):
		return Link{Kind: LinkKB, Target: stem, Raw: raw}, stem != ""
	}
	return Link{}, false
}

// matches reports whether link l points at node n. kb matches by exact slug;
// todo matches by exact id or a git-style unique prefix; sprint never matches a
// node (kb holds no sprint records).
func (l Link) matches(n LinkNode) bool {
	switch l.Kind {
	case LinkKB:
		return n.Kind == LinkKB && n.ID == l.Target
	case LinkTodo:
		return n.Kind == LinkTodo && (n.ID == l.Target || strings.HasPrefix(n.ID, l.Target))
	}
	return false
}

// ResolveLink returns the node link l points at within nodes, or (zero, false)
// when it is dangling or external (sprint).
func ResolveLink(l Link, nodes []LinkNode) (LinkNode, bool) {
	for _, n := range nodes {
		if l.matches(n) {
			return n, true
		}
	}
	return LinkNode{}, false
}

// Backlinks returns every node whose body links to target (itself excluded).
func Backlinks(target LinkNode, nodes []LinkNode) []LinkNode {
	var out []LinkNode
	for _, n := range nodes {
		if n.Ref() == target.Ref() {
			continue
		}
		for _, l := range ParseLinks(n.Body) {
			if l.matches(target) {
				out = append(out, n)
				break
			}
		}
	}
	return out
}

// ResolveOutbound splits a node's outbound links into the nodes they resolve to
// and the links that resolve to nothing (dangling or external). Order follows
// the body.
func ResolveOutbound(node LinkNode, nodes []LinkNode) (resolved []LinkNode, dangling []Link) {
	for _, l := range ParseLinks(node.Body) {
		if n, ok := ResolveLink(l, nodes); ok && n.Ref() != node.Ref() {
			resolved = append(resolved, n)
		} else {
			dangling = append(dangling, l)
		}
	}
	return resolved, dangling
}

// todoRepoSub mirrors todo.RepoSub ("docs/todo"). kb cannot import pkg/todo —
// todo imports kb for the resolver, so the reverse would cycle — hence the one
// spelled constant here, kept in step with pkg/todo.RepoSub.
const todoRepoSub = "docs/todo"

// siblingTodoDir locates the repo's docs/todo beside a repo-ring kb store, so
// backlinks and doctor see todo→kb citations. Returns "" for the host/agent
// rings or an arbitrary --dir (no enclosing repo), which callers treat as "todo
// links are not enumerable here" rather than "every todo link is dangling".
func siblingTodoDir(kbDir string) string {
	root := repoRootOf(kbDir)
	if root == "" {
		return ""
	}
	return filepath.Join(root, todoRepoSub)
}

// TodoNodesFromDir reads a directory of todo/issue markdown files into link
// nodes WITHOUT importing pkg/issue (the leaf pin): it splits the frontmatter
// for the id/title and takes the body generically. A missing directory yields
// no nodes and no error.
func TodoNodesFromDir(dir string) ([]LinkNode, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []LinkNode
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		id, title, body, ok := parseTodoRecord(e.Name(), b)
		if !ok || id == "" {
			continue
		}
		out = append(out, TodoNode(id, title, body))
	}
	return out, nil
}

// parseTodoRecord pulls the id, title and body out of a todo/issue file. The id
// comes from the frontmatter, falling back to the filename's hex prefix
// ("<id>-<slug>.md").
func parseTodoRecord(filename string, b []byte) (id, title, body string, ok bool) {
	fm, bd, fok := splitFrontmatter(b)
	if !fok {
		return "", "", "", false
	}
	var meta struct {
		ID    string `yaml:"id"`
		Title string `yaml:"title"`
	}
	_ = yaml.Unmarshal([]byte(fm), &meta)
	id = strings.TrimSpace(meta.ID)
	if id == "" {
		stem := strings.TrimSuffix(filename, ".md")
		if i := strings.IndexByte(stem, '-'); i > 0 {
			id = stem[:i]
		} else {
			id = stem
		}
	}
	return id, strings.TrimSpace(meta.Title), bd, true
}

// splitFrontmatter splits a "--- … ---" framed markdown file into its raw
// frontmatter and body, mirroring ParsePage's framing exactly.
func splitFrontmatter(b []byte) (fm, body string, ok bool) {
	s := string(b)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return "", "", false
	}
	_, rest, _ := strings.Cut(s, "\n")
	fm, body, ok = strings.Cut(rest, "\n---")
	if !ok {
		return "", "", false
	}
	body = strings.TrimPrefix(body, "\r")
	body = strings.TrimPrefix(body, "\n")
	body = strings.TrimRight(strings.TrimPrefix(body, "\n"), "\n")
	return fm, body, true
}
