package loom

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureConfig_SeedsAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	cfg, err := ensureConfig(dir, "127.0.0.1", 3000, "https://ai.dhnt.io/matrix/h/dragon/app/loom/", true)
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
		"ROOT_URL = https://ai.dhnt.io/matrix/h/dragon/app/loom/",
		"SECRET_KEY = ",
		"DISABLE_REGISTRATION = true",
		"ENABLE_REVERSE_PROXY_AUTHENTICATION = true",
		"ENABLE_REVERSE_PROXY_AUTO_REGISTRATION = true",
		"REVERSE_PROXY_AUTHENTICATION_USER = X-WEBAUTH-USER",
		"REVERSE_PROXY_AUTHENTICATION_EMAIL = X-WEBAUTH-EMAIL",
		"[actions]",      // local CI control plane
		"ENABLED = true", // act_runner registers against it
	} {
		if !strings.Contains(s, want) {
			t.Errorf("seeded config missing %q", want)
		}
	}
	header, err := os.ReadFile(filepath.Join(dir, "custom", "templates", "custom", "header.tmpl"))
	if err != nil {
		t.Fatalf("custom header: %v", err)
	}
	for _, want := range []string{"https://docs.gitea.com", ".page-footer", "p.large", "navbar-logo", "/app/loom/"} {
		if !strings.Contains(string(header), want) {
			t.Errorf("custom header missing %q", want)
		}
	}
	if strings.Contains(string(header), "/user/login") {
		t.Error("custom header should not hide the sign-in link; browser issue filing needs login")
	}
	// Second call must not overwrite (stable secret across restarts), with the
	// same actions toggle.
	if _, err := ensureConfig(dir, "127.0.0.1", 3000, "https://ai.dhnt.io/matrix/h/dragon/app/loom/", true); err != nil {
		t.Fatal(err)
	}
	if b2, _ := os.ReadFile(cfg); string(b2) != s {
		t.Fatal("ensureConfig overwrote an existing config")
	}
}

func TestEnsureConfig_ReconcilesServerAndActions(t *testing.T) {
	dir := t.TempDir()
	cfg, err := ensureConfig(dir, "127.0.0.1", 3000, "http://127.0.0.1:3000/", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ensureConfig(dir, "127.0.0.1", 3001, "https://ai.dhnt.io/matrix/h/dragon/app/loom/", false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(cfg)
	s := string(data)
	for _, want := range []string{
		"HTTP_PORT = 3001",
		"ROOT_URL = https://ai.dhnt.io/matrix/h/dragon/app/loom/",
		"ENABLED = false",
		"ENABLE_REVERSE_PROXY_AUTHENTICATION = true",
		"REVERSE_PROXY_AUTHENTICATION_USER = X-WEBAUTH-USER",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("reconciled config missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "SECRET_KEY = \n") {
		t.Fatalf("secret was lost:\n%s", s)
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

func TestCommandSurfaceIncludesLifecycleManagement(t *testing.T) {
	cmd := NewLoomCmd()
	have := map[string]bool{}
	for _, c := range cmd.Commands() {
		have[c.Name()] = true
	}
	for _, name := range []string{"serve", "start", "status", "stop", "logs", "expose", "path", "proxy"} {
		if !have[name] {
			t.Fatalf("missing command %q", name)
		}
	}
}

func TestProxyTranslatesRemoteIdentityToWebauth(t *testing.T) {
	var gotUser, gotEmail, gotName string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = r.Header.Get("X-WEBAUTH-USER")
		gotEmail = r.Header.Get("X-WEBAUTH-EMAIL")
		gotName = r.Header.Get("X-WEBAUTH-FULLNAME")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	handler, err := loomProxyHandler(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	req, _ := http.NewRequest(http.MethodGet, proxy.URL+"/", nil)
	req.Header.Set("Remote-User", "alice@example.com")
	req.Header.Set("Remote-Email", "alice@example.com")
	req.Header.Set("Remote-Name", "Alice")
	req.Header.Set("X-WEBAUTH-USER", "attacker@example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if gotUser != "alice@example.com" || gotEmail != "alice@example.com" || gotName != "Alice" {
		t.Fatalf("webauth headers = (%q,%q,%q), want alice@example.com/alice@example.com/Alice", gotUser, gotEmail, gotName)
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st := State{
		PID:       12345,
		URL:       "http://127.0.0.1:3000",
		RootURL:   "https://ai.dhnt.io/matrix/h/dragon/app/loom/",
		Addr:      "127.0.0.1:3000",
		Version:   "v1.2.3",
		DataDir:   dir,
		LogPath:   filepath.Join(dir, "loom.log"),
		StartedAt: time.Date(2026, 7, 2, 1, 2, 3, 0, time.UTC),
	}
	if err := writeState(st); err != nil {
		t.Fatal(err)
	}
	got, err := readState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.PID != st.PID || got.URL != st.URL || got.RootURL != st.RootURL || got.LogPath != st.LogPath || !got.StartedAt.Equal(st.StartedAt) {
		t.Fatalf("state mismatch: %+v", got)
	}
	if err := removeState(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := readState(dir); !os.IsNotExist(err) {
		t.Fatalf("expected removed state, got %v", err)
	}
}
