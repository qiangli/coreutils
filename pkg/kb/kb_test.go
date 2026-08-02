package kb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// run executes the kb CLI against a store dir and returns stdout.
func run(t *testing.T, dir string, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := NewKBCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(append([]string{"--dir", dir}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func mustRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := run(t, dir, "", args...)
	if err != nil {
		t.Fatalf("kb %s: %v", strings.Join(args, " "), err)
	}
	return out
}

func TestPageRoundTrip(t *testing.T) {
	p := &Page{
		Slug: "cp-signed-binary", Type: TypeGotcha,
		Title:       "cp over a live signed binary kills it",
		Description: "WHEN overwriting a running signed binary on macOS",
		Tags:        []string{"macos", "codesign"},
		Scope:       &Scope{Repos: []string{"outpost"}, OS: "darwin"},
		Status:      StatusCandidate,
		Evidence:    "reproduced on darwin-arm64",
		Source:      &Source{Tool: "claude", Host: "host-a"},
		Body:        "rm first, then cp, then codesign.\n\nSee [other-page.md].",
	}
	b, err := p.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParsePage("cp-signed-binary", b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != p.Title || got.Description != p.Description || got.Type != p.Type {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.Scope == nil || got.Scope.OS != "darwin" || len(got.Scope.Repos) != 1 {
		t.Fatalf("scope lost: %+v", got.Scope)
	}
	if got.Body != p.Body {
		t.Fatalf("body mismatch: %q != %q", got.Body, p.Body)
	}
	// A page without optional fields keeps its frontmatter minimal.
	min := &Page{Slug: "x", Type: TypeFact, Title: "t", Description: "d"}
	mb, err := min.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"scope:", "source:", "supersedes", "evidence"} {
		if strings.Contains(string(mb), absent) {
			t.Fatalf("minimal page leaked %q:\n%s", absent, mb)
		}
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"cp over a LIVE signed Mach-O":   "cp-over-a-live-signed-mach-o",
		"  --weird?? punctuation!!  ":    "weird-punctuation",
		"":                               "page",
		strings.Repeat("very long ", 20): "very-long-very-long-very-long-very-long-very-long-very-long-very",
	} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAddSearchShow(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add",
		"--type", "gotcha",
		"--title", "cp over a live signed binary kills it",
		"--description", "WHEN overwriting a running signed binary on macOS rm before cp",
		"--tags", "macos,codesign",
		"--body", "rm first, then cp, then codesign --force.")

	out := mustRun(t, dir, "search", "codesign")
	if !strings.Contains(out, "cp-over-a-live-signed-binary-kills-it") {
		t.Fatalf("search missed the page:\n%s", out)
	}
	if !strings.Contains(out, "[candidate/gotcha]") {
		t.Fatalf("search line missing status/type:\n%s", out)
	}

	show := mustRun(t, dir, "show", "cp-over-a-live-signed-binary-kills-it")
	if !strings.Contains(show, "codesign --force") {
		t.Fatalf("show missing body:\n%s", show)
	}

	// index.md regenerated as the always-load surface.
	idx, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(idx), "cp-over-a-live-signed-binary-kills-it") {
		t.Fatalf("index.md missing the page:\n%s", idx)
	}

	// journal has the add.
	log := mustRun(t, dir, "log")
	if !strings.Contains(log, `"op":"add"`) {
		t.Fatalf("journal missing add:\n%s", log)
	}
}

func TestAddReconcilesDuplicates(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--title", "weave workspaces lack external ollama",
		"--description", "WHEN launching fleet workers that need models")
	_, err := run(t, dir, "", "add", "--title", "weave workspaces lack the external ollama",
		"--description", "fleet workers that need models WHEN launching")
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate refusal, got %v", err)
	}
	// --force overrides.
	if _, err := run(t, dir, "", "add", "--force", "--title", "weave workspaces lack the external ollama",
		"--description", "fleet workers that need models WHEN launching"); err != nil {
		t.Fatalf("--force should override: %v", err)
	}
}

