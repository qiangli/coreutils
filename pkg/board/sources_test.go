package board

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDecodeWeaveDoctorFindings(t *testing.T) {
	raw := []byte(`{"status":"ok","result":{"open":[{"issue":7,"age_seconds":14401,"flags":["needs-steward","stale"]},{"issue":8,"age_seconds":60}]}}`)
	got, err := decodeWeaveDoctorFindings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got[7].AgeSeconds != 14401 || !got[7].Stale {
		t.Fatalf("stale finding = %+v", got[7])
	}
	if got[8].AgeSeconds != 60 || got[8].Stale {
		t.Fatalf("fresh finding = %+v", got[8])
	}
}

// The board reported zero todos on a host with 49 of them: `todo list --json`
// moved its rows under `result` in the loom-v2 envelope while this package still
// decoded a top-level `items`, so the decode yielded nothing and Load returned
// nil. Nothing failed, nothing warned. These pin the envelope contract between
// pkg/todo and pkg/board, which nothing tested before — each package was only
// ever checked against its own assumption.
func TestDecodeTodoListReadsLoomV2Envelope(t *testing.T) {
	raw := []byte(`{
	  "schema_version": "loom-v2",
	  "command": "todo list",
	  "status": "ok",
	  "result": {
	    "scope": "repo", "folder": "docs/todo", "count": 2,
	    "items": [
	      {"id":"aaaa1111","seq":7,"title":"first","state":"todo","priority":"p1","created":"2026-08-30T20:54:41Z"},
	      {"id":"bbbb2222","seq":8,"title":"second","state":"doing","priority":"p2","created":"2026-08-30T21:00:00Z"}
	    ]
	  }
	}`)
	items, err := decodeTodoList(raw, "repo x")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	// `state` is the loom-v2 spelling of what the board calls Status; reading the
	// old `status` key here would populate the column with empty strings, which
	// is the second, latent half of the same drift.
	if got := items[0].status(); got != "todo" {
		t.Errorf("status = %q, want %q", got, "todo")
	}
	if items[1].Seq != 8 || items[1].Title != "second" {
		t.Errorf("row 1 = %+v", items[1])
	}
}

func TestDecodeTodoListStillReadsPreLoomEnvelope(t *testing.T) {
	raw := []byte(`{"schema_version":"todo-v1","items":[
	  {"id":"cccc3333","seq":1,"title":"legacy","status":"todo","priority":"p3"}
	]}`)
	items, err := decodeTodoList(raw, "repo y")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 1 || items[0].status() != "todo" {
		t.Fatalf("got %+v, want one item with status todo", items)
	}
}

