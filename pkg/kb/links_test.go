package kb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseLinks(t *testing.T) {
	body := `Start with a bare [[pkill-guard]] and an explicit [[kb:restart-check]].
File against [[todo:a5f5cfc8]] under [[sprint:163]].
Markdown: see [the note](pages/never-pkill.md) and [the task](../todo/deadbeef-fix-it.md).
Ignore [external](https://example.com/x.md) and [anchor](#section) and [non-md](pages/x.txt).`

	got := ParseLinks(body)
	want := map[string]LinkKind{
		"kb:pkill-guard":   LinkKB,
		"kb:restart-check": LinkKB,
		"todo:a5f5cfc8":    LinkTodo,
		"sprint:163":       LinkSprint,
		"kb:never-pkill":   LinkKB,
		"todo:deadbeef":    LinkTodo,
	}
	if len(got) != len(want) {
		t.Fatalf("parsed %d links, want %d: %+v", len(got), len(want), got)
	}
	for _, l := range got {
		wk, ok := want[l.Ref()]
		if !ok {
			t.Errorf("unexpected link %q", l.Ref())
			continue
		}
		if l.Kind != wk {
			t.Errorf("%q: kind %q, want %q", l.Ref(), l.Kind, wk)
		}
	}
}

// mkRepoKB makes a temp repo with a docs/kb store and a docs/todo dir, returning
// the kb store dir so a CLI run can point --dir at it and the sibling todo dir
// is discoverable by walking up to the .git marker.
func mkRepoKB(t *testing.T) (kbDir, todoDir string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	kbDir = filepath.Join(root, "docs", "kb")
	todoDir = filepath.Join(root, "docs", "todo")
	if err := os.MkdirAll(todoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return kbDir, todoDir
}

func writeTodo(t *testing.T, todoDir, id, title, body string) {
	t.Helper()
	content := "---\nid: " + id + "\ntitle: " + title + "\nstatus: open\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(todoDir, id+"-slug.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestBacklinksResolvesTodoToKB is the story gate: a todo whose body cites a kb
// page is reported by `kb backlinks`.
func TestBacklinksResolvesTodoToKB(t *testing.T) {
	kbDir, todoDir := mkRepoKB(t)
	mustRun(t, kbDir, "add", "--title", "pkill guard", "--slug", "pkill-guard",
		"--description", "WHEN tempted to kill a stuck process", "--body", "find the pid first")
	writeTodo(t, todoDir, "a5f5cfc81287", "wire the guard", "Implements [[kb:pkill-guard]] for the mesh.")

	out := mustRun(t, kbDir, "backlinks", "--json", "pkill-guard")
	var payload struct {
		Slug      string `json:"slug"`
		Backlinks []struct {
			Ref   string `json:"ref"`
			Kind  string `json:"kind"`
			Title string `json:"title"`
		} `json:"backlinks"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out)
	}
	if len(payload.Backlinks) != 1 {
		t.Fatalf("want 1 backlink, got %d: %+v", len(payload.Backlinks), payload.Backlinks)
	}
	if b := payload.Backlinks[0]; b.Kind != "todo" || b.Ref != "todo:a5f5cfc81287" {
		t.Fatalf("unexpected backlink: %+v", b)
	}
}

// TestDoctorDanglingLeavesBodyByteIdentical is the story gate: a body with a
// dangling [[kb:x]] is reported by doctor AND left byte-identical (doctor flags,
// never fixes).
func TestDoctorDanglingLeavesBodyByteIdentical(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--title", "alpha", "--slug", "alpha",
		"--description", "d", "--body", "this cites [[kb:missing]] which does not exist")

	path := filepath.Join(dir, "pages", "alpha.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	out := mustRun(t, dir, "doctor", "--json")
	var rep DoctorReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out)
	}
	found := false
	for _, d := range rep.Dangling {
		if d.From == "kb:alpha" && d.Target == "kb:missing" {
			found = true
		}
	}
	if !found {
		t.Fatalf("dangling [[kb:missing]] not reported: %+v", rep.Dangling)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("doctor rewrote the body:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestDoctorOrphanDuplicateFormDescription(t *testing.T) {
	dir := t.TempDir()
	store := Open(dir)

	// A validated, linked page is neither orphan nor flagged.
	if err := store.Write(&Page{Slug: "anchor", Form: FormPage, Type: TypeLesson,
		Title: "Anchor", Description: "anchored", Status: StatusValidated,
		Body: "links to [[kb:sidecar]]"}, "add"); err != nil {
		t.Fatal(err)
	}
	// sidecar is cited by anchor (inbound) so it is not an orphan.
	if err := store.Write(&Page{Slug: "sidecar", Form: FormPage, Type: TypeLesson,
		Title: "Sidecar", Description: "", Status: StatusCandidate}, "add"); err != nil {
		t.Fatal(err)
	}
	// An orphan: candidate, no links in or out.
	if err := store.Write(&Page{Slug: "lonely", Form: FormPage, Type: TypeLesson,
		Title: "Lonely", Description: "nobody cites me", Status: StatusCandidate}, "add"); err != nil {
		t.Fatal(err)
	}
	// Near-duplicates of each other.
	if err := store.Write(&Page{Slug: "dup-one", Form: FormPage, Type: TypeLesson,
		Title: "stop a stuck process on a paired host", Description: "x", Status: StatusCandidate}, "add"); err != nil {
		t.Fatal(err)
	}
	if err := store.Write(&Page{Slug: "dup-two", Form: FormPage, Type: TypeLesson,
		Title: "stop a stuck process on a paired host", Description: "y", Status: StatusCandidate}, "add"); err != nil {
		t.Fatal(err)
	}
	// A legacy record on disk with no form: key.
	legacy := "---\ntype: lesson\ntitle: Legacy\ndescription: old page\nstatus: candidate\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "pages", "legacy.md"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	pages, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	rep := Doctor(pages, store, nil, false)

	if !slices.Contains(rep.Orphans, "lonely") {
		t.Errorf("lonely should be an orphan: %+v", rep.Orphans)
	}
	if slices.Contains(rep.Orphans, "sidecar") {
		t.Errorf("sidecar has an inbound link, must not be an orphan: %+v", rep.Orphans)
	}
	if slices.Contains(rep.Orphans, "anchor") {
		t.Errorf("anchor is validated, must not be an orphan: %+v", rep.Orphans)
	}
	if !slices.Contains(rep.MissingDescription, "sidecar") {
		t.Errorf("sidecar is missing a description: %+v", rep.MissingDescription)
	}
	if !slices.Contains(rep.MissingForm, "legacy") {
		t.Errorf("legacy has no form: key: %+v", rep.MissingForm)
	}
	if slices.Contains(rep.MissingForm, "anchor") {
		t.Errorf("anchor declares form:, must not be flagged: %+v", rep.MissingForm)
	}
	foundDup := false
	for _, p := range rep.NearDuplicates {
		if p.A == "dup-one" && p.B == "dup-two" {
			foundDup = true
		}
	}
	if !foundDup {
		t.Errorf("dup-one/dup-two near-duplicate pair not reported: %+v", rep.NearDuplicates)
	}
}
