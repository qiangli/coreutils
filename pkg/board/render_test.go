package board

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) *Board {
	t.Helper()
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	sources := []Source{SourceFunc{SourceName: "fixture", Func: func(_ context.Context, b *Board, _ Options) error {
		b.Runs = []Run{{ID: 7, Label: "ship board", Repo: "coreutils", State: "working", Tool: "codex", Agent: "sol", Model: "gpt-5.6-sol", Band: 4, StartedAt: now.Add(-5 * time.Minute), MaxRuntime: 1800}, {ID: 8, Label: "merge me", Repo: "bashy", State: "submitted", Tool: "claude", Model: "opus4.8", Band: 4}, {ID: 9, Label: "commit survived watchdog", Repo: "coreutils", State: "killed", Tool: "codex", Salvageable: true, UnmergedCommits: 2}}
		b.Todos = []Todo{{ID: "abc", Number: 3, Title: "blocked chore", Status: "blocked", Scope: "user steward"}}
		b.Sprints = []Sprint{{ID: 2, Title: "board sprint", Column: "review"}}
		b.Agents = []Agent{{Name: "sol", Tool: "codex", Band: 4, Model: "gpt-5.6-sol", Available: true, Availability: "available", State: "working"}}
		return nil
	}}}
	b, err := Collect(context.Background(), Options{Now: now}, sources, nil)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTerminalAndJSONGoldens(t *testing.T) {
	b := fixture(t)
	text, err := (TerminalRenderer{}).Render(b, Options{Expand: map[string]bool{"agents": true}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fmt.Sprintf("%x", sha256.Sum256(text)), "29340581cc139cd993472e7041c36bfd86bfc88cd0ef5b220075f3ecdc1f85ae"; got != want {
		t.Errorf("terminal golden changed: got %s\n%s", got, text)
	}
	raw, err := (JSONRenderer{}).Render(b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var got Board
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != SchemaVersion || got.Summary.NeedsSteward != 4 {
		t.Fatalf("bad JSON envelope: %+v", got.Summary)
	}
	if sum, want := fmt.Sprintf("%x", sha256.Sum256(raw)), "0c0d7e4e2a1d09d28d5cd618ff989d1f2ffb58d1b6f33e0b3bddd86b107579cc"; sum != want {
		t.Errorf("JSON golden changed: got %s\n%s", sum, raw)
	}
}

func TestSalvageableRunRoutesToNeedsStewardAndPanel(t *testing.T) {
	b := fixture(t)
	var inLane bool
	for _, lane := range b.Lanes {
		if lane.ID != "needs-steward" {
			continue
		}
		for _, card := range lane.Cards {
			inLane = inLane || card.ID == "9" && card.Salvageable && card.Unmerged == 2
		}
	}
	if !inLane {
		t.Fatal("salvageable killed run was not routed to needs-steward")
	}
	var salvage PanelView
	for _, panel := range b.Panels {
		if panel.ID == "salvage" {
			salvage = panel
		}
	}
	if len(salvage.Rows) != 1 || salvage.Rows[0][0] != "#9" {
		t.Fatalf("salvage panel = %+v, want exactly run #9", salvage)
	}
	text, err := (TerminalRenderer{}).Render(b, Options{Expand: map[string]bool{"salvage": true}})
	if err != nil || !strings.Contains(string(text), "#9") || !strings.Contains(string(text), "2 commits") {
		t.Fatalf("expanded salvage panel did not list the steward decision: err=%v\n%s", err, text)
	}
}

func TestHTMLIsSelfContained(t *testing.T) {
	raw, err := (HTMLRenderer{}).Render(fixture(t), Options{Expand: map[string]bool{"agents": true}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{"<!doctype html>", "<style>", "prefers-color-scheme", "<details id=\"agents\" open"} {
		if !strings.Contains(s, want) {
			t.Errorf("html missing %q", want)
		}
	}
	for _, bad := range []string{"http://", "https://", "<script", "src=", "href="} {
		if strings.Contains(strings.ToLower(s), bad) {
			t.Errorf("HTML contains external-capable token %q", bad)
		}
	}
}
