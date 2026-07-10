package fanout

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newBoard(t *testing.T) *Board {
	t.Helper()
	return Open(t.TempDir(), "b1")
}

// --- P0: the board substrate -------------------------------------------------

func TestSeedAndPostRoundTrip(t *testing.T) {
	b := newBoard(t)
	if err := b.Seed("build the widget", []string{"main.go"}, "alice"); err != nil {
		t.Fatal(err)
	}
	txt, by, refs, err := b.SeedText()
	if err != nil {
		t.Fatal(err)
	}
	if txt != "build the widget" || by != "alice" || len(refs) != 1 {
		t.Fatalf("seed round-trip: %q %q %v", txt, by, refs)
	}

	if err := b.Post("finding one", "007", "risk", []string{"risk"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := b.Post("finding two", "codex", "perf", nil, ""); err != nil {
		t.Fatal(err)
	}
	posts, _ := b.Contributions()
	if len(posts) != 2 {
		t.Fatalf("want 2 posts, got %d", len(posts))
	}
}

func TestRePostIsIdempotent(t *testing.T) {
	b := newBoard(t)
	_ = b.Post("same", "007", "risk", nil, "")
	_ = b.Post("same", "007", "risk", nil, "") // identical author+scope+text → same id
	posts, _ := b.Contributions()
	if len(posts) != 1 {
		t.Fatalf("idempotent re-post should collapse: got %d", len(posts))
	}
}

func TestStatus(t *testing.T) {
	b := newBoard(t)
	_ = b.Post("a", "007", "x", nil, "")
	_ = b.Post("b", "007", "y", nil, "")
	_ = b.Post("c", "codex", "z", nil, "")
	byAuthor, total, _ := b.Status()
	if total != 3 || byAuthor["007"] != 2 || byAuthor["codex"] != 1 {
		t.Fatalf("status = %v total=%d", byAuthor, total)
	}
}

// --- P2: scoped reads (context-pollution mitigation) -------------------------

func TestReadUnscopedReturnsAll(t *testing.T) {
	b := newBoard(t)
	_ = b.Seed("goal", nil, "alice")
	_ = b.Post("p1", "007", "risk", nil, "")
	_ = b.Post("p2", "codex", "perf", nil, "")
	v, err := b.Read("", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if v.Seed != "goal" || len(v.Posts) != 2 || v.Scoped {
		t.Fatalf("unscoped view = %+v", v)
	}
}

func TestReadScopedRanksAndExcludesSelf(t *testing.T) {
	b := newBoard(t)
	_ = b.Seed("goal", nil, "alice")
	_ = b.Post("cache invalidation is the perf bottleneck", "codex", "perf", []string{"perf"}, "")
	_ = b.Post("null deref risk in the parser", "aider", "risk", []string{"risk"}, "")
	_ = b.Post("my own note", "007", "risk", nil, "") // self — must be excluded for reader 007

	v, err := b.Read("perf", "007", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Scoped {
		t.Fatal("expected a scoped view")
	}
	// self's post is excluded from Total and Posts.
	for _, p := range v.Posts {
		if p.By == "007" {
			t.Fatal("reader's own post leaked into its scoped view")
		}
	}
	// the perf post (exact scope tag) ranks first.
	if len(v.Posts) == 0 || !strings.Contains(v.Posts[0].Text, "perf bottleneck") {
		t.Fatalf("scope filter did not surface the perf post first: %+v", v.Posts)
	}
	// the unrelated risk post (different scope, zero term-overlap with "perf")
	// is DROPPED, not just ranked last — the view is genuinely narrower.
	for _, p := range v.Posts {
		if strings.Contains(p.Text, "null deref") {
			t.Fatal("scoped view leaked the unrelated risk stream (context pollution)")
		}
	}
}

func TestReadLimitCaps(t *testing.T) {
	b := newBoard(t)
	for i := 0; i < 5; i++ {
		_ = b.Post("post", "a"+itoa(i), "s", nil, "") // distinct authors → distinct ids
	}
	v, _ := b.Read("", "", 2)
	if len(v.Posts) != 2 {
		t.Fatalf("limit not applied: %d", len(v.Posts))
	}
}

// --- P1: the fan-out orchestrator --------------------------------------------

// fakeLauncher records concurrency and returns a scripted output per agent.
type fakeLauncher struct {
	inflight, peak int32
	outputs        map[string]string
}

func (f *fakeLauncher) Launch(ctx context.Context, agent, prompt string, timeout time.Duration) (string, int, error) {
	n := atomic.AddInt32(&f.inflight, 1)
	for {
		p := atomic.LoadInt32(&f.peak)
		if n <= p || atomic.CompareAndSwapInt32(&f.peak, p, n) {
			break
		}
	}
	time.Sleep(5 * time.Millisecond) // widen the concurrency window
	atomic.AddInt32(&f.inflight, -1)
	out := f.outputs[agent]
	if out == "" {
		out = "output from " + agent
	}
	return out, 0, nil
}

func TestRunFansOutConcurrentlyAndPosts(t *testing.T) {
	b := newBoard(t)
	_ = b.Seed("shared goal", nil, "alice")
	instances := []Instance{
		{Agent: "007", Instruction: "angle A", Scope: "a"},
		{Agent: "codex", Instruction: "angle B", Scope: "b"},
		{Agent: "aider", Instruction: "angle C", Scope: "c"},
	}
	fl := &fakeLauncher{outputs: map[string]string{
		"007": "finding A", "codex": "finding B", "aider": "finding C",
	}}
	results, err := Run(context.Background(), Config{Board: b, Instances: instances, Jobs: 3}, fl)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("want 3 results, got %d", len(results))
	}
	// Ran concurrently.
	if fl.peak < 2 {
		t.Fatalf("expected concurrency, peak inflight = %d", fl.peak)
	}
	// Each output was posted to the board under its agent+scope.
	posts, _ := b.Contributions()
	if len(posts) != 3 {
		t.Fatalf("want 3 posts on the board, got %d", len(posts))
	}
	byScope := map[string]string{}
	for _, p := range posts {
		byScope[p.Scope] = p.Text
	}
	if byScope["a"] != "finding A" || byScope["b"] != "finding B" || byScope["c"] != "finding C" {
		t.Fatalf("posts not attributed by scope: %v", byScope)
	}
	// Results preserve input order.
	if results[0].Agent != "007" || results[2].Agent != "aider" {
		t.Fatalf("result order not preserved: %+v", results)
	}
}

func TestRunJobsBoundsConcurrency(t *testing.T) {
	b := newBoard(t)
	_ = b.Seed("g", nil, "alice")
	var instances []Instance
	for i := 0; i < 6; i++ {
		instances = append(instances, Instance{Agent: "a" + itoa(i), Instruction: "x", Scope: "s" + itoa(i)})
	}
	fl := &fakeLauncher{}
	if _, err := Run(context.Background(), Config{Board: b, Instances: instances, Jobs: 2}, fl); err != nil {
		t.Fatal(err)
	}
	if fl.peak > 2 {
		t.Fatalf("--jobs 2 exceeded: peak = %d", fl.peak)
	}
}

func TestRunValidates(t *testing.T) {
	b := newBoard(t)
	if _, err := Run(context.Background(), Config{Board: b, Instances: nil}, &fakeLauncher{}); err == nil {
		t.Error("empty instances must error")
	}
	if _, err := Run(context.Background(), Config{Board: nil, Instances: []Instance{{Agent: "x"}}}, &fakeLauncher{}); err == nil {
		t.Error("nil board must error")
	}
}

// --- pairing -----------------------------------------------------------------

func TestPairInstancesRoundRobinAndScopePrefix(t *testing.T) {
	inst := pairInstances([]string{"007", "codex"}, []string{
		"risk: look for null derefs",
		"just do the thing", // no scope prefix → lens-2
		"perf: profile the hot path",
	})
	if len(inst) != 3 {
		t.Fatalf("want 3 instances, got %d", len(inst))
	}
	if inst[0].Agent != "007" || inst[1].Agent != "codex" || inst[2].Agent != "007" {
		t.Fatalf("round-robin wrong: %v", []string{inst[0].Agent, inst[1].Agent, inst[2].Agent})
	}
	if inst[0].Scope != "risk" || inst[0].Instruction != "look for null derefs" {
		t.Fatalf("scope prefix not parsed: %+v", inst[0])
	}
	if inst[1].Scope != "lens-2" || inst[1].Instruction != "just do the thing" {
		t.Fatalf("default scope wrong: %+v", inst[1])
	}
	if inst[2].Scope != "perf" {
		t.Fatalf("third scope wrong: %+v", inst[2])
	}
}

func TestPairInstancesExplicitToolFacetBinding(t *testing.T) {
	// The practical division-of-labor form: each line binds its own tool.
	inst := pairInstances([]string{"codex", "agy", "opencode"}, []string{
		"codex code: implement the limiter",
		"agy tests: write table tests",
		"opencode testdata: provide edge-case JSON",
		"codex: refactor",        // agent with trailing colon, no scope
		"perf: profile the path", // no agent → round-robin
	})
	want := []struct{ agent, scope, text string }{
		{"codex", "code", "implement the limiter"},
		{"agy", "tests", "write table tests"},
		{"opencode", "testdata", "provide edge-case JSON"},
		{"codex", "lens-4", "refactor"},
		{"codex", "perf", "profile the path"}, // round-robin lands on codex (rr=0)
	}
	if len(inst) != len(want) {
		t.Fatalf("want %d, got %d", len(want), len(inst))
	}
	for i, w := range want {
		if inst[i].Agent != w.agent || inst[i].Scope != w.scope || inst[i].Instruction != w.text {
			t.Errorf("inst[%d] = {%q %q %q}, want {%q %q %q}",
				i, inst[i].Agent, inst[i].Scope, inst[i].Instruction, w.agent, w.scope, w.text)
		}
	}
}

// --- P-staging: dependency waves ---------------------------------------------

func TestComputeWavesTopological(t *testing.T) {
	inst := []Instance{
		{Scope: "code"},
		{Scope: "tests", Needs: []string{"code"}},
		{Scope: "testdata", Needs: []string{"code"}},
		{Scope: "review", Needs: []string{"tests", "testdata"}},
	}
	waves, err := computeWaves(inst)
	if err != nil {
		t.Fatal(err)
	}
	if len(waves) != 3 {
		t.Fatalf("want 3 waves, got %d: %v", len(waves), waves)
	}
	if len(waves[0]) != 1 || waves[0][0] != 0 {
		t.Fatalf("wave 0 should be just code: %v", waves[0])
	}
	if len(waves[1]) != 2 { // tests + testdata in parallel
		t.Fatalf("wave 1 should be tests+testdata: %v", waves[1])
	}
	if len(waves[2]) != 1 || waves[2][0] != 3 {
		t.Fatalf("wave 2 should be review: %v", waves[2])
	}
}

func TestComputeWavesErrors(t *testing.T) {
	if _, err := computeWaves([]Instance{
		{Scope: "a", Needs: []string{"b"}},
		{Scope: "b", Needs: []string{"a"}},
	}); err == nil {
		t.Error("cycle must error")
	}
	if _, err := computeWaves([]Instance{
		{Scope: "tests", Needs: []string{"code"}}, // no producer of "code"
	}); err == nil {
		t.Error("missing dependency scope must error")
	}
}

// A dependent instance runs AFTER its dependency and sees its contribution.
func TestRunStagedFeedsDependencyOutput(t *testing.T) {
	b := newBoard(t)
	_ = b.Seed("build a thing", nil, "alice")
	inst := []Instance{
		{Agent: "codex", Instruction: "write the code", Scope: "code"},
		{Agent: "agy", Instruction: "write tests for the code", Scope: "tests", Needs: []string{"code"}},
	}
	// The fake launcher captures the prompt the tests agent receives, so we can
	// assert it contained the code agent's posted output.
	var testsPrompt string
	fl := launcherFunc(func(ctx context.Context, agent, prompt string, _ time.Duration) (string, int, error) {
		if agent == "agy" {
			testsPrompt = prompt
		}
		return "output from " + agent, 0, nil
	})
	if _, err := Run(context.Background(), Config{Board: b, Instances: inst}, fl); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(testsPrompt, "output from codex") {
		t.Fatalf("tests agent did not receive the code agent's contribution:\n%s", testsPrompt)
	}
	if !strings.Contains(testsPrompt, "prior work you depend on (code)") {
		t.Fatalf("dependency section missing from tests prompt:\n%s", testsPrompt)
	}
}

func TestPairInstancesAfterDependency(t *testing.T) {
	inst := pairInstances([]string{"codex", "agy", "opencode"}, []string{
		"codex code: implement it",
		"agy tests after code: test it",
		"opencode testdata after code,tests: make data",
	})
	if len(inst[0].Needs) != 0 {
		t.Fatalf("code should have no deps: %v", inst[0].Needs)
	}
	if inst[1].Scope != "tests" || len(inst[1].Needs) != 1 || inst[1].Needs[0] != "code" {
		t.Fatalf("tests dep parse: %+v", inst[1])
	}
	if inst[1].Instruction != "test it" {
		t.Fatalf("tests instruction: %q", inst[1].Instruction)
	}
	if inst[2].Scope != "testdata" || len(inst[2].Needs) != 2 {
		t.Fatalf("testdata deps: %+v", inst[2])
	}
}

// launcherFunc adapts a func to the Launcher interface.
type launcherFunc func(context.Context, string, string, time.Duration) (string, int, error)

func (f launcherFunc) Launch(ctx context.Context, a, p string, to time.Duration) (string, int, error) {
	return f(ctx, a, p, to)
}
