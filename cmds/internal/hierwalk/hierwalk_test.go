package hierwalk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type record struct {
	visited   []string
	links     []string
	cycles    []string
	statErrs  []string
	readErrs  []string
	skipped   []string
	skipNames map[string]bool
}

func (r *record) walker(mode Mode, recursive bool) *Walker {
	return &Walker{
		Mode:      mode,
		Recursive: recursive,
		Visit: func(_, display string, isLink bool) {
			r.visited = append(r.visited, display)
			if isLink {
				r.links = append(r.links, display)
			}
		},
		StatError: func(_, display string, _ error) { r.statErrs = append(r.statErrs, display) },
		ReadError: func(_, display string, _ error) { r.readErrs = append(r.readErrs, display) },
		Cycle:     func(_, display string) { r.cycles = append(r.cycles, display) },
		EnterDir: func(_, display string, _ bool) bool {
			if r.skipNames[display] {
				r.skipped = append(r.skipped, display)
				return false
			}
			return true
		},
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
}

// tree builds
//
//	d/sub/f
//	d/link -> sub          (a symbolic link to a directory, below the operand)
//	d/dangling -> nowhere
//	toplink -> d           (a symbolic link to a directory, as the operand)
func tree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d", "sub", "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, "sub", filepath.Join(dir, "d", "link"))
	mustSymlink(t, "nowhere", filepath.Join(dir, "d", "dangling"))
	mustSymlink(t, "d", filepath.Join(dir, "toplink"))
	return dir
}

func TestWalkNonRecursiveVisitsOperandOnly(t *testing.T) {
	dir := tree(t)
	for _, mode := range []Mode{Physical, CommandLine, Logical} {
		r := &record{}
		r.walker(mode, false).Walk(filepath.Join(dir, "d"), "d")
		if got := strings.Join(r.visited, " "); got != "d" {
			t.Errorf("mode %d: visited %q, want %q", mode, got, "d")
		}
	}
}

func TestWalkPhysicalDoesNotFollowAnyLink(t *testing.T) {
	dir := tree(t)
	r := &record{}
	r.walker(Physical, true).Walk(filepath.Join(dir, "toplink"), "toplink")
	// The operand link is not followed, so it is the whole hierarchy.
	if got := strings.Join(r.visited, " "); got != "toplink" {
		t.Errorf("operand link: visited %q, want %q", got, "toplink")
	}
	if got := strings.Join(r.links, " "); got != "toplink" {
		t.Errorf("operand link: links %q, want %q", got, "toplink")
	}

	r = &record{}
	r.walker(Physical, true).Walk(filepath.Join(dir, "d"), "d")
	want := "d/dangling d/link d/sub/f d/sub d"
	if got := strings.Join(r.visited, " "); got != filepath.FromSlash(want) {
		t.Errorf("visited %q, want %q", got, want)
	}
	if got := strings.Join(r.links, " "); got != filepath.FromSlash("d/dangling d/link") {
		t.Errorf("links %q, want the two links unfollowed", got)
	}
}

func TestWalkCommandLineFollowsOperandLinkOnly(t *testing.T) {
	dir := tree(t)
	r := &record{}
	r.walker(CommandLine, true).Walk(filepath.Join(dir, "toplink"), "toplink")
	want := filepath.FromSlash("toplink/dangling toplink/link toplink/sub/f toplink/sub toplink")
	if got := strings.Join(r.visited, " "); got != want {
		t.Errorf("-H visited %q, want %q", got, want)
	}
	// toplink/link is a link to a directory below the operand: -H must
	// not descend into it, so sub/f appears exactly once.
	if strings.Count(strings.Join(r.visited, " "), string(filepath.Separator)+"f") != 1 {
		t.Errorf("-H descended below an interior link: %v", r.visited)
	}
}

func TestWalkLogicalFollowsEveryLink(t *testing.T) {
	dir := tree(t)
	r := &record{}
	r.walker(Logical, true).Walk(filepath.Join(dir, "d"), "d")
	want := filepath.FromSlash("d/dangling d/link/f d/link d/sub/f d/sub d")
	if got := strings.Join(r.visited, " "); got != want {
		t.Errorf("-L visited %q, want %q", got, want)
	}
	// The dangling link stays an unfollowed entry under -L too; only the
	// command can decide what that means.
	if got := strings.Join(r.links, " "); got != filepath.FromSlash("d/dangling d/link") {
		t.Errorf("-L links %q, want both links reported as links", got)
	}
}

