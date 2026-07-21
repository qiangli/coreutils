package resources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestCanonicalProvider(t *testing.T) {
	tests := []struct {
		model, provider, want string
	}{
		{"claude-opus", "anthropic", "Anthropic"},
		{"fable5", "", "Anthropic"},
		{"gpt-5.5", "openai", "OpenAI"},
		{"codex-gpt-5.5", "openai-compat", "OpenAI"},
		{"gemini3.1", "gemini", "Google"},
		{"agy-gemini", "google", "Google"},
		{"glm-5.2", "openai-compat", "Zhipu"},
		{"zhipu-model", "z.ai", "Zhipu"},
		{"kimi-k3", "openai-compat", "Moonshot"},
		{"moonshot-v1", "moonshot", "Moonshot"},
		{"deepseek-v4-pro", "openai-compat", "DeepSeek"},
	}

	for _, tt := range tests {
		got := CanonicalProvider(tt.model, tt.provider)
		if got != tt.want {
			t.Errorf("CanonicalProvider(%q, %q) = %q, want %q", tt.model, tt.provider, got, tt.want)
		}
	}
}

func TestCollectFleetResourcesSchema(t *testing.T) {
	ctx := context.Background()
	fr, err := CollectFleetResources(ctx)
	if err != nil {
		t.Fatalf("CollectFleetResources failed: %v", err)
	}

	if fr.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", fr.SchemaVersion, SchemaVersion)
	}

	if len(fr.Groups) == 0 {
		t.Errorf("expected non-empty Groups")
	}

	// Verify busy + idle + cooling + unavailable == total for every group
	for _, g := range fr.Groups {
		if g.Busy+g.Idle+g.Cooling+g.Unavailable != g.Total {
			t.Errorf("Group %s %s sum mismatch: busy(%d) + idle(%d) + cooling(%d) + unavail(%d) != total(%d)",
				g.Provider, g.Band, g.Busy, g.Idle, g.Cooling, g.Unavailable, g.Total)
		}
		if g.Subscription+g.APIKey != g.Total {
			t.Errorf("Group %s %s sub/api mismatch: sub(%d) + api(%d) != total(%d)",
				g.Provider, g.Band, g.Subscription, g.APIKey, g.Total)
		}
	}

	// Verify JSON serialization
	b, err := json.Marshal(fr)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var unmarshaled FleetResources
	if err := json.Unmarshal(b, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if unmarshaled.SchemaVersion != SchemaVersion {
		t.Errorf("unmarshaled SchemaVersion = %q, want %q", unmarshaled.SchemaVersion, SchemaVersion)
	}

	// Verify table formatting
	tableStr := FormatTable(fr)
	if !strings.Contains(tableStr, "PROVIDER") || !strings.Contains(tableStr, "BAND") {
		t.Errorf("FormatTable output missing expected header columns:\n%s", tableStr)
	}
}