// The guard that shipped only asserted schema_version was non-empty, so
// "loom-v2" satisfied it while the payload shape had changed completely. The
// count cross-check is what actually detects a reshape.
func TestDecodeTodoListFailsLoudlyWhenTheEnvelopeReshapes(t *testing.T) {
	raw := []byte(`{"schema_version":"loom-v3","result":{"count":49,"entries":[{"id":"x"}]}}`)
	_, err := decodeTodoList(raw, "repo z")
	if err == nil {
		t.Fatal("want an error when the payload declares 49 rows and none decode")
	}
	for _, want := range []string{"49", "loom-v3", "changed shape"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

func TestDecodeTodoListRejectsUnversionedPayload(t *testing.T) {
	if _, err := decodeTodoList([]byte(`{"items":[]}`), "repo w"); err == nil {
		t.Fatal("want an error for an unversioned payload")
	}
}

// The web console is a host service and normally has no repository cwd. Fleet
// still has useful catalog/PATH evidence there; absence of repository-scoped
// weave cooldown data is an expected scope reduction, not a broken board
// source and must not become the dashboard's "1 source warning" banner.
func TestFleetSourceOutsideRepoUsesPATHFallbackWithoutWarning(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	b := &Board{}
	if err := (fleetSource{}).Load(context.Background(), b, Options{}); err != nil {
		t.Fatalf("non-repo fleet snapshot: %v", err)
	}
	if len(b.Agents) == 0 {
		t.Fatal("non-repo fleet snapshot lost the host catalog")
	}
	for _, a := range b.Agents {
		if a.Availability == "" {
			t.Fatalf("agent %s has no PATH/catalog fallback verdict: %+v", a.Name, a)
		}
	}
}

// A genuine availability collector failure remains a warning after the PATH
// fallback is populated. Only the expected absence of a repo is quiet.
func TestFleetSourceStillReportsRealAvailabilityFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	b := &Board{}
	want := errors.New("broken availability payload")
	err := (fleetSource{loadAvailability: func() (map[string]fleetAvailability, error) {
		return nil, want
	}}).Load(context.Background(), b, Options{})
	if err == nil || !strings.Contains(err.Error(), want.Error()) {
		t.Fatalf("fleet error = %v, want the genuine collector failure", err)
	}
	if len(b.Agents) == 0 {
		t.Fatal("a real collector failure must still populate the PATH fallback")
	}
}

// THE SPRINT BOARD IS USER-GLOBAL AND ITS STORIES ARE NOT.
//
// Story stores were discovered from the reader's own working directory and from
// the repo roots of weave RUNS. A cross-repo sprint whose work is not driven
// through weave has no runs, so its stories were visible only to a reader who
// happened to be standing in the right repo. Measured on a live host: the same
// sprint reported 23 stories from one directory and 0 from another, and the web
// console — whose process sits in whatever directory it was launched from —
// showed every sprint as having no stories at all.
//
// A sprint card carries story_roots. Reading it is what makes the answer
// independent of where the reader stands.
func TestSprintSourceCarriesStoryRoots(t *testing.T) {
	raw := []byte(`{
	  "status":"ok",
	  "result":{"stories":[
	    {"id":99,"title":"cross-repo sprint","column":"doing",
	     "story_roots":["/repos/umbrella","/repos/other"],
	     "lease":{"holder":"trestle","at":"2026-09-02T13:59:53Z"}}
	  ]}
	}`)
	var env wireEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Stories []struct {
			ID         int64    `json:"id"`
			StoryRoots []string `json:"story_roots"`
		} `json:"stories"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Stories) != 1 || len(result.Stories[0].StoryRoots) != 2 {
		t.Fatalf("story_roots did not survive the envelope: %+v", result.Stories)
	}
}

// The todo source must ask every sprint where its stories live. Without this
// the only cross-repo hints are the reader's cwd and the repo roots of weave
// runs, and neither is a property of the sprint being reported on: a sprint
// driven by hand has no runs, so its stories were found only by a reader who
// happened to be standing in the right repo.
func TestTodoSourceLooksInEverySprintStoryRoot(t *testing.T) {
	b := &Board{Sprints: []Sprint{
		{ID: 99, StoryRoots: []string{"/repos/umbrella", "/repos/other"}},
		{ID: 100, StoryRoots: []string{"/repos/umbrella"}}, // shared root, added once
		{ID: 101}, // no roots is not an error
	}}

	var args []string
	for _, st := range todoStores(b) {
		args = append(args, strings.Join(st.args, " "))
	}
	joined := strings.Join(args, "\n")
	for _, want := range []string{"--base-dir /repos/umbrella", "--base-dir /repos/other"} {
		if !strings.Contains(joined, want) {
			t.Errorf("story root not consulted (%q):\n%s", want, joined)
		}
	}
	if n := strings.Count(joined, "--base-dir /repos/umbrella"); n != 1 {
		t.Errorf("shared story root added %d times, want once", n)
	}
}

// An empty root is not a store. Cleaning "" yields ".", which would silently
// read whatever directory the process happens to be sitting in — the exact
// class of answer this change exists to remove.
func TestTodoSourceIgnoresAnEmptyStoryRoot(t *testing.T) {
	b := &Board{Sprints: []Sprint{{ID: 99, StoryRoots: []string{"", "   "}}}}
	for _, st := range todoStores(b) {
		for i, a := range st.args {
			if a == "--base-dir" && i+1 < len(st.args) && strings.TrimSpace(st.args[i+1]) == "" {
				t.Fatalf("empty story root became a store: %v", st.args)
			}
		}
	}
}

// Sprint loads BEFORE todo, because the todo source reads Sprint.StoryRoots.
// This ordering is not cosmetic and has no compiler to enforce it.
func TestDefaultSourcesLoadSprintBeforeTodo(t *testing.T) {
	var sprintAt, todoAt = -1, -1
	for i, s := range DefaultSources() {
		switch s.Name() {
		case "sprint":
			sprintAt = i
		case "todo":
			todoAt = i
		}
	}
	if sprintAt < 0 || todoAt < 0 {
		t.Fatalf("default sources lost a source: sprint=%d todo=%d", sprintAt, todoAt)
	}
	if sprintAt > todoAt {
		t.Fatalf("todo loads at %d before sprint at %d — story roots are not known yet", todoAt, sprintAt)
	}
}

// A story row must carry the sprint it belongs to. Run and sprint rows always
// did; the todo row dropped it, so the union view could not correlate a story
// to its sprint — the one correlation a board across three tools exists to make.
func TestTodoRowsCarryTheirSprintID(t *testing.T) {
	b := &Board{Todos: []Todo{
		{ID: "aaaa", Title: "linked", Status: "todo", SprintID: 99, Scope: "repo x"},
		{ID: "bbbb", Title: "unlinked", Status: "todo", Scope: "repo x"},
	}}
	b.finalize(time.Now())

	var linked, unlinked *Row
	for i := range b.Rows {
		switch b.Rows[i].ID {
		case "aaaa":
			linked = &b.Rows[i]
		case "bbbb":
			unlinked = &b.Rows[i]
		}
	}
	if linked == nil || unlinked == nil {
		t.Fatalf("todo rows missing: %+v", b.Rows)
	}
	if linked.SprintID != 99 {
		t.Errorf("linked story row sprint_id = %d, want 99", linked.SprintID)
	}
	// An unlinked todo is perfectly ordinary — todo does not require a sprint —
	// and must not be invented onto one.
	if unlinked.SprintID != 0 {
		t.Errorf("unlinked story row sprint_id = %d, want 0", unlinked.SprintID)
	}
}
