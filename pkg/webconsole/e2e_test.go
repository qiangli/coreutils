//go:build e2e

// End-to-end tests for the launcher and its apps.
//
// These bind a REAL port and speak real HTTP and WebSocket, because the failures
// this package has actually shipped were invisible to handler-level tests: a
// script whose entry point had been spliced out (every response still 200, page
// blank), assets served with no validator (fix shipped, browser kept the old
// copy), a panel mounted at a path its own tile did not link to. Each of those
// needs the whole thing running to see.
//
//	go test -tags e2e ./pkg/webconsole/
package webconsole

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/qiangli/coreutils/pkg/hostauth"
	"github.com/qiangli/coreutils/pkg/websession"
	"github.com/qiangli/coreutils/pkg/webterm"
)

// serve starts the launcher on a real loopback port and returns its base URL.
func serve(t *testing.T, opts Options) string {
	t.Helper()
	opts.Ctx = context.Background()
	if opts.Terminal.Shell == nil && runtime.GOOS != "windows" {
		// A fixed, dependency-free shell: the point here is the plumbing, and a
		// login shell would drag in the operator's startup files.
		opts.Terminal.Shell = []string{"/bin/sh"}
	}

	h, closer, err := Handler(opts)
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		_ = closer()
	})

	base := "http://" + ln.Addr().String()
	// Do not proceed until it actually answers; a race here would be reported as
	// whatever assertion happened to run first.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(base + "/healthz"); err == nil {
			resp.Body.Close()
			return base
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("launcher never became ready")
	return ""
}

func get(t *testing.T, url string) (int, string, http.Header) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b := make([]byte, 1<<20)
	n, _ := resp.Body.Read(b)
	for n < len(b) {
		m, err := resp.Body.Read(b[n:])
		n += m
		if err != nil {
			break
		}
	}
	return resp.StatusCode, string(b[:n]), resp.Header
}

// ---------------------------------------------------------------- launcher --

func TestE2ELauncherServesItsApps(t *testing.T) {
	base := serve(t, Options{})

	code, body, _ := get(t, base+"/")
	if code != http.StatusOK {
		t.Fatalf("start page = %d", code)
	}
	if !strings.Contains(body, `id="grid-host"`) {
		t.Error("start page is not the launcher")
	}
	// Asset URLs must be content-versioned, or a browser can keep running the
	// previous build's script indefinitely (embed.FS has a zero modtime).
	if !strings.Contains(body, "app.js?v=") {
		t.Error("assets are not content-versioned; a cached script can outlive a fix")
	}

	code, body, _ = get(t, base+"/api/apps")
	if code != http.StatusOK {
		t.Fatalf("/api/apps = %d", code)
	}
	var apps struct {
		Schema string `json:"schema_version"`
		Apps   []struct {
			Name, Label, Path, Status, StartHint string
		} `json:"apps"`
	}
	if err := json.Unmarshal([]byte(body), &apps); err != nil {
		t.Fatalf("decode /api/apps: %v", err)
	}
	if apps.Schema != appsSchemaVersion {
		t.Errorf("schema = %q", apps.Schema)
	}

	want := map[string]bool{"terminal": false, "files": false, "relay": false}
	for _, a := range apps.Apps {
		if _, ok := want[a.Name]; ok {
			want[a.Name] = true
			if a.Status != StatusReady {
				t.Errorf("%s is %q, want ready", a.Name, a.Status)
			}
			// Every tile links somewhere that answers; the terminal once
			// advertised a path nothing was mounted at.
			if c, b, _ := get(t, base+a.Path); c != http.StatusOK {
				t.Errorf("%s advertises %s which serves %d", a.Name, a.Path, c)
			} else if strings.Contains(b, `id="grid-host"`) {
				t.Errorf("%s advertises %s but that fell through to the start page", a.Name, a.Path)
			}
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("%s missing from /api/apps", name)
		}
	}
}

func TestE2EAssetsRevalidate(t *testing.T) {
	base := serve(t, Options{})

	_, body, _ := get(t, base+"/")
	// Follow the versioned URL the page actually asks for.
	i := strings.Index(body, `src="app.js?v=`)
	if i < 0 {
		t.Fatal("no versioned app.js reference")
	}
	ref := body[i+len(`src="`):]
	ref = ref[:strings.IndexByte(ref, '"')]

	code, _, hdr := get(t, base+"/"+ref)
	if code != http.StatusOK {
		t.Fatalf("%s = %d", ref, code)
	}
	etag := hdr.Get("ETag")
	if etag == "" {
		t.Fatal("asset has no ETag")
	}

	req, _ := http.NewRequest("GET", base+"/"+ref, nil)
	req.Header.Set("If-None-Match", etag)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("conditional GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotModified {
		t.Errorf("conditional GET = %d, want 304", resp.StatusCode)
	}
}

