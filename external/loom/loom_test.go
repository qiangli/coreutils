package loom

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureConfig_SeedsAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	cfg, err := ensureConfig(dir, "127.0.0.1", 3000, true)
	if err != nil {
		t.Fatalf("ensureConfig: %v", err)
	}
	if cfg != filepath.Join(dir, "app.ini") {
		t.Fatalf("cfg path = %s", cfg)
	}
	b, _ := os.ReadFile(cfg)
	s := string(b)
	for _, want := range []string{
		"INSTALL_LOCK = true", // boots ready, not /install
		"DB_TYPE = sqlite3",   // no external DB
		"HTTP_ADDR = 127.0.0.1",
		"HTTP_PORT = 3000",
		"SECRET_KEY = ",
		"DISABLE_REGISTRATION = true",
		"[actions]",         // local CI control plane
		"ENABLED = true",    // act_runner registers against it
	} {
		if !strings.Contains(s, want) {
			t.Errorf("seeded config missing %q", want)
		}
	}
	// Second call must not overwrite (stable secret across restarts), with the
	// same actions toggle.
	if _, err := ensureConfig(dir, "127.0.0.1", 3000, true); err != nil {
		t.Fatal(err)
	}
	if b2, _ := os.ReadFile(cfg); string(b2) != s {
		t.Fatal("ensureConfig overwrote an existing config")
	}
}

func TestSpec(t *testing.T) {
	s := Spec("")
	if s.Repo != "go-gitea/gitea" || s.Name != "loom" || s.Version != "latest" {
		t.Fatalf("default spec = %+v", s)
	}
	if Spec("v1.24.0").Version != "v1.24.0" {
		t.Fatal("version override not honored")
	}
}