func TestWalkLogicalSelfReferenceStopsWithoutCycleReport(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, ".", filepath.Join(dir, "d", "self"))

	r := &record{}
	r.walker(Logical, true).Walk(filepath.Join(dir, "d"), "d")
	want := filepath.FromSlash("d/self d")
	if got := strings.Join(r.visited, " "); got != want {
		t.Errorf("-L self link: visited %q, want %q", got, want)
	}
	if len(r.cycles) != 0 {
		t.Errorf("-L self link reported a corrupt-hierarchy cycle: %v", r.cycles)
	}
}

func TestWalkLogicalMutualLoopTerminates(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustSymlink(t, filepath.Join("..", "b"), filepath.Join(dir, "a", "toB"))
	mustSymlink(t, filepath.Join("..", "a"), filepath.Join(dir, "b", "toA"))

	r := &record{}
	r.walker(Logical, true).Walk(filepath.Join(dir, "a"), "a")
	want := filepath.FromSlash("a/toB/toA a/toB a")
	if got := strings.Join(r.visited, " "); got != want {
		t.Errorf("-L mutual loop: visited %q, want %q", got, want)
	}
}

func TestWalkEnterDirSkipsSubtreeButNotOperand(t *testing.T) {
	dir := tree(t)
	r := &record{skipNames: map[string]bool{filepath.FromSlash("d/sub"): true}}
	r.walker(Physical, true).Walk(filepath.Join(dir, "d"), "d")
	want := filepath.FromSlash("d/dangling d/link d")
	if got := strings.Join(r.visited, " "); got != want {
		t.Errorf("EnterDir skip: visited %q, want %q", got, want)
	}
	if got := strings.Join(r.skipped, " "); got != filepath.FromSlash("d/sub") {
		t.Errorf("EnterDir skip: skipped %q", got)
	}

	// The operand is never offered to EnterDir: the command has already
	// screened it before the walk starts.
	r = &record{skipNames: map[string]bool{"d": true}}
	r.walker(Physical, true).Walk(filepath.Join(dir, "d"), "d")
	if len(r.skipped) != 0 {
		t.Errorf("operand offered to EnterDir: %v", r.skipped)
	}
}

func TestWalkReportsMissingOperand(t *testing.T) {
	dir := t.TempDir()
	r := &record{}
	r.walker(Physical, true).Walk(filepath.Join(dir, "nope"), "nope")
	if got := strings.Join(r.statErrs, " "); got != "nope" {
		t.Errorf("stat errors %q, want %q", got, "nope")
	}
	if len(r.visited) != 0 {
		t.Errorf("missing operand was visited: %v", r.visited)
	}
}

func TestWalkUnreadableDirectoryIsStillVisited(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable directories")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Skipf("chmod is unavailable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	r := &record{}
	r.walker(Physical, true).Walk(locked, "locked")
	if len(r.readErrs) != 1 {
		t.Skipf("directory remained readable on this platform: %v", r.readErrs)
	}
	if got := strings.Join(r.visited, " "); got != "locked" {
		t.Errorf("unreadable directory: visited %q, want %q", got, "locked")
	}
}

// A hierarchy that repeats a directory without a symbolic link having
// been followed is corrupt — hard-linked directories, which no
// unprivileged test can create. Substituting the identity predicate
// exercises the walk's response to one; everything else in the walk,
// including which directories reach the check, stays real.
func TestWalkCorruptHierarchyIsReportedNotDescended(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "d", "sub", "deeper")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	restore := sameFile
	sameFile = func(a, b os.FileInfo) bool { return b.Name() == "deeper" }
	t.Cleanup(func() { sameFile = restore })

	for _, mode := range []Mode{Physical, CommandLine} {
		r := &record{}
		r.walker(mode, true).Walk(filepath.Join(dir, "d"), "d")
		if got := strings.Join(r.cycles, " "); got != filepath.FromSlash("d/sub/deeper") {
			t.Errorf("mode %d: cycles %q, want the repeated directory", mode, got)
		}
		// The cyclic directory is neither visited nor descended into.
		if got := strings.Join(r.visited, " "); got != filepath.FromSlash("d/sub d") {
			t.Errorf("mode %d: visited %q", mode, got)
		}
	}

	// Under -L the same hierarchy is an ordinary symbolic link back up
	// the tree: the walk stops descending, reports nothing, and the
	// command still gets the directory.
	r := &record{}
	r.walker(Logical, true).Walk(filepath.Join(dir, "d"), "d")
	if len(r.cycles) != 0 {
		t.Errorf("-L reported a corrupt hierarchy: %v", r.cycles)
	}
	if got := strings.Join(r.visited, " "); got != filepath.FromSlash("d/sub/deeper d/sub d") {
		t.Errorf("-L visited %q", got)
	}
}