func TestSupersedeAndValidateLadder(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--title", "always use pkill for stuck agents",
		"--description", "WHEN an agent process hangs")
	mustRun(t, dir, "supersede", "always-use-pkill-for-stuck-agents",
		"--title", "never pkill on an outpost host",
		"--description", "WHEN an agent process hangs find the exact pid instead")

	store := Open(dir)
	old, err := store.Load("always-use-pkill-for-stuck-agents")
	if err != nil {
		t.Fatal(err)
	}
	if old.Status != StatusSuperseded || old.SupersededBy != "never-pkill-on-an-outpost-host" {
		t.Fatalf("old page not linked: %+v", old)
	}
	neu, err := store.Load("never-pkill-on-an-outpost-host")
	if err != nil {
		t.Fatal(err)
	}
	if neu.Supersedes != old.Slug {
		t.Fatalf("new page not linked back: %+v", neu)
	}

	// Superseded pages drop out of default search; --all shows them.
	out := mustRun(t, dir, "search", "pkill")
	if strings.Contains(out, "always-use-pkill") {
		t.Fatalf("superseded page leaked into default search:\n%s", out)
	}
	all := mustRun(t, dir, "search", "--all", "--k", "10", "pkill")
	if !strings.Contains(all, "always-use-pkill") {
		t.Fatalf("--all should include superseded:\n%s", all)
	}

	// validate requires evidence and flips the status.
	if _, err := run(t, dir, "", "validate", "never-pkill-on-an-outpost-host"); err == nil {
		t.Fatal("validate without --evidence should fail")
	}
	mustRun(t, dir, "validate", "never-pkill-on-an-outpost-host", "--evidence", "outpost daemon survived; issue #42")
	neu, _ = store.Load("never-pkill-on-an-outpost-host")
	if neu.Status != StatusValidated || neu.Evidence == "" {
		t.Fatalf("validate did not stick: %+v", neu)
	}
}

func TestSearchScopingAndRanking(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--title", "darwin only trick", "--description", "codesign dance", "--os", "darwin")
	mustRun(t, dir, "add", "--title", "windows only trick", "--description", "codesign equivalent on windows", "--os", "windows")
	mustRun(t, dir, "add", "--title", "repo scoped trick", "--description", "codesign in one repo", "--repos", "outpost")

	out := mustRun(t, dir, "search", "--os", "darwin", "--repo", "bashy", "--k", "10", "codesign")
	if !strings.Contains(out, "darwin-only-trick") {
		t.Fatalf("darwin page missing:\n%s", out)
	}
	if strings.Contains(out, "windows-only-trick") {
		t.Fatalf("windows-scoped page should be filtered:\n%s", out)
	}
	if strings.Contains(out, "repo-scoped-trick") {
		t.Fatalf("other-repo page should be filtered:\n%s", out)
	}
	// Matching repo passes the scope filter.
	out = mustRun(t, dir, "search", "--os", "darwin", "--repo", "outpost", "--k", "10", "codesign")
	if !strings.Contains(out, "repo-scoped-trick") {
		t.Fatalf("repo-scoped page should show for its repo:\n%s", out)
	}

	// Validated ranks above candidate on equal matches; K caps output.
	mustRun(t, dir, "validate", "darwin-only-trick", "--evidence", "e2e")
	hits := Search(mustList(t, dir), Query{Terms: []string{"codesign"}, OS: "darwin", Repo: "outpost", K: 1})
	if len(hits) != 1 || hits[0].Page.Slug != "darwin-only-trick" {
		t.Fatalf("validated page should win k=1: %+v", hits)
	}
}

