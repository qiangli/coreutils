package loom

import (
	"io"
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

	handler, err := loomProxyHandler(upstream.URL, "")
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

func TestProxyUsesSharedLoopbackAdminIdentity(t *testing.T) {
	var gotUser, gotEmail, gotName string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = r.Header.Get("X-WEBAUTH-USER")
		gotEmail = r.Header.Get("X-WEBAUTH-EMAIL")
		gotName = r.Header.Get("X-WEBAUTH-FULLNAME")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	handler, err := loomProxyHandler(upstream.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	resp, err := http.Get(proxy.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if gotUser != LoopbackUser || gotEmail != LoopbackEmail || gotName != LoopbackName {
		t.Fatalf("loopback webauth headers = (%q,%q,%q), want %s/%s/%s", gotUser, gotEmail, gotName, LoopbackUser, LoopbackEmail, LoopbackName)
	}
}

func TestProxyStripsPublicPrefix(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write([]byte("body{}"))
	}))
	t.Cleanup(upstream.Close)

	handler, err := loomProxyHandler(upstream.URL, "/matrix/h/dragon/app/loom")
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	resp, err := http.Get(proxy.URL + "/matrix/h/dragon/app/loom/assets/css/index.css")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if gotPath != "/assets/css/index.css" {
		t.Fatalf("upstream path = %q, want /assets/css/index.css", gotPath)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/css") {
		t.Fatalf("content-type = %q, want text/css", ct)
	}
}

func TestProxyRewritesPublicPrefixForDirectLocalHTML(t *testing.T) {
	const prefix = "/matrix/h/dragon/app/loom"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Location", "https://ai.dhnt.io"+prefix+"/user/login?redirect_to=%2F")
		_, _ = w.Write([]byte(`<meta content="http://127.0.0.1:31880` + prefix + `/avatars/a"><link href="` + prefix + `/assets/css/index.css"><script src="` + prefix + `/assets/js/index.js"></script>`))
	}))
	t.Cleanup(upstream.Close)

	handler, err := loomProxyHandler(upstream.URL, prefix)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	req, _ := http.NewRequest(http.MethodGet, proxy.URL+"/loom/repo/issues/new", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), prefix) {
		t.Fatalf("direct local HTML still contains public prefix:\n%s", body)
	}
	for _, want := range []string{`href="/assets/css/index.css"`, `src="/assets/js/index.js"`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("direct local HTML missing %q:\n%s", want, body)
		}
	}
	if got := resp.Header.Get("Location"); got != "/user/login?redirect_to=%2F" {
		t.Fatalf("Location = %q, want local path", got)
	}
}

func TestProxyPreservesPublicPrefixForForwardedHTML(t *testing.T) {
	const prefix = "/matrix/h/dragon/app/loom"
	var gotAcceptEncoding string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAcceptEncoding = r.Header.Get("Accept-Encoding")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<link href="` + prefix + `/assets/css/index.css">`))
	}))
	t.Cleanup(upstream.Close)

	handler, err := loomProxyHandler(upstream.URL, prefix)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	req, _ := http.NewRequest(http.MethodGet, proxy.URL+prefix+"/loom/repo/issues/new", nil)
	req.Header.Set("X-Forwarded-Prefix", prefix)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), prefix+"/assets/css/index.css") {
		t.Fatalf("forwarded HTML lost public prefix:\n%s", body)
	}
	if gotAcceptEncoding != "gzip" {
		t.Fatalf("Accept-Encoding = %q, want preserved for forwarded traffic", gotAcceptEncoding)
	}
}

func TestAdminListContainsUser(t *testing.T) {
	out := `ID   Username Email           IsAdmin
1    admin    admin@localhost true
2    alice    alice@test      true
`
	if !adminListContainsUser(out, "admin") {
		t.Fatal("admin user not detected")
	}
	if adminListContainsUser(out, "root") {
		t.Fatal("unexpected root user detected")
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
