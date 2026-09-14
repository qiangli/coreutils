package kb

// The ref resolver: kb answers `kb:<slug>` for the uniform addressing grammar
// (pkg/ref). Registration is one call the embedding shell makes; kb keeps
// resolving only its own kind, exactly as the leaf pin (TestKBIsALeaf) requires.
//
// The lookup mirrors `kb show`: the rings the caller can see, in the order kb
// reads them — the repo ring of the cwd first, then the host store. A superseded
// page still resolves (a ref is stable for the record's life, ref design D6):
// its Status is "superseded" and Node.Successor points forward to the page that
// replaced it, the pair supersede records (see publishSuperseded in bus.go).

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qiangli/coreutils/pkg/ref"
)

// kbRing is one store the resolver consults, with the short name that names it
// in a Node's Where ("repo" | "host" | "dir").
type kbRing struct {
	name  string
	store *Store
}

// RegisterRefs installs the kb resolver on g. dir, when non-empty, pins the
// store to that one directory (the CLI's --dir); empty resolves the way
// `kb show` auto-detects — the repo ring of the cwd first, then the host store.
func RegisterRefs(g *ref.Registry, dir string) {
	g.Register(ref.KB, ref.ResolverFunc(func(id string) (ref.Node, error) {
		return resolveKB(dir, id)
	}))
}

// resolveKB looks a slug up across the visible rings in order. A ring where the
// page is simply absent is skipped; a ring that cannot be read is a hard error,
// never ErrNotFound — "not found" is a fact about the record, "cannot read" is a
// fact about this host, and the two must stay distinct.
func resolveKB(dir, slug string) (ref.Node, error) {
	for _, r := range kbRings(dir) {
		p, err := r.store.Load(slug)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue // absent in this ring — try the next
			}
			return ref.Node{}, fmt.Errorf("kb: read %s: %w", r.store.Dir(), err)
		}
		return kbNode(p, r), nil
	}
	return ref.Node{}, fmt.Errorf("kb:%s: %w", slug, ref.ErrNotFound)
}

// kbRings returns the stores to consult, in read order. An explicit dir pins one
// store; otherwise the repo ring of the cwd (when in a git repo) comes first,
// then the host store.
func kbRings(dir string) []kbRing {
	if dir != "" {
		return []kbRing{{name: "dir", store: Open(dir)}}
	}
	var rings []kbRing
	if cwd, err := os.Getwd(); err == nil {
		if root := repoRootOf(cwd); root != "" {
			rings = append(rings, kbRing{name: "repo", store: Open(filepath.Join(root, RepoSub))})
		}
	}
	rings = append(rings, kbRing{name: "host", store: Open(DefaultDir())})
	return rings
}

// kbNode renders a page as a ref.Node. A superseded page carries its Successor —
// the ref of the page that replaced it — so a reader that resolves a stale ref is
// pointed forward rather than left believing an invalidated fact.
func kbNode(p *Page, r kbRing) ref.Node {
	n := ref.NewNode(ref.KB, p.Slug)
	n.Title = p.Title
	n.Status = p.Status
	n.Where = r.name + " " + r.store.Dir()
	n.Open = "bashy kb show " + p.Slug
	if p.Status == StatusSuperseded && p.SupersededBy != "" {
		n.Successor = ref.Format(ref.KB, p.SupersededBy)
	}
	return n
}
