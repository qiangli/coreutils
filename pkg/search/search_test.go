package search

import (
	"context"
	"errors"
	"testing"
)

func TestParseTavily(t *testing.T) {
	raw := []byte(`{"results":[{"title":"RFT survey","url":"https://x.test/rft","content":"rejection sampling fine-tuning..."}]}`)
	got, err := parseTavily(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].URL != "https://x.test/rft" || got[0].Snippet == "" || got[0].Backend != "tavily" {
		t.Fatalf("bad tavily parse: %+v", got)
	}
}

func TestParseBrave(t *testing.T) {
	raw := []byte(`{"web":{"results":[{"title":"A2A","url":"https://x.test/a2a","description":"agent to agent"}]}}`)
	got, err := parseBrave(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].URL != "https://x.test/a2a" || got[0].Snippet != "agent to agent" || got[0].Backend != "brave" {
		t.Fatalf("bad brave parse: %+v", got)
	}
}

func TestParseSerper(t *testing.T) {
	raw := []byte(`{"organic":[{"title":"MCP","link":"https://x.test/mcp","snippet":"model context protocol"}]}`)
	got, err := parseSerper(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].URL != "https://x.test/mcp" || got[0].Backend != "serper" {
		t.Fatalf("bad serper parse: %+v", got)
	}
}

func clearKeys(t *testing.T) {
	for _, k := range []string{"TAVILY_API_KEY", "BRAVE_API_KEY", "BRAVE_SEARCH_API_KEY", "SERPER_API_KEY", "BASHY_SEARCH_BACKEND"} {
		t.Setenv(k, "")
	}
}

func TestWebNoBackendConfigured(t *testing.T) {
	clearKeys(t)
	_, _, err := Web(context.Background(), "anything", Options{})
	if !errors.Is(err, ErrNoBackend) {
		t.Fatalf("want ErrNoBackend, got %v", err)
	}
}

func TestWebForcedUnknownBackend(t *testing.T) {
	clearKeys(t)
	_, _, err := Web(context.Background(), "q", Options{Backend: "bing"})
	if err == nil {
		t.Fatal("forcing an unknown backend must error")
	}
}

func TestWebForcedBackendMissingKey(t *testing.T) {
	clearKeys(t)
	_, _, err := Web(context.Background(), "q", Options{Backend: "tavily"})
	if err == nil {
		t.Fatal("forcing a backend whose key is unset must error clearly")
	}
}

func TestWebEmptyQuery(t *testing.T) {
	if _, _, err := Web(context.Background(), "   ", Options{}); err == nil {
		t.Fatal("empty query must error")
	}
}
