package webconsole

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/coopauth"
)

func newTestHandler(t *testing.T, opts Options) http.Handler {
	t.Helper()
	opts.Ctx = context.Background()
	h, closer, err := Handler(opts)
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	t.Cleanup(func() { _ = closer() })
	return h
}

// do issues a request with an explicit peer address, which is what the gate and
// the spoofed-header strip both key on.
func do(h http.Handler, method, path, peer string, hdr http.Header) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = peer
	for k, vs := range hdr {
		for _, v := range vs {
			r.Header.Add(k, v)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestLoopbackIsUngated(t *testing.T) {
	h := newTestHandler(t, Options{})
	if got := do(h, "GET", "/", "127.0.0.1:5555", nil).Code; got != http.StatusOK {
		t.Fatalf("start page on loopback = %d, want 200", got)
	}
	if got := do(h, "GET", "/healthz", "127.0.0.1:5555", nil).Code; got != http.StatusOK {
		t.Fatalf("healthz = %d, want 200", got)
	}
}

// The nested-prefix trap: mounting the room stamps X-Forwarded-Prefix so the
// SPA gets a correct <base href>, and ArrivedViaCloud is DEFINED as that header
// being present — so meet's own gate would 403 the machine owner on their own
// loopback. It must receive the pass-through gate instead.
func TestMountedRoomDoesNotLockOutTheOwner(t *testing.T) {
	h := newTestHandler(t, Options{})
	w := do(h, "GET", "/relay/api/rooms", "127.0.0.1:5555", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("mounted room on loopback = %d, want 200 (body %q)\n"+
			"the mount's X-Forwarded-Prefix is being read as 'arrived via cloud'",
			w.Code, strings.TrimSpace(w.Body.String()))
	}
}

// A mounted panel must see the CONCATENATED prefix, never a replaced one, or a
// tunnelled console renders every panel with broken asset URLs.
func TestMountComposesPrefixes(t *testing.T) {
	var got string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = coopauth.BaseHref(r) + "|" + r.URL.Path
	})
	h := coopauth.Mount("/relay", inner)

	r := httptest.NewRequest("GET", "/relay/api/rooms", nil)
	r.Header.Set(coopauth.HdrForwardedPrefix, "/matrix/h/laptop/app/console")
	h.ServeHTTP(httptest.NewRecorder(), r)

	want := "/matrix/h/laptop/app/console/relay/|/api/rooms"
	if got != want {
		t.Fatalf("nested mount = %q, want %q", got, want)
	}
}

