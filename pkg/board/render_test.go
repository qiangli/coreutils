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
		b.Runs = []Run{{ID: 7, Label: "ship board", Repo: "coreutils", State: "working", Tool: "codex", Agent: "sol", Model: "gpt-5.6-sol", Band: 4, StartedAt: now.Add(-5 * time.Minute), MaxRuntime: 1800}, {ID: 8, Label: "merge me", Repo: "bashy", State: "submitted", Tool: "claude", Model: "opus4.8", Band: 4}}
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
	if got, want := fmt.Sprintf("%x", sha256.Sum256(text)), "b696e19d68b58fc08b5a86b970867a38ae218a47615bc6c219d089181c559d23"; got != want {
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
	if got.SchemaVersion != SchemaVersion || got.Summary.NeedsSteward != 3 {
		t.Fatalf("bad JSON envelope: %+v", got.Summary)
	}
	if sum, want := fmt.Sprintf("%x", sha256.Sum256(raw)), "a34d0042b480a656507fdcc77e278c8947b8e984d77810d15c2390f857e05996"; sum != want {
		t.Errorf("JSON golden changed: got %s\n%s", sum, raw)
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