// ------------------------------------------------------------------- files --

func TestE2EFilesServesItsScope(t *testing.T) {
	dir := t.TempDir()
	marker := "e2e-marker-file.txt"
	if err := os.WriteFile(filepath.Join(dir, marker), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := serve(t, Options{Scope: dir})

	code, body, _ := get(t, base+"/files/")
	if code != http.StatusOK {
		t.Fatalf("/files/ = %d", code)
	}
	if !strings.Contains(strings.ToLower(body), "file browser") {
		t.Error("/files/ did not serve File Browser")
	}
	// fbembed carries the mount in StaticURL rather than a <base href> — asserting
	// the wrong mechanism is how a passing test can describe a panel it never
	// actually checked. If this is absent the panel is loading its assets from
	// the launcher root and the page is broken under any prefix.
	if !strings.Contains(body, `"/files/static"`) {
		t.Error("/files/ did not rewrite StaticURL for its mount")
	}

	// And the scope must be the one we asked for: this panel already shipped once
	// defaulting to the server's working directory, which showed a different tree
	// per invocation.
	code, listing, _ := get(t, base+"/files/api/resources/")
	if code == http.StatusOK && !strings.Contains(listing, marker) {
		t.Errorf("scope %s does not list %s: %s", dir, marker, ellipsize(listing, 200))
	}
}

// ellipsize is the test-local shortener. It is NOT named truncate: pair_http.go
// declares a package-level truncate, and a same-package _test.go redeclaring it
// makes the whole e2e build fail — which is why the e2e tag had stopped
// compiling at all, taking the login round-trip guarantee with it.
func ellipsize(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ------------------------------------------------------------------- relay --

func TestE2ERelayServesTheRoom(t *testing.T) {
	base := serve(t, Options{})

	code, body, _ := get(t, base+"/meet/")
	if code != http.StatusOK {
		t.Fatalf("/meet/ = %d", code)
	}
	// Mounting under a prefix must rewrite the room's base href, or every asset
	// it requests resolves against the launcher instead.
	if !strings.Contains(body, `<base href="/meet/"`) {
		t.Errorf("/meet/ base href not rewritten for the mount")
	}

	// The room's own API must work through the mount. This is the regression
	// that matters most: stamping X-Forwarded-Prefix makes the room read the
	// request as cloud-vouched, and its default gate would 403 the machine owner.
	code, body, _ = get(t, base+"/meet/api/rooms")
	if code != http.StatusOK {
		t.Fatalf("/meet/api/rooms = %d (%s) — the mounted gate is wrong",
			code, strings.TrimSpace(body))
	}
	if !json.Valid([]byte(body)) {
		t.Errorf("/meet/api/rooms did not return JSON: %q", body)
	}
}

// ---------------------------------------------------------------- terminal --

func TestE2ETerminalRunsAShell(t *testing.T) {
	if !webterm.Supported() {
		t.Skip("no pseudo-console on this platform")
	}
	base := serve(t, Options{})

	// The page and the socket are different things at the same mount.
	code, body, _ := get(t, base+"/term/")
	if code != http.StatusOK {
		t.Fatalf("/term/ = %d", code)
	}
	if !strings.Contains(body, "term.js") {
		t.Error("/term/ did not serve the terminal page")
	}

	u := "ws" + strings.TrimPrefix(base, "http") + "/term/ws?cols=100&rows=30"
	c, resp, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		code := 0
		if resp != nil {
			code = resp.StatusCode
		}
		t.Fatalf("dial %s: %v (status %d)", u, err, code)
	}
	defer c.Close()

	if err := c.WriteMessage(websocket.BinaryMessage, []byte("echo E2E-TERMINAL-OK\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	var sb strings.Builder
	_ = c.SetReadDeadline(time.Now().Add(20 * time.Second))
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v (saw %q)", err, sb.String())
		}
		sb.Write(data)
		if strings.Contains(sb.String(), "E2E-TERMINAL-OK") {
			break
		}
	}

	// Resize is a control frame, not input, and must reach the pty.
	if err := c.WriteMessage(websocket.TextMessage,
		[]byte(`{"type":"size","cols":133,"rows":47}`)); err != nil {
		t.Fatalf("resize: %v", err)
	}
	if err := c.WriteMessage(websocket.BinaryMessage, []byte("stty size\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	sb.Reset()
	_ = c.SetReadDeadline(time.Now().Add(20 * time.Second))
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("read after resize: %v (saw %q)", err, sb.String())
		}
		sb.Write(data)
		if strings.Contains(sb.String(), "47 133") {
			return
		}
	}
}

// A terminal that hands out a shell must not accept an upgrade from another
// origin: the launcher can be bound to a LAN address.
func TestE2ETerminalRefusesCrossOrigin(t *testing.T) {
	if !webterm.Supported() {
		t.Skip("no pseudo-console on this platform")
	}
	base := serve(t, Options{})

	u := "ws" + strings.TrimPrefix(base, "http") + "/term/ws"
	_, resp, err := websocket.DefaultDialer.Dial(u, http.Header{"Origin": {"http://evil.example"}})
	if err == nil {
		t.Fatal("cross-origin upgrade accepted")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403, got %v", resp)
	}
}

// ------------------------------------------------------------------- login --

// stubAuth accepts one password, so the e2e can exercise the real gate, cookie
// and rate limiter without needing this machine's actual credentials.
type stubAuth struct{ password string }

func (s stubAuth) Authenticate(user, pass string) error {
	if pass == s.password {
		return nil
	}
	return hostauth.ErrInvalidCredentials
}

func TestE2ELoginGuardsAnExposedConsole(t *testing.T) {
	base := serve(t, Options{
		RequireLogin: true,
		Auth:         stubAuth{password: "correct-horse"},
		Sessions:     websession.NewStore(time.Hour, []byte("test-key-test-key-test-key-32byt")),
	})

	// Unauthenticated: the launcher and every app must be closed, and a browser
	// should be sent to the form rather than given a bare 403.
	jar := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	for _, p := range []string{"/", "/api/apps", "/term/", "/files/", "/meet/"} {
		req, _ := http.NewRequest("GET", base+p, nil)
		req.Header.Set("Accept", "text/html")
		resp, err := jar.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s is reachable without a session (%d)", p, resp.StatusCode)
		}
	}

	// The form itself must be reachable, or there is no way in.
	if code, body, _ := get(t, base+"/login"); code != http.StatusOK ||
		!strings.Contains(body, `name="password"`) ||
		!strings.Contains(body, `aria-label="Show password"`) ||
		!strings.Contains(body, `input.type = show ? "text" : "password"`) {
		t.Fatalf("/login = %d, no form", code)
	}

	// A wrong password does not mint a session.
	c := &http.Client{Jar: newJar(t), CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := c.PostForm(base+"/api/login", map[string][]string{
		"user": {currentOSUser()}, "password": {"wrong"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusSeeOther {
		t.Fatal("a wrong password was accepted")
	}

	// The right one does, and the session then opens the launcher and its apps.
	resp, err = c.PostForm(base+"/api/login", map[string][]string{
		"user": {currentOSUser()}, "password": {"correct-horse"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login returned %d, want 303", resp.StatusCode)
	}

	for _, p := range []string{"/", "/api/apps", "/files/", "/meet/api/rooms"} {
		req, _ := http.NewRequest("GET", base+p, nil)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s = %d after signing in", p, resp.StatusCode)
		}
	}

	// The session must be visible to the page, since that is what decides whether
	// a sign-out control is shown at all.
	req, _ := http.NewRequest("GET", base+"/api/session", nil)
	resp, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var sess struct{ User, Via string }
	_ = json.NewDecoder(resp.Body).Decode(&sess)
	resp.Body.Close()
	if sess.Via != "session" {
		t.Errorf("/api/session via = %q, want \"session\" — the header cannot know it is signed in", sess.Via)
	}

	// Signing out must actually revoke: a logout that only clears the cookie
	// leaves a still-valid token in anything that captured it.
	resp, err = c.PostForm(base+"/api/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("logout = %d, want 303", resp.StatusCode)
	}
	req, _ = http.NewRequest("GET", base+"/api/apps", nil)
	req.Header.Set("Accept", "text/html")
	resp, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("still authenticated after signing out")
	}
}

// The throttle is the only thing standing between a LAN peer and unlimited
// password guesses.
func TestE2ELoginIsRateLimited(t *testing.T) {
	base := serve(t, Options{
		RequireLogin: true,
		Auth:         stubAuth{password: "correct-horse"},
		Sessions:     websession.NewStore(time.Hour, []byte("test-key-test-key-test-key-32byt")),
	})

	limited := false
	for i := 0; i < 12; i++ {
		resp, err := http.PostForm(base+"/api/login", map[string][]string{
			"user": {currentOSUser()}, "password": {"wrong"},
		})
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("unlimited password attempts accepted")
	}
}

func newJar(t *testing.T) http.CookieJar {
	t.Helper()
	j, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return j
}
