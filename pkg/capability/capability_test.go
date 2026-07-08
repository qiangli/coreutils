package capability

import (
	"testing"
)

func withTempStore(t *testing.T) {
	t.Helper()
	t.Setenv("BASHY_CAPABILITY_DIR", t.TempDir())
}

func TestSeedAndLoad(t *testing.T) {
	withTempStore(t)
	m, err := Load() // first Load seeds priors
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Agents) == 0 {
		t.Fatal("seed produced no agents")
	}
	if _, ok := m.Agents["opencode:kimi-k2.7-code"]; !ok {
		t.Fatalf("expected seeded agent opencode:kimi-k2.7-code; got %v", keys(m.Agents))
	}
	// Every seeded agent must have every canonical capability.
	for agent, row := range m.Agents {
		for _, c := range AllCaps() {
			if _, ok := row[c]; !ok {
				t.Fatalf("%s missing capability %s", agent, c)
			}
		}
	}
}

func TestFactorization(t *testing.T) {
	withTempStore(t)
	m, _ := Load()
	// Same model, different tool → close QUALITY (deep-research) column.
	ad := m.Agents["aider:deepseek-v4"][CapCoding].Quality
	od := m.Agents["opencode:deepseek-v4"][CapCoding].Quality
	if diff := ad - od; diff > 0.06 || diff < -0.06 {
		t.Errorf("same-model coding quality should be close: aider=%.2f opencode=%.2f", ad, od)
	}
	// Same tool, different model → HARNESS column (operability) identical.
	ok := m.Agents["opencode:kimi-k2.7-code"][CapOperability].Quality
	od2 := m.Agents["opencode:deepseek-v4"][CapOperability].Quality
	if ok != od2 {
		t.Errorf("same-tool operability should match: %.2f vs %.2f", ok, od2)
	}
	// gemini should lead web-search among seeded agents.
	best := m.Best(CapWebSearch, false)
	if len(best) == 0 || ToolOf(best[0].Agent) != "agy" {
		t.Errorf("expected agy to lead web-search, got %v", best)
	}
}

func TestBestRoutableFilter(t *testing.T) {
	withTempStore(t)
	m, _ := Load()
	// With routableOnly, every returned agent's tool must be operable here.
	for _, r := range m.Best(CapCoding, true) {
		if ok, _ := Operable(ToolOf(r.Agent)); !ok {
			t.Errorf("non-routable agent %s returned under routableOnly", r.Agent)
		}
	}
}

func TestRecordMovesQualityAndPersists(t *testing.T) {
	withTempStore(t)
	NowRFC = func() string { return "2026-07-08T00:00:00Z" }
	m, _ := Load()
	before := m.Agents["aider:kimi-k2.7-code"][CapCoding].Quality
	// Two failures should pull quality DOWN and flip source to host.
	if err := Record("aider:kimi-k2.7-code", CapCoding, false, 1200, 50, NowRFC()); err != nil {
		t.Fatal(err)
	}
	if err := Record("aider:kimi-k2.7-code", CapCoding, false, 0, 0, NowRFC()); err != nil {
		t.Fatal(err)
	}
	m2, _ := Load() // reload from disk → persistence check
	cell := m2.Agents["aider:kimi-k2.7-code"][CapCoding]
	if cell.Source != SourceHost {
		t.Errorf("source should be host after Record, got %s", cell.Source)
	}
	if cell.Samples != 2 {
		t.Errorf("samples should be 2, got %d", cell.Samples)
	}
	if cell.Quality >= before {
		t.Errorf("two failures should lower quality: before=%.2f after=%.2f", before, cell.Quality)
	}
	if cell.LatencyMS != 1200 {
		t.Errorf("latency not recorded: %d", cell.LatencyMS)
	}
}

func TestParseCapability(t *testing.T) {
	for in, want := range map[string]Capability{
		"research": CapDeepResearch, "WEB": CapWebSearch, "code-review": CapCodeReview,
		"coding": CapCoding, "judge": CapCodeReview,
	} {
		if got, ok := ParseCapability(in); !ok || got != want {
			t.Errorf("ParseCapability(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	if _, ok := ParseCapability("nonsense"); ok {
		t.Error("expected unknown capability to fail")
	}
}

func keys(m map[string]map[Capability]Cell) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
