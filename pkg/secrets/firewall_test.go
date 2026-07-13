package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

// seedVaultTemplate points the secrets machinery at a scratch config/cache dir
// and writes a binding template naming one vault-ref secret and one literal.
func seedVaultTemplate(t *testing.T) {
	t.Helper()
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	mapPath := filepath.Join(cfg, "bashy", "secrets.map")
	if err := os.MkdirAll(filepath.Dir(mapPath), 0o700); err != nil {
		t.Fatal(err)
	}
	// OPENAI_API_KEY is a @ref (a real vault secret); EDITOR is a bare literal.
	if err := os.WriteFile(mapPath, []byte("OPENAI_API_KEY=@host-openai\nEDITOR=vim\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func has(env []string, name string) bool {
	for _, kv := range env {
		if len(kv) >= len(name)+1 && kv[:len(name)+1] == name+"=" {
			return true
		}
	}
	return false
}

// A spawned agent must NOT inherit a vault secret by default — the lethal
// trifecta. Non-secret variables (and template literals) are left alone.
func TestScrubAgentEnvRemovesVaultSecretByDefault(t *testing.T) {
	seedVaultTemplate(t)
	t.Setenv(AllowAgentSecretsEnv, "") // deny (the default)

	in := []string{"PATH=/bin", "OPENAI_API_KEY=sk-secret", "HOME=/h", "EDITOR=vim"}
	got := ScrubAgentEnv(in)

	if has(got, "OPENAI_API_KEY") {
		t.Fatalf("vault secret leaked to spawned agent: %v", got)
	}
	if !has(got, "PATH") || !has(got, "HOME") {
		t.Fatalf("non-secret env was dropped: %v", got)
	}
	if !has(got, "EDITOR") {
		t.Fatalf("a template literal (not a secret) must survive: %v", got)
	}
}

// The operator can explicitly opt back into passing the vault through, and that
// acceptance is the ONLY thing that restores it.
func TestScrubAgentEnvOptInRestoresSecrets(t *testing.T) {
	seedVaultTemplate(t)
	t.Setenv(AllowAgentSecretsEnv, "1")

	in := []string{"PATH=/bin", "OPENAI_API_KEY=sk-secret"}
	got := ScrubAgentEnv(in)
	if !has(got, "OPENAI_API_KEY") {
		t.Fatalf("opt-in should pass the vault through: %v", got)
	}
}

// With no binding template there is nothing to scrub, and the environment is
// returned untouched.
func TestScrubAgentEnvNoTemplateIsNoOp(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv(AllowAgentSecretsEnv, "")

	in := []string{"PATH=/bin", "OPENAI_API_KEY=sk-secret"}
	got := ScrubAgentEnv(in)
	if !has(got, "OPENAI_API_KEY") {
		t.Fatalf("with no vault template, nothing should be scrubbed: %v", got)
	}
}
