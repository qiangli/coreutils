package board

import (
	"context"
	"errors"
	"strings"
	"testing"
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
