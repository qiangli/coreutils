package secrets

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeVault is an in-memory stand-in for cloudbox's /api/v1/secrets.
type fakeVault struct {
	t      *testing.T
	data   map[string]string
	server *httptest.Server
}

func newFakeVault(t *testing.T) *fakeVault {
	fv := &fakeVault{t: t, data: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/secrets", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			http.Error(w, "bad auth: "+got, http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			var items []Item
			for k, v := range fv.data {
				items = append(items, Item{Name: k, Value: v})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": items})
		case http.MethodPost:
			var body struct {
				Secrets []Item `json:"secrets"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			for _, s := range body.Secrets {
				fv.data[s.Name] = s.Value
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "count": len(body.Secrets)})
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/v1/secrets/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/api/v1/secrets/")
		switch r.Method {
		case http.MethodGet:
			v, ok := fv.data[name]
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(Item{Name: name, Value: v})
		case http.MethodDelete:
			if _, ok := fv.data[name]; !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			delete(fv.data, name)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	})
	fv.server = httptest.NewServer(mux)
	t.Cleanup(fv.server.Close)
	return fv
}

func (fv *fakeVault) cfg() Config { return Config{URL: fv.server.URL, Token: "test-token"} }

// run executes the secrets command tree with isolated stdout/stderr and a
// scratch HOME/XDG_CACHE so the cache lands in a tempdir.
func run(t *testing.T, cfg Config, args ...string) (string, string, error) {
	t.Helper()
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	// Neutralize ambient token/url discovery so tests are hermetic.
	t.Setenv("BASHY_SECRETS_TOKEN", "")
	t.Setenv("DHNT_SECRETS_TOKEN", "")
	t.Setenv("DHNT_API_KEY", "")
	t.Setenv("DHNT_BASE_URL", "")
	t.Setenv("BASHY_CLOUDBOX_URL", "")

	cmd := newSecretsCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	full := append([]string{"--url", cfg.URL, "--token", cfg.Token}, args...)
	cmd.SetArgs(full)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

func TestImportThenEnvRoundTrip(t *testing.T) {
	fv := newFakeVault(t)
	rc := `# a comment
export OPENAI_API_KEY=sk-proj-abc
export GITHUB_TOKEN=ghp_xyz
#export ANTHROPIC_API_KEY=should-be-skipped
KIMI_API_KEY="sk-quoted"
`
	rcFile := filepath.Join(t.TempDir(), ".novigensrc")
	if err := os.WriteFile(rcFile, []byte(rc), 0o600); err != nil {
		t.Fatal(err)
	}

	out, _, err := run(t, fv.cfg(), "import", rcFile)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !strings.Contains(out, "imported 3 secret(s)") {
		t.Fatalf("import out = %q", out)
	}
	if _, ok := fv.data["ANTHROPIC_API_KEY"]; ok {
		t.Fatal("commented line must not import")
	}
	if fv.data["KIMI_API_KEY"] != "sk-quoted" {
		t.Fatalf("quoted value = %q, want sk-quoted", fv.data["KIMI_API_KEY"])
	}

	out, _, err = run(t, fv.cfg(), "env")
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	want := "export GITHUB_TOKEN='ghp_xyz'\nexport KIMI_API_KEY='sk-quoted'\nexport OPENAI_API_KEY='sk-proj-abc'\n"
	if out != want {
		t.Fatalf("env out =\n%q\nwant\n%q", out, want)
	}
}

func TestEnvCacheFallbackWhenServerDown(t *testing.T) {
	fv := newFakeVault(t)
	fv.data["DEEPSEEK_API_KEY"] = "sk-d"

	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("BASHY_SECRETS_TOKEN", "")
	t.Setenv("DHNT_API_KEY", "")
	t.Setenv("DHNT_BASE_URL", "")
	t.Setenv("BASHY_CLOUDBOX_URL", "")

	// First env populates the cache.
	cmd := newSecretsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--url", fv.server.URL, "--token", "test-token", "env"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("env(1): %v", err)
	}
	if !strings.Contains(out.String(), "export DEEPSEEK_API_KEY='sk-d'") {
		t.Fatalf("env(1) = %q", out.String())
	}

	// Server goes away; a refresh must fall back to cache and still exit 0.
	fv.server.Close()
	cmd = newSecretsCmd()
	var out2, err2 bytes.Buffer
	cmd.SetOut(&out2)
	cmd.SetErr(&err2)
	cmd.SetArgs([]string{"--url", fv.server.URL, "--token", "test-token", "env", "--refresh"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("env(2) should not error (degrade gracefully): %v", err)
	}
	if !strings.Contains(out2.String(), "export DEEPSEEK_API_KEY='sk-d'") {
		t.Fatalf("env(2) cache fallback = %q", out2.String())
	}
	if !strings.Contains(out2.String(), "served from cache") {
		t.Fatalf("env(2) should note cache fallback: %q", out2.String())
	}
}

func TestGetSetRm(t *testing.T) {
	fv := newFakeVault(t)

	if _, _, err := run(t, fv.cfg(), "set", "TELEGRAM_BOT_TOKEN", "123:abc"); err != nil {
		t.Fatalf("set: %v", err)
	}
	out, _, err := run(t, fv.cfg(), "get", "TELEGRAM_BOT_TOKEN")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.TrimSpace(out) != "123:abc" {
		t.Fatalf("get = %q", out)
	}

	out, _, err = run(t, fv.cfg(), "ls")
	if err != nil || !strings.Contains(out, "TELEGRAM_BOT_TOKEN") {
		t.Fatalf("ls = %q err=%v", out, err)
	}

	if _, _, err := run(t, fv.cfg(), "rm", "TELEGRAM_BOT_TOKEN"); err != nil {
		t.Fatalf("rm: %v", err)
	}
	if _, ok := fv.data["TELEGRAM_BOT_TOKEN"]; ok {
		t.Fatal("rm did not delete")
	}
}

func TestSetFromStdin(t *testing.T) {
	fv := newFakeVault(t)
	cmd := newSecretsCmd()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader("sk-from-stdin\n"))
	cmd.SetArgs([]string{"--url", fv.server.URL, "--token", "test-token", "set", "OPENAI_API_KEY"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("set stdin: %v", err)
	}
	if fv.data["OPENAI_API_KEY"] != "sk-from-stdin" {
		t.Fatalf("stdin value = %q (trailing newline must be trimmed)", fv.data["OPENAI_API_KEY"])
	}
}

func TestShellSingleQuote(t *testing.T) {
	cases := map[string]string{
		"plain":     "'plain'",
		"a b":       "'a b'",
		"it's":      `'it'\''s'`,
		"$(rm -rf)": "'$(rm -rf)'",
	}
	for in, want := range cases {
		if got := shellSingleQuote(in); got != want {
			t.Errorf("shellSingleQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseEnvFileEdgeCases(t *testing.T) {
	in := `
export A=1
B=2
  export C = 3
export D=val # trailing comment
export E='quoted # hash'
export 9BAD=nope
# export F=skip
export G=
`
	items, err := parseEnvFile(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, it := range items {
		got[it.Name] = it.Value
	}
	checks := map[string]string{
		"A": "1",
		"B": "2",
		"D": "val",
		"E": "quoted # hash",
		"G": "",
	}
	for k, v := range checks {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got["F"]; ok {
		t.Error("commented F must be skipped")
	}
	if _, ok := got["9BAD"]; ok {
		t.Error("invalid name 9BAD must be skipped")
	}
	// "C = 3" -> name "C", value "3" (spaces trimmed both sides).
	if got["C"] != "3" {
		t.Errorf("C = %q, want 3", got["C"])
	}
}

func TestResolveTokenPrecedence(t *testing.T) {
	t.Setenv("BASHY_SECRETS_TOKEN", "from-bashy")
	t.Setenv("DHNT_API_KEY", "from-dhnt")
	c, err := Config{URL: "http://x"}.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "from-bashy" {
		t.Fatalf("token = %q, want from-bashy (BASHY_SECRETS_TOKEN wins)", c.Token)
	}
	// Flag beats env.
	c, _ = Config{URL: "http://x", Token: "from-flag"}.Resolve()
	if c.Token != "from-flag" {
		t.Fatalf("token = %q, want from-flag", c.Token)
	}
}

func TestResolveURLFromDHNT(t *testing.T) {
	t.Setenv("BASHY_CLOUDBOX_URL", "")
	t.Setenv("DHNT_BASE_URL", "https://ai.dhnt.io/v1")
	c, err := Config{Token: "x"}.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != "https://ai.dhnt.io" {
		t.Fatalf("baseURL = %q, want https://ai.dhnt.io (strip /v1)", c.BaseURL)
	}
}

// guard: the JSON encoder used by the fake is the real wire shape.
var _ = io.Discard
