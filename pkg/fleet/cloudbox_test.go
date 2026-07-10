package fleet

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/assetring"
)

// fakePlane serves the Bearer asset API with canned bodies.
func fakePlane(t *testing.T, bodies map[string]string) (CloudClient, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, ok := bodies[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return CloudClient{BaseURL: srv.URL, Token: "tok", HTTP: srv.Client()}, &calls
}

func TestResolveNeedsAToken(t *testing.T) {
	t.Setenv("BASHY_FLEET_TOKEN", "")
	t.Setenv("BASHY_API_KEY", "")
	_, err := CloudConfig{}.Resolve()
	if err == nil || !strings.Contains(err.Error(), "works fine without one") {
		t.Fatalf("err = %v — the message must say the registry does not need it", err)
	}

	t.Setenv("BASHY_FLEET_TOKEN", "abc")
	t.Setenv("BASHY_CLOUDBOX_URL", "https://box.example/")
	c, err := CloudConfig{}.Resolve()
	if err != nil || c.Token != "abc" || c.BaseURL != "https://box.example" {
		t.Fatalf("client = %+v, %v (trailing slash must be trimmed)", c, err)
	}
	// Flags beat the environment.
	c, _ = CloudConfig{URL: "https://flag.example", Token: "flagtok"}.Resolve()
	if c.BaseURL != "https://flag.example" || c.Token != "flagtok" {
		t.Fatalf("client = %+v", c)
	}
}

// A pulled model is rendered back into the canonical YAML this package writes,
// so an overlay entry is indistinguishable from a local one once cached.
func TestSyncModelsRendersStructuredColumns(t *testing.T) {
	client, _ := fakePlane(t, map[string]string{
		"/api/v1/models": `{"models":[
		  {"name":"deepseek-v4","display":"DeepSeek V4","kind":"api","source":"cloud",
		   "provider":"openai-compat","base_url":"https://api.deepseek.com",
		   "api_key_ref":"deepseek","model":"deepseek/deepseek-v4",
		   "capabilities":["completion","tools"],"domain":["coding"],
		   "context_length":128000,"price":1.5}]}`,
	})
	root := t.TempDir()
	res, err := client.Sync(CloudCacheRoot(root), dirModels)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 1 {
		t.Fatalf("fetched %d", res.Fetched)
	}

	c := New(WithRoot(root))
	m, ok := c.Model("deepseek-v4")
	if !ok {
		t.Fatal("the pulled model is not in the catalog")
	}
	if m.Ring != assetring.RingCloud {
		t.Fatalf("ring = %v, want cloud", m.Ring)
	}
	if m.Target() != "deepseek/deepseek-v4" {
		t.Fatalf("target = %q — the provider-side id must survive the round trip", m.Target())
	}
	if m.Kind != ModelKindAPI || m.APIKeyRef != "deepseek" || m.ContextLength != 128000 {
		t.Fatalf("structured columns lost: %+v", m)
	}
	if strings.Join(m.Capabilities, ",") != "completion,tools" {
		t.Fatalf("capabilities = %v", m.Capabilities)
	}
}

// The tool namespace is shared with function kits. A kit is not something the
// fleet can launch, so pulling one would list a name `verify` could only ever
// report as unusable.
func TestSyncToolsSkipsFunctionKits(t *testing.T) {
	client, _ := fakePlane(t, map[string]string{
		"/api/v1/tools": `{"tools":[
		  {"name":"codex","content":"name: codex\nkind: cli\ncli:\n  binary: codex\n  launch:\n    exec: codex --model {model} {prompt}\n"},
		  {"name":"ai","content":"name: ai\nkind: func\n"},
		  {"name":"legacy","content":"kit: legacy\ntype: cli\ncli:\n  binary: legacy\n  launch:\n    exec: legacy --model {model} {prompt}\n"}]}`,
	})
	root := t.TempDir()
	res, err := client.Sync(CloudCacheRoot(root), dirTools)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 2 || res.Skipped != 1 {
		t.Fatalf("fetched=%d skipped=%d — the func kit must be skipped and reported", res.Fetched, res.Skipped)
	}

	c := New(WithRoot(root))
	if _, ok := c.Tool("ai"); ok {
		t.Fatal("a function kit leaked into the tool ring")
	}
	// The legacy kit:/type: spelling still parses on the way in.
	legacy, ok := c.Tool("legacy")
	if !ok || !legacy.IsCLI() {
		t.Fatalf("legacy spelling rejected: %+v", legacy)
	}
}

// Precedence: the overlay beats the compiled-in baseline; the local store
// beats the overlay.
func TestOverlayRingSitsBetweenBaselineAndLocal(t *testing.T) {
	client, _ := fakePlane(t, map[string]string{
		"/api/v1/tools": `{"tools":[
		  {"name":"claude","content":"name: claude\nkind: cli\ncli:\n  binary: org-claude\n  launch:\n    exec: claude --model {model} {prompt}\n"}]}`,
	})
	root := t.TempDir()
	if _, err := client.Sync(CloudCacheRoot(root), dirTools); err != nil {
		t.Fatal(err)
	}

	c := New(WithRoot(root))
	tl, _ := c.Tool("claude")
	if tl.Ring != assetring.RingCloud || tl.CLI.Binary != "org-claude" {
		t.Fatalf("the org overlay must beat the baseline: %+v", tl)
	}

	// The operator's own entry beats the org.
	tl.CLI.Binary = "my-claude"
	if err := c.SaveTool(tl); err != nil {
		t.Fatal(err)
	}
	again, _ := c.Tool("claude")
	if again.Ring != assetring.RingLocal || again.CLI.Binary != "my-claude" {
		t.Fatalf("the local store must beat the org: %+v", again)
	}
}

// An unreachable control plane never breaks a read. Pairing enhances; it is
// not a gate.
func TestUnreachablePlaneDegradesToTheCachedRing(t *testing.T) {
	client, _ := fakePlane(t, map[string]string{
		"/api/v1/models": `{"models":[{"name":"orgmodel","kind":"subscription","model":"org-1"}]}`,
	})
	root := t.TempDir()
	if _, err := client.Sync(CloudCacheRoot(root), dirModels); err != nil {
		t.Fatal(err)
	}

	// The plane is now gone; the catalog still answers from the cache.
	dead := CloudClient{BaseURL: "http://127.0.0.1:1", Token: "tok", HTTP: client.HTTP}
	if _, err := dead.Sync(CloudCacheRoot(root), dirModels); err == nil {
		t.Fatal("sync must report an unreachable plane")
	}

	c := New(WithRoot(root))
	if m, ok := c.Model("orgmodel"); !ok || m.Target() != "org-1" {
		t.Fatalf("the cached ring stopped answering: %+v %v", m, ok)
	}
	// And the compiled-in baseline is untouched.
	if _, ok := c.Tool("codex"); !ok {
		t.Fatal("the baseline must answer with no network at all")
	}
}

// A pull REPLACES the noun's cache: an entry deleted upstream must disappear,
// and a partial merge would resurrect it forever.
func TestSyncReplacesTheCacheWholesale(t *testing.T) {
	root := t.TempDir()
	first, _ := fakePlane(t, map[string]string{
		"/api/v1/models": `{"models":[{"name":"a","kind":"subscription"},{"name":"b","kind":"subscription"}]}`,
	})
	if _, err := first.Sync(CloudCacheRoot(root), dirModels); err != nil {
		t.Fatal(err)
	}
	second, _ := fakePlane(t, map[string]string{
		"/api/v1/models": `{"models":[{"name":"a","kind":"subscription"}]}`,
	})
	if _, err := second.Sync(CloudCacheRoot(root), dirModels); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(CloudCacheRoot(root), dirModels, "b.yaml")); !os.IsNotExist(err) {
		t.Fatal("an entry deleted upstream survived the pull")
	}
}

// No pull ever landed: the overlay ring is simply absent.
func TestNoOverlayIsNotAnError(t *testing.T) {
	c := New(WithRoot(t.TempDir()))
	if tools, errs := c.Tools(false); len(errs) != 0 || len(tools) == 0 {
		t.Fatalf("an unpaired host reads its baseline: %d tools, errs=%v", len(tools), errs)
	}
}

// A 401 is reported, not silently swallowed into an empty catalog.
func TestSyncReportsAnAuthFailure(t *testing.T) {
	client, _ := fakePlane(t, map[string]string{"/api/v1/models": `{}`})
	client.Token = "wrong"
	_, err := client.Sync(CloudCacheRoot(t.TempDir()), dirModels)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}

// A name that would escape the store is dropped rather than written.
func TestSyncRejectsATraversalName(t *testing.T) {
	client, _ := fakePlane(t, map[string]string{
		"/api/v1/models": `{"models":[{"name":"../evil","kind":"subscription"},{"name":"ok","kind":"subscription"}]}`,
	})
	root := t.TempDir()
	res, err := client.Sync(CloudCacheRoot(root), dirModels)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 1 {
		t.Fatalf("fetched %d — the traversal name must be dropped", res.Fetched)
	}
}