// The header-spoofing fix: off-loopback, a caller must not be able to vouch for
// themselves. Without the strip, coopauth admits on these headers alone whenever
// no SSO secret is configured — which is the default.
func TestSpoofedVouchFromOffHostIsRefused(t *testing.T) {
	h := newTestHandler(t, Options{})
	w := do(h, "GET", "/api/apps", "203.0.113.9:41234", http.Header{
		coopauth.HdrForwardedPrefix: {"/x"},
		coopauth.HdrRemoteUser:      {"root@example.com"},
		coopauth.HdrRemoteGroups:    {"admin"},
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("spoofed vouch from off-host = %d, want 403", w.Code)
	}
}

func TestOffHostWithoutVouchIsRefused(t *testing.T) {
	h := newTestHandler(t, Options{})
	if got := do(h, "GET", "/api/apps", "203.0.113.9:41234", nil).Code; got != http.StatusForbidden {
		t.Fatalf("off-host = %d, want 403", got)
	}
}

func TestAppsListsTheBuiltinSurfaces(t *testing.T) {
	h := newTestHandler(t, Options{})
	w := do(h, "GET", "/api/apps", "127.0.0.1:5555", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("apps = %d", w.Code)
	}
	var got struct {
		Schema string `json:"schema_version"`
		Apps   []struct {
			Name      string `json:"name"`
			Status    string `json:"status"`
			StartHint string `json:"start_hint"`
			Mode      string `json:"mode"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, w.Body.String())
	}
	if got.Schema != appsSchemaVersion {
		t.Errorf("schema = %q, want %q", got.Schema, appsSchemaVersion)
	}
	seen := map[string]string{}
	for _, a := range got.Apps {
		seen[a.Name] = a.Status
		// A stopped proxy tile must always be able to tell the reader how to
		// start it; a tile that only says "stopped" sends them hunting.
		if a.Mode == "proxy" && a.StartHint == "" {
			t.Errorf("proxy surface %q has no start hint", a.Name)
		}
	}
	for _, want := range []string{"terminal", "relay"} {
		if _, ok := seen[want]; !ok {
			t.Errorf("surface %q missing from /api/apps (have %v)", want, seen)
		}
	}
}

func TestDeepLinkAliasesSurvive(t *testing.T) {
	h := newTestHandler(t, Options{})
	for path, want := range map[string]string{"/shell": "/term/", "/meet": "/relay/"} {
		w := do(h, "GET", path, "127.0.0.1:5555", nil)
		if w.Code != http.StatusFound || w.Header().Get("Location") != want {
			t.Errorf("%s -> %d %q, want 302 %q", path, w.Code, w.Header().Get("Location"), want)
		}
	}
}

// A stopped proxied panel must explain itself rather than leak a dial error.
func TestStoppedProxyPanelExplainsItself(t *testing.T) {
	h := newTestHandler(t, Options{Panels: []Panel{{
		Name: "loom", Label: "Loom", Path: "/loom/", Mode: "proxy",
		Port: 1, Start: []string{"loom", "start"}, Source: "atlas", Available: true,
	}}})
	w := do(h, "GET", "/loom/", "127.0.0.1:5555", nil)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("stopped proxy = %d, want 502", w.Code)
	}
	if !strings.Contains(w.Body.String(), "bashy loom start") {
		t.Errorf("stopped proxy body lacks the start hint: %q", w.Body.String())
	}
}

// The base href is per-request, not per-build: the same binary is reached at /
// on loopback and under a tunnel prefix.
func TestIndexBaseHrefFollowsTheMount(t *testing.T) {
	h := newTestHandler(t, Options{})
	plain := do(h, "GET", "/", "127.0.0.1:5555", nil).Body.String()
	if !strings.Contains(plain, `<base href="/">`) {
		t.Errorf("loopback index missing <base href=\"/\">")
	}
	// A tunnelled request arrives FROM outpost over loopback and carries a
	// vouched identity; without one the gate refuses it, which is the point of
	// TestSpoofedVouchFromOffHostIsRefused.
	tunnelled := do(h, "GET", "/", "127.0.0.1:5555", http.Header{
		coopauth.HdrForwardedPrefix: {"/matrix/h/laptop/app/console"},
		coopauth.HdrRemoteUser:      {"owner@example.com"},
	})
	if !strings.Contains(tunnelled.Body.String(), `<base href="/matrix/h/laptop/app/console/">`) {
		t.Errorf("tunnelled index has the wrong <base href>")
	}
}

// A tile's Path is what the browser follows; mounting from the panel NAME
// instead served a 404 at the exact link the start page renders.
func TestEveryAvailablePanelIsMountedAtItsAdvertisedPath(t *testing.T) {
	panels := Discover()
	h := newTestHandler(t, Options{Panels: panels})
	for _, p := range panels {
		if !p.Available {
			continue
		}
		w := do(h, "GET", p.Path, "127.0.0.1:5555", nil)
		// An unmounted path does not 404 — it falls through to the console's own
		// SPA route and returns the START PAGE, which looks fine in a browser and
		// is exactly how the terminal's /term/ mismatch hid. So assert on what
		// came back, not on the status.
		if strings.Contains(w.Body.String(), "data-bashy-console") {
			t.Errorf("panel %q advertises %s but that path fell through to the console start page",
				p.Name, p.Path)
		}
	}
}