func mustList(t *testing.T, dir string) []*Page {
	t.Helper()
	pages, err := Open(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	return pages
}

func TestSearchJSONIsTokenLean(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--title", "json check", "--description", "WHEN asserting json output")
	out := mustRun(t, dir, "search", "--json", "json")
	var payload struct {
		Pages []searchHitJSON `json:"pages"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out)
	}
	if len(payload.Pages) != 1 || payload.Pages[0].Slug != "json-check" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	store := Open(dir)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := &Page{
				Slug: fmt.Sprintf("page-%d", i), Type: TypeFact,
				Title: fmt.Sprintf("page %d", i), Description: "concurrent",
			}
			if err := store.Write(p, "add"); err != nil {
				t.Errorf("write %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	pages := mustList(t, dir)
	if len(pages) != 8 {
		t.Fatalf("want 8 pages, got %d", len(pages))
	}
	lines, err := store.JournalTail(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 8 {
		t.Fatalf("want 8 journal lines, got %d", len(lines))
	}
	// Every journal line is intact JSON (O_APPEND kept lines whole).
	for _, l := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(l), &rec); err != nil {
			t.Fatalf("torn journal line %q: %v", l, err)
		}
	}
}

func TestIndexOrdering(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--title", "bbb candidate", "--description", "d")
	mustRun(t, dir, "add", "--title", "aaa validated", "--description", "d")
	mustRun(t, dir, "validate", "aaa-validated", "--evidence", "e")
	idx, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(idx)
	if strings.Index(s, "aaa-validated") > strings.Index(s, "bbb-candidate") {
		t.Fatalf("validated should list first:\n%s", s)
	}
}

func TestFederatedSearch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// os.UserHomeDir reads %USERPROFILE% on windows, not HOME.
	t.Setenv("USERPROFILE", home)
	repo := filepath.Join(home, "src", "myrepo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Repo graph contribution log with a live note and a forgotten one.
	gdir := filepath.Join(repo, ".agents", "bashy", "graph")
	if err := os.MkdirAll(gdir, 0o755); err != nil {
		t.Fatal(err)
	}
	contrib := `{"id":"n1","op":"note","target":"pkg/x","text":"flaky test needs serial run"}
{"id":"n2","op":"note","target":"pkg/y","text":"forgotten flaky note"}
{"id":"f1","op":"forget","forget_id":"n2"}
`
	if err := os.WriteFile(filepath.Join(gdir, "contrib.jsonl"), []byte(contrib), 0o644); err != nil {
		t.Fatal(err)
	}

	// Weave campaign memory under the fnv-tagged queue dir.
	h := fnv.New32a()
	_, _ = h.Write([]byte(repo))
	qdir := filepath.Join(home, ".bashy", "weave", fmt.Sprintf("%s-%08x", "myrepo", h.Sum32()))
	if err := os.MkdirAll(qdir, 0o755); err != nil {
		t.Fatal(err)
	}
	obs := `{"issue_id":7,"title":"fix flaky suite","tool":"codex","outcome":"merged","summary":"root cause was test pollution"}
`
	if err := os.WriteFile(filepath.Join(qdir, "memory.jsonl"), []byte(obs), 0o644); err != nil {
		t.Fatal(err)
	}

	hits := FederatedSearch(filepath.Join(repo, "sub", "dir2"), []string{"flaky"}, 5)
	// subdir doesn't exist on disk but repoRootOf only stats upward; create it.
	if len(hits) == 0 {
		if err := os.MkdirAll(filepath.Join(repo, "sub", "dir2"), 0o755); err != nil {
			t.Fatal(err)
		}
		hits = FederatedSearch(filepath.Join(repo, "sub", "dir2"), []string{"flaky"}, 5)
	}
	var origins []string
	for _, h := range hits {
		origins = append(origins, h.Origin)
		if strings.Contains(h.Text, "forgotten") {
			t.Fatalf("forgotten note leaked: %+v", h)
		}
	}
	joined := strings.Join(origins, ",")
	if !strings.Contains(joined, "repo-graph") || !strings.Contains(joined, "weave-memory") {
		t.Fatalf("want both origins, got %v (%+v)", origins, hits)
	}
}

func TestRetroTemplate(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--title", "retro subject", "--description", "WHEN retro runs")
	out := mustRun(t, dir, "retro", "retro")
	for _, want := range []string{"retro-subject", "ADD", "UPDATE", "SUPERSEDE", "VALIDATE", "NOOP"} {
		if !strings.Contains(out, want) {
			t.Fatalf("retro output missing %q:\n%s", want, out)
		}
	}
}

// TestSearchTokenizesQuotedQuery is the G1 regression guard
// (docs/kb-usage-gaps-fix-plan.md): a quoted, task-shaped query must be
// tokenized through Terms() so it matches per word. Before the fix the CLI
// passed raw argv as Query.Terms, so a single 5-word term matched nothing.
func TestSearchTokenizesQuotedQuery(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--type", "lesson",
		"--title", "How to gate a merge",
		"--description", "run make test-bash 86/86 before merging any weave branch")

	out := mustRun(t, dir, "search", "how do I gate a merge")
	if strings.Contains(out, "no matching kb pages") || strings.TrimSpace(out) == "" {
		t.Fatalf("quoted task-shaped query returned nothing (G1 regression):\n%s", out)
	}
	if !strings.Contains(out, "gate-a-merge") {
		t.Fatalf("expected the page in results, got:\n%s", out)
	}
}

func TestGitSnapshotBestEffort(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--title", "git snap", "--description", "d")
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Skipf("git snapshot unavailable: %v", err) // best-effort by contract
	}
	mustRun(t, dir, "update", "git-snap", "--evidence", "e2")
	// Two mutations → the store repo has history (HEAD exists).
	if _, err := os.Stat(filepath.Join(dir, ".git", "HEAD")); err != nil {
		t.Fatalf("git HEAD missing after writes: %v", err)
	}
}

// TestSearchIsWordAnchored guards the tokenisation half of the BM25 port: the
// substring scorer it replaced matched INSIDE words, so a query for "rust"
// scored every page containing "trust". Measured as a real false-recall source
// on the host store (dhnt/docs/memory-eval-plan.md §4d).
func TestSearchIsWordAnchored(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--title", "trust prompt hangs the agent",
		"--description", "WHEN a headless agent waits at a directory trust prompt")
	hits := Search(mustList(t, dir), Query{Terms: []string{"rust"}, K: 5})
	if len(hits) != 0 {
		t.Fatalf("`rust` must not match inside `trust`: %+v", hits)
	}
	if hits := Search(mustList(t, dir), Query{Terms: []string{"trust"}, K: 5}); len(hits) != 1 {
		t.Fatalf("the whole word must still match: %+v", hits)
	}
}

// TestSearchLengthNormalisation guards the ranking half: the substring scorer
// counted raw occurrences, so a long page beat a short exact match. Measured:
// on 36 real pages the top hit for an unrelated query sat in the 87th
// percentile by length.
func TestSearchLengthNormalisation(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--title", "cgroup weights", "--description", "WHEN sharing a host between workloads")
	mustRun(t, dir, "add", "--title", "unrelated long page", "--description", "WHEN reading a very long page",
		"--body", strings.Repeat("weights of many other things plus filler prose ", 60))
	hits := Search(mustList(t, dir), Query{Terms: []string{"cgroup", "weights"}, K: 2})
	if len(hits) == 0 || hits[0].Page.Slug != "cgroup-weights" {
		t.Fatalf("the short exact match must win: %+v", hits)
	}
}

// TestSearchMinCoverageAbstains guards the empty-result path. Without it the
// shipped ranker answered every query, measured at 0% abstention over ten
// answer-is-nothing queries — ~417 tokens of irrelevant context each.
func TestSearchMinCoverageAbstains(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--title", "renew the edge certificate", "--description", "WHEN a cert is near expiry")
	pages := mustList(t, dir)
	q := Query{Terms: Terms("kafka partition rebalance certificate"), K: 3}
	if hits := Search(pages, q); len(hits) == 0 {
		t.Fatal("with the gate off the historical behaviour must hold: answer anyway")
	}
	q.MinCoverage = 0.5
	if hits := Search(pages, q); len(hits) != 0 {
		t.Fatalf("1 of 4 terms matched is not an answer: %+v", hits)
	}
	q.Terms = Terms("renew the edge certificate")
	if hits := Search(pages, q); len(hits) != 1 {
		t.Fatalf("a well-covered query must still answer: %+v", hits)
	}
}

// TestRenderResolutionLadder pins the second retrieval axis: which pages come
// back is the ranker's job, how much of each one arrives is the renderer's.
// The ratio is the point — the eval harness once compared rankers whose token
// columns were really measuring this (dhnt/docs/memory-eval-plan.md §4f-bis).
func TestRenderResolutionLadder(t *testing.T) {
	p := &Page{
		Slug: "renew-the-edge-certificate", Type: TypeRunbook, Status: StatusValidated,
		Title:       "renew the edge certificate",
		Description: "WHEN the edge certificate is within 30 days of expiry — the renewal steps and the reload order",
		Body:        strings.Repeat("step. ", 200),
	}
	cue := CueRenderer().Page(p)
	line := LineRenderer().Page(p)
	full := Renderer{Resolution: ResFull, Sep: "  "}.Page(p)

	if !strings.Contains(cue, p.Slug) || !strings.Contains(cue, p.Title) {
		t.Fatalf("cue must carry the address: %q", cue)
	}
	if strings.Contains(cue, p.Description) {
		t.Fatalf("cue must NOT carry the routing prose: %q", cue)
	}
	if !strings.Contains(line, p.Description) || !strings.Contains(line, "[validated/runbook]") {
		t.Fatalf("line must carry the routing surface: %q", line)
	}
	if len(cue) >= len(line) || len(line) >= len(full) {
		t.Fatalf("resolutions must be strictly increasing: cue=%d line=%d full=%d", len(cue), len(line), len(full))
	}
	if !strings.Contains(full, "…") {
		t.Fatalf("a clipped body must be marked so a reader can tell: %q", full)
	}
	if n := len([]rune(full)) - len([]rune(line)); n > DefaultBodyCap+8 {
		t.Fatalf("body cap not applied: full-line = %d runes", n)
	}
}

// TestUseHistoryAndBaseLevel pins the growth-axis substrate. Before this,
// nothing in the store recorded that a page had been USED, so recency and
// frequency were unrankable and ACC/BWT/FWT were unmeasurable.
func TestUseHistoryAndBaseLevel(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--title", "opened often", "--description", "WHEN the thing happens")
	mustRun(t, dir, "add", "--force", "--title", "never opened", "--description", "WHEN the thing happens")
	store := Open(dir)

	if got := store.UseHistory()["never-opened"]; got != nil && got.N > 0 {
		t.Fatalf("an unopened page must have no use history: %+v", got)
	}
	for i := 0; i < 3; i++ {
		store.RecordOpen("opened-often")
	}
	hist := store.UseHistory()
	u := hist["opened-often"]
	if u == nil || u.N != 3 {
		t.Fatalf("three opens must be recorded, got %+v", u)
	}
	if u.Last.IsZero() {
		t.Fatal("last-used must be set")
	}
	now := time.Now()
	if b := u.BaseLevel(now, DefaultDecay); b <= 0 {
		t.Fatalf("recent repeated use must raise base level, got %v", b)
	}
	// Decay: the same three uses, a year on, must be worth less.
	old := &Use{N: 3, Times: []time.Time{
		now.AddDate(-1, 0, 0), now.AddDate(-1, 0, -1), now.AddDate(-1, 0, -2),
	}}
	if old.BaseLevel(now, DefaultDecay) >= u.BaseLevel(now, DefaultDecay) {
		t.Fatal("year-old uses must decay below today's")
	}
	// Never used is neutral (0), not disqualifying: absence of evidence is not
	// evidence of absence.
	if (&Use{}).BaseLevel(now, DefaultDecay) != 0 {
		t.Fatal("an unused page must be neutral, not penalised")
	}
}

// TestUseWeightIsOptInAndBreaksTies guards that ranking is unchanged unless a
// caller asks for the term, and that when asked for, it does what it says.
func TestUseWeightIsOptInAndBreaksTies(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--title", "alpha widget guide", "--description", "WHEN configuring a widget")
	// --force: reconcile-on-write correctly refuses this near-duplicate, and a
	// near-duplicate is exactly what a tie-break test needs.
	mustRun(t, dir, "add", "--force", "--title", "beta widget guide", "--description", "WHEN configuring a widget")
	store := Open(dir)
	pages := mustList(t, dir)
	q := Query{Terms: []string{"widget"}, K: 2}

	base := Search(pages, q)
	if len(base) != 2 || base[0].Page.Slug != "alpha-widget-guide" {
		t.Fatalf("without use history the tie breaks on slug: %+v", base)
	}
	store.RecordOpen("beta-widget-guide")
	q.Use, q.UseWeight = store.UseHistory(), 1.0
	boosted := Search(pages, q)
	if len(boosted) != 2 || boosted[0].Page.Slug != "beta-widget-guide" {
		t.Fatalf("the opened page must win once use is weighted: %+v", boosted)
	}
	q.UseWeight = 0
	if again := Search(pages, q); again[0].Page.Slug != "alpha-widget-guide" {
		t.Fatalf("weight 0 must restore the lexical order: %+v", again)
	}
}

// TestRetireUnopenedAfter guards the grace period, which is the whole design:
// retiring on zero opens alone would retire every page the moment it is
// written, because a new page has never been read.
func TestRetireUnopenedAfter(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--title", "old and unread", "--description", "WHEN nobody has ever needed this")
	mustRun(t, dir, "add", "--force", "--title", "old and used", "--description", "WHEN nobody has ever needed this")
	store := Open(dir)
	pages := mustList(t, dir)
	for _, p := range pages {
		p.Created = "2020-01-01T00:00:00Z" // both are old
	}
	store.RecordOpen("old-and-used")

	// Observation began before these pages existed, so both are inside the
	// window where "was it opened?" is answerable. (The derived case — a store
	// with no read log at all — is TestRetirementNeedsAnObservationWindow.)
	q := Query{Terms: []string{"needed"}, K: 5, Use: store.UseHistory(),
		UseObservedFrom: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)}
	if hits := Search(pages, q); len(hits) != 2 {
		t.Fatalf("with retirement off both pages answer: %+v", hits)
	}
	q.RetireUnopenedAfter = time.Hour
	hits := Search(pages, q)
	if len(hits) != 1 || hits[0].Page.Slug != "old-and-used" {
		t.Fatalf("the never-opened page must be retired, the used one kept: %+v", hits)
	}
	// Grace: a page inside its window is never retired, opened or not.
	for _, p := range pages {
		p.Created = time.Now().UTC().Format(time.RFC3339)
	}
	_ = pages
	if hits := Search(pages, q); len(hits) != 2 {
		t.Fatalf("a page inside its grace period must survive: %+v", hits)
	}
	// No parseable creation time => never retired. Missing evidence is not
	// evidence.
	for _, p := range pages {
		p.Created = ""
	}
	if hits := Search(pages, q); len(hits) != 2 {
		t.Fatalf("an undated page must not be retired: %+v", hits)
	}
}

// TestRetirementNeedsAnObservationWindow is the guard for a measured near-miss:
// pointing the retirement rule at a store that never recorded opens retired
// ALL of it (real store, hit@3 100% -> 0%). A page older than the read log has
// no opens because nothing was recorded, which is absence of evidence.
func TestRetirementNeedsAnObservationWindow(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "--title", "written before anyone watched", "--description", "WHEN nobody has ever needed this")
	store := Open(dir)
	pages := mustList(t, dir)
	for _, p := range pages {
		p.Created = "2020-01-01T00:00:00Z"
	}
	q := Query{Terms: []string{"needed"}, K: 5, RetireUnopenedAfter: time.Hour,
		Use: store.UseHistory(), UseObservedFrom: store.ObservedFrom()}
	if hits := Search(pages, q); len(hits) != 1 {
		t.Fatalf("with no read log, retirement must be inert: %+v", hits)
	}
	// Once recording has started, only pages created inside the window are
	// judged — this one still predates it.
	store.RecordOpen("some-other-page")
	q.Use, q.UseObservedFrom = store.UseHistory(), store.ObservedFrom()
	if hits := Search(pages, q); len(hits) != 1 {
		t.Fatalf("a page older than the read log must not be retired: %+v", hits)
	}
}
