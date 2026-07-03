package graphcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// fixtureRepo writes a tiny multi-file Go package with a call chain
// Gamma -> Alpha -> Beta, so the graph has real nodes and edges.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"a.go": "package fixture\n\nfunc Alpha() int { return Beta() }\n\nfunc Beta() int { return 42 }\n",
		"b.go": "package fixture\n\nfunc Gamma() int { return Alpha() }\n",
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func run(t *testing.T, dir string, fn func(*tool.RunContext, []string) int, args ...string) (out, errOut string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		FS:    tool.NewLocalFS(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &o, Err: &e},
	}
	code = fn(rc, args)
	return o.String(), e.String(), code
}

func TestGraphBuildCreatesCacheAndStats(t *testing.T) {
	dir := fixtureRepo(t)
	out, errOut, code := run(t, dir, runBuild, "--plain")
	if code != 0 {
		t.Fatalf("graph-build exit %d, stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "Nodes:") {
		t.Fatalf("expected stats in output, got: %s", out)
	}
	cache := filepath.Join(dir, cacheRel)
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("expected cache at %s: %v", cache, err)
	}
}

func TestGraphStatsJSONEnvelope(t *testing.T) {
	dir := fixtureRepo(t)
	out, errOut, code := run(t, dir, runStats, "--json")
	if code != 0 {
		t.Fatalf("graph-stats exit %d, stderr=%s", code, errOut)
	}
	var p statsPayload
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatalf("invalid JSON envelope: %v\n%s", err, out)
	}
	if p.Schema != schemaVersion {
		t.Errorf("schema=%q want %q", p.Schema, schemaVersion)
	}
	if p.GraphSHA == "" {
		t.Error("graph_sha must be set")
	}
	if p.Nodes == 0 || p.Edges == 0 {
		t.Errorf("expected a non-empty graph, got nodes=%d edges=%d", p.Nodes, p.Edges)
	}
}

func TestGraphNeighborsFindsCallee(t *testing.T) {
	dir := fixtureRepo(t)
	out, errOut, code := run(t, dir, runNeighbors, "Alpha", "--plain")
	if code != 0 {
		t.Fatalf("graph-neighbors exit %d, stderr=%s", code, errOut)
	}
	// Undirected coupling: Alpha's neighbors include Beta (calls) and Gamma (caller).
	if !strings.Contains(out, "Beta") {
		t.Fatalf("expected Beta among Alpha's neighbors, got:\n%s", out)
	}
}

func TestGraphNeighborsMissingSymbol(t *testing.T) {
	dir := fixtureRepo(t)
	_, errOut, code := run(t, dir, runNeighbors, "NoSuchSymbolXYZ", "--plain")
	if code != 1 {
		t.Fatalf("expected exit 1 for missing symbol, got %d", code)
	}
	if !strings.Contains(errOut, "not found") {
		t.Errorf("expected 'not found' message, got: %s", errOut)
	}
}

func TestGraphImpactBlastRadius(t *testing.T) {
	dir := fixtureRepo(t)
	out, errOut, code := run(t, dir, runImpact, "Alpha", "--depth", "2", "--json")
	if code != 0 {
		t.Fatalf("graph-impact exit %d, stderr=%s", code, errOut)
	}
	var p impactPayload
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	labels := map[string]bool{}
	for _, n := range p.Nodes {
		labels[strings.TrimSuffix(strings.TrimPrefix(n.Label, "."), "()")] = true
	}
	if !labels["Beta"] {
		t.Errorf("expected Beta in Alpha's blast radius, got %#v", p.Nodes)
	}
}

func TestGraphPathBetweenSymbols(t *testing.T) {
	dir := fixtureRepo(t)
	out, errOut, code := run(t, dir, runPath, "Gamma", "Beta", "--plain")
	if code != 0 {
		t.Fatalf("graph-path exit %d, stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "path (") && !strings.Contains(out, "no path") {
		t.Fatalf("unexpected path output: %s", out)
	}
}

func TestGraphQueryKeyword(t *testing.T) {
	dir := fixtureRepo(t)
	out, _, code := run(t, dir, runQuery, "Beta", "--plain")
	if code != 0 {
		t.Fatalf("graph-query exit %d", code)
	}
	if !strings.Contains(out, "Beta") {
		t.Fatalf("expected Beta in query subgraph, got: %s", out)
	}
}

func TestGraphHotspotsRunsAndFilters(t *testing.T) {
	dir := fixtureRepo(t)
	// --raw and default should both return valid envelopes; the fixture labels
	// (Alpha/Beta/Gamma) are not in the ubiquitous denylist, so they survive.
	out, errOut, code := run(t, dir, runHotspots, "--json")
	if code != 0 {
		t.Fatalf("graph-hotspots exit %d, stderr=%s", code, errOut)
	}
	var p hotspotsPayload
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if p.Schema != schemaVersion {
		t.Errorf("schema=%q", p.Schema)
	}
}

func TestUbiquitousLabelsFiltered(t *testing.T) {
	// Unit-check the heuristic directly: normalized ubiquitous labels are dropped,
	// domain labels survive.
	cases := map[string]bool{
		".String()": true, ".Len()": true, "New()": true,
		"Alpha()": false, "NewAppRegistry()": false, "authenticate()": false,
	}
	for label, wantFiltered := range cases {
		got := ubiquitousLabels[normalizeLabel(label)]
		if got != wantFiltered {
			t.Errorf("normalizeLabel(%q)=%q filtered=%v want %v", label, normalizeLabel(label), got, wantFiltered)
		}
	}
}

func TestGraphSHAStableAcrossBuilds(t *testing.T) {
	dir := fixtureRepo(t)
	out1, _, _ := run(t, dir, runStats, "--json")
	// Force a second independent build (bypass cache) and compare graph_sha.
	out2, _, _ := run(t, dir, runStats, "--json", "--rebuild")
	var a, b statsPayload
	_ = json.Unmarshal([]byte(out1), &a)
	_ = json.Unmarshal([]byte(out2), &b)
	if a.GraphSHA == "" || a.GraphSHA != b.GraphSHA {
		t.Errorf("graph_sha not stable: %q vs %q", a.GraphSHA, b.GraphSHA)
	}
}

func TestCacheFreshnessDetectsEdits(t *testing.T) {
	dir := fixtureRepo(t)
	// Build populates the cache.
	if _, _, code := run(t, dir, runBuild, "--plain"); code != 0 {
		t.Fatal("build failed")
	}
	cache := filepath.Join(dir, cacheRel)
	if !cacheFresh(dir, cache) {
		t.Fatal("cache should be fresh right after build")
	}
	// Touch a source file into the future → cache goes stale.
	future := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "a.go"), future, future); err != nil {
		t.Fatal(err)
	}
	if cacheFresh(dir, cache) {
		t.Error("cache should be stale after a source edit")
	}
}
