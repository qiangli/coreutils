package zot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureConfig_SeedsAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	cfg, err := ensureConfig(dir, "127.0.0.1", 5000)
	if err != nil {
		t.Fatalf("ensureConfig: %v", err)
	}
	if cfg != filepath.Join(dir, "config.json") {
		t.Fatalf("cfg path = %s", cfg)
	}
	b, _ := os.ReadFile(cfg)
	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("config is not valid JSON: %v", err)
	}
	http := parsed["http"].(map[string]any)
	if http["address"] != "127.0.0.1" || http["port"] != "5000" {
		t.Fatalf("http config = %v", http)
	}
	if _, ok := parsed["storage"]; !ok {
		t.Fatal("config missing storage")
	}
	// idempotent: second call must not overwrite
	first := string(b)
	if _, err := ensureConfig(dir, "127.0.0.1", 5000); err != nil {
		t.Fatal(err)
	}
	if b2, _ := os.ReadFile(cfg); string(b2) != first {
		t.Fatal("ensureConfig overwrote an existing config")
	}
}

func TestSpec(t *testing.T) {
	s := Spec("")
	if s.Repo != "project-zot/zot" || s.Name != "zot" || s.Version != "latest" {
		t.Fatalf("default spec = %+v", s)
	}
	if Spec("v2.1.0").Version != "v2.1.0" {
		t.Fatal("version override not honored")
	}
	if s.AssetMatch == nil {
		t.Fatal("zot spec needs a custom AssetMatch (full build, not minimal)")
	}
}

func TestAssetMatch_PicksFullBuild(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"zot-linux-amd64", true},
		{"zot-minimal-linux-amd64", false}, // minimal excluded
		{"zot-exporter-linux-amd64", false},
		{"zot-darwin-arm64", true},
		{"zot-linux-amd64", true},
	}
	for _, c := range cases {
		if got := assetMatch(c.name, "linux", "amd64"); strings.Contains(c.name, "darwin") {
			if assetMatch(c.name, "darwin", "arm64") != c.want {
				t.Errorf("assetMatch(%q, darwin/arm64) != %v", c.name, c.want)
			}
		} else if got != c.want {
			t.Errorf("assetMatch(%q, linux/amd64)=%v, want %v", c.name, got, c.want)
		}
	}
}
