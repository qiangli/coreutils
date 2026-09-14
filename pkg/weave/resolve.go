package weave

// The ref resolver: weave answers `sprint:<n>` and `run:<repo-basename>-<n>` for
// the uniform addressing grammar (pkg/ref). Registration is one call the
// embedding shell makes; weave keeps resolving only its own kinds.
//
// A sprint card lives in the one global sprint store; a run lives in a per-repo
// weave queue, and the same run id (an id is queue-local) can exist in two
// checkouts that share a repo basename. So a run reference resolves only when it
// is UNAMBIGUOUS: the current checkout's queue first, then every queue on the
// machine (weave list --all's set). Exactly one match resolves; two is an error
// that names both repo paths and refuses to pick; none is ErrNotFound. Conflating
// "absent" with "cannot read" is the one thing the ref contract forbids.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/pkg/ref"
)

// RegisterRefs installs the weave resolvers on g: the sprint card and the weave
// run. Both read their stores from the environment/home the weave CLI uses, so
// there are no store options to pass.
func RegisterRefs(g *ref.Registry) {
	g.Register(ref.Sprint, ref.ResolverFunc(resolveSprint))
	g.Register(ref.Run, ref.ResolverFunc(resolveRun))
}

// resolveSprint answers sprint:<n> from the global sprint store.
func resolveSprint(id string) (ref.Node, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
	if err != nil {
		return ref.Node{}, fmt.Errorf("sprint: %q is not a sprint number", id)
	}
	dir, err := sprintStoreDir()
	if err != nil {
		return ref.Node{}, err
	}
	q, err := loadWeaveQueue(dir)
	if err != nil {
		return ref.Node{}, fmt.Errorf("weave: read sprint store %s: %w", dir, err)
	}
	s := findWeaveStory(q, n)
	if s == nil {
		return ref.Node{}, fmt.Errorf("sprint:%s: %w", id, ref.ErrNotFound)
	}
	node := ref.NewNode(ref.Sprint, strconv.FormatInt(s.ID, 10))
	node.Title = s.Title
	node.Status = s.Column // backlog|doing|done — the sprint's stage word
	node.Where = dir
	node.Open = fmt.Sprintf("bashy sprint show %d", s.ID)
	return node, nil
}

// runHit is one queue that carries the requested run.
type runHit struct {
	repo string
	item *weaveItem
}

// resolveRun answers run:<repo-basename>-<n>. It searches the current checkout's
// queue first, then every queue on the machine, deduped by directory, and
// resolves only when exactly one queue with the named basename carries run n.
func resolveRun(id string) (ref.Node, error) {
	base, n, err := parseRunID(id)
	if err != nil {
		return ref.Node{}, err
	}
	var hits []runHit
	for _, dir := range runQueueDirs() {
		q, err := loadWeaveQueue(dir)
		if err != nil {
			return ref.Node{}, fmt.Errorf("weave: read queue %s: %w", dir, err)
		}
		root := strings.TrimSpace(q.Root)
		if root == "" {
			continue // a queue with no root cannot be named by basename
		}
		if filepath.Base(filepath.Clean(root)) != base {
			continue
		}
		if it := findWeaveItem(q, n); it != nil {
			hits = append(hits, runHit{repo: root, item: it})
		}
	}
	switch len(hits) {
	case 0:
		return ref.Node{}, fmt.Errorf("run:%s: %w", id, ref.ErrNotFound)
	case 1:
		node := ref.NewNode(ref.Run, fmt.Sprintf("%s-%d", base, n))
		node.Title = hits[0].item.Title
		node.Status = hits[0].item.State
		node.Where = hits[0].repo
		node.Open = fmt.Sprintf("bashy weave status %d", hits[0].item.ID)
		return node, nil
	default:
		paths := make([]string, 0, len(hits))
		for _, h := range hits {
			paths = append(paths, h.repo)
		}
		return ref.Node{}, fmt.Errorf("run:%s is ambiguous — %d queues share basename %q: %s; cd into the repo you mean",
			id, len(hits), base, strings.Join(paths, ", "))
	}
}

// runQueueDirs lists the queue directories to consult, current checkout first,
// then every queue on the machine — deduped, since the machine-wide scan already
// enumerates the current queue when it exists on disk.
func runQueueDirs() []string {
	seen := map[string]bool{}
	var dirs []string
	add := func(d string) {
		if d != "" && !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if root, err := weaveRepoRoot(cwd); err == nil {
			if qd, err := weaveQueueDir(root); err == nil {
				add(qd)
			}
		}
	}
	for _, d := range weaveAllQueueDirs() {
		add(d)
	}
	return dirs
}

// parseRunID splits <repo-basename>-<n> on the LAST dash — a repo basename may
// itself contain dashes, but the run number never does.
func parseRunID(id string) (base string, n int64, err error) {
	id = strings.TrimSpace(id)
	i := strings.LastIndex(id, "-")
	if i <= 0 || i == len(id)-1 {
		return "", 0, fmt.Errorf("run: %q is not <repo-basename>-<n>", id)
	}
	base = id[:i]
	n, err = strconv.ParseInt(id[i+1:], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("run: %q is not <repo-basename>-<n>", id)
	}
	return base, n, nil
}
