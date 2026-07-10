// Package capability is the living capability matrix behind capability-routed
// delegation (see dhnt/docs/capability-routed-delegation.md). Rows are AGENTS
// (tool:model), columns are the predefined bashy capabilities, cells carry a
// quality/latency/cost estimate plus its evidence. The matrix is seeded from
// research priors and refined by observed outcomes on THIS host, so the router
// can pick the best-fit agent for each step of a task.
//
// It is standalone-first (a local JSON store under ~/.bashy/capability); no
// cloudbox dependency.
package capability

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/fleet"
)

// Capability is a canonical capability key. The set mirrors
// docs/agentic-capability-taxonomy.md: harness columns are governed by the TOOL,
// quality columns by the MODEL.
type Capability string

const (
	// Harness columns (tool-governed).
	CapOperability Capability = "operability"
	CapShell       Capability = "shell"
	CapToolUse     Capability = "tool-use"
	CapIsolation   Capability = "isolation"
	// Quality columns (model-governed).
	CapCoding          Capability = "coding"
	CapBugFixing       Capability = "bug-fixing"
	CapCodeReview      Capability = "code-review"
	CapTestGen         Capability = "test-generation"
	CapDeepResearch    Capability = "deep-research"
	CapWebSearch       Capability = "web-search"
	CapBrowserUse      Capability = "browser-use"
	CapDataAnalysis    Capability = "data-analysis"
	CapPlanning        Capability = "planning"
	CapDecisionSupport Capability = "decision-support"
	CapOrchestration   Capability = "orchestration"
)

// HarnessCaps are governed by the tool; QualityCaps by the model.
var HarnessCaps = []Capability{CapOperability, CapShell, CapToolUse, CapIsolation}

var QualityCaps = []Capability{
	CapCoding, CapBugFixing, CapCodeReview, CapTestGen, CapDeepResearch,
	CapWebSearch, CapBrowserUse, CapDataAnalysis, CapPlanning, CapDecisionSupport,
	CapOrchestration,
}

// AllCaps is every canonical capability, in display order.
func AllCaps() []Capability { return append(append([]Capability{}, HarnessCaps...), QualityCaps...) }

// ParseCapability resolves a user string (case-insensitive, aliases) to a
// canonical Capability, or returns false.
func ParseCapability(s string) (Capability, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	alias := map[string]Capability{
		"research": CapDeepResearch, "web": CapWebSearch, "browser": CapBrowserUse,
		"review": CapCodeReview, "judge": CapCodeReview, "tests": CapTestGen,
		"test": CapTestGen, "bugfix": CapBugFixing, "code": CapCoding,
		"plan": CapPlanning, "data": CapDataAnalysis, "ops": CapOrchestration,
	}
	if c, ok := alias[s]; ok {
		return c, true
	}
	for _, c := range AllCaps() {
		if string(c) == s {
			return c, true
		}
	}
	return "", false
}

// Source records where a cell's value came from.
type Source string

const (
	SourcePrior Source = "prior" // research / external benchmark seed
	SourceHost  Source = "host"  // measured from an assignment on this host
)

// Cell is one (agent, capability) estimate.
type Cell struct {
	Quality    float64 `json:"quality"`              // 0..1 fitness
	LatencyMS  int64   `json:"latency_ms,omitempty"` // typical, observed
	CostMicro  int64   `json:"cost_micro,omitempty"` // millionths of a unit, observed
	Source     Source  `json:"source"`               // prior | host
	Samples    int     `json:"samples"`              // host observations folded in
	UpdatedRFC string  `json:"updated,omitempty"`    // RFC3339 of last update
}

// Matrix is the persisted capability matrix: agent (tool:model) -> capability -> cell.
type Matrix struct {
	SchemaVersion string                         `json:"schema_version"`
	Agents        map[string]map[Capability]Cell `json:"agents"`
}

const schemaVersion = "bashy-capability-v1"

// --- storage ---------------------------------------------------------------

// Dir is the capability store directory (override with BASHY_CAPABILITY_DIR).
func Dir() string {
	if d := os.Getenv("BASHY_CAPABILITY_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".bashy", "capability")
}

func matrixPath() string { return filepath.Join(Dir(), "matrix.json") }

// Load returns the stored matrix, seeding priors on first use.
func Load() (*Matrix, error) {
	p := matrixPath()
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			m := seedPriors()
			_ = m.save() // best-effort persist of the seed
			return m, nil
		}
		return nil, err
	}
	m := &Matrix{}
	if err := json.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	if m.Agents == nil {
		m.Agents = map[string]map[Capability]Cell{}
	}
	return m, nil
}

func (m *Matrix) save() error {
	if Dir() == "" {
		return fmt.Errorf("capability: no home dir")
	}
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	tmp, err := os.CreateTemp(Dir(), ".matrix-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, matrixPath())
}

// --- queries ---------------------------------------------------------------

// Ranked is an agent's score for a capability, used by Best.
type Ranked struct {
	Agent       string
	Cell        Cell
	Operable    bool
	Reason      string  // operability note
	Reliability float64 // the agent's operability quality — a gate-pass-rate proxy
	Value       float64 // expected value = quality × reliability ÷ cost (the routing objective, coarsely)
}

// SortKey selects how Best ranks agents.
type SortKey string

const (
	ByQuality SortKey = "quality" // raw capability fit (default)
	ByValue   SortKey = "value"   // quality × reliability ÷ cost — the routing objective
	ByCost    SortKey = "cost"    // cheapest first
)

// Best ranks agents for a capability. It always computes each agent's Reliability
// (its operability quality — a gate-pass-rate proxy, so a flaky agent is penalised
// per the meeting's reliability/rework term) and Value (quality × reliability ÷
// cost — the dishwasher rule made computable). If routableOnly, non-operable
// agents are dropped. Sort order is by `key` (default ByQuality).
func (m *Matrix) Best(c Capability, routableOnly bool, key SortKey) []Ranked {
	var out []Ranked
	for agent, caps := range m.Agents {
		cell, ok := caps[c]
		if !ok {
			continue
		}
		ok2, reason := Operable(ToolOf(agent))
		if routableOnly && !ok2 {
			continue
		}
		rel := 1.0
		if op, ok := caps[CapOperability]; ok {
			rel = op.Quality
		}
		costNorm := float64(cell.CostMicro) / 1000.0
		if costNorm < 0.5 {
			costNorm = 0.5 // floor so free/unpriced agents don't dominate infinitely
		}
		out = append(out, Ranked{
			Agent: agent, Cell: cell, Operable: ok2, Reason: reason,
			Reliability: rel, Value: cell.Quality * rel / costNorm,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch key {
		case ByValue:
			if a.Value != b.Value {
				return a.Value > b.Value
			}
		case ByCost:
			if a.Cell.CostMicro != b.Cell.CostMicro {
				return a.Cell.CostMicro < b.Cell.CostMicro
			}
		default: // ByQuality
			if a.Cell.Quality != b.Cell.Quality {
				return a.Cell.Quality > b.Cell.Quality
			}
			if a.Cell.CostMicro != b.Cell.CostMicro {
				return a.Cell.CostMicro < b.Cell.CostMicro
			}
		}
		return a.Agent < b.Agent
	})
	return out
}

// AgentsForTool returns the matrix agent ids (tool:model) whose tool matches.
func (m *Matrix) AgentsForTool(tool string) []string {
	var out []string
	for agent := range m.Agents {
		if ToolOf(agent) == tool {
			out = append(out, agent)
		}
	}
	sort.Strings(out)
	return out
}

// RecordOperability folds an observed operability outcome for a TOOL into every
// matrix row of that tool (operability is tool-governed, model-independent). Used
// by meet to self-update the matrix from each meeting.
func RecordOperability(tool string, pass bool) error {
	m, err := Load()
	if err != nil {
		return err
	}
	for _, agent := range m.AgentsForTool(tool) {
		_ = Record(agent, CapOperability, pass, 0, 0, NowRFC())
	}
	return nil
}

// Record folds one observed outcome into an agent's capability cell (rolling
// quality via an exponential moving average toward pass=1/fail=0) and persists.
func Record(agent string, c Capability, pass bool, latencyMS, costMicro int64, nowRFC string) error {
	m, err := Load()
	if err != nil {
		return err
	}
	if m.Agents[agent] == nil {
		m.Agents[agent] = map[Capability]Cell{}
	}
	cell := m.Agents[agent][c]
	obs := 0.0
	if pass {
		obs = 1.0
	}
	// EMA: priors start at their seeded quality; host samples pull toward obs.
	const alpha = 0.35
	if cell.Source == SourceHost && cell.Samples > 0 {
		cell.Quality = cell.Quality + alpha*(obs-cell.Quality)
	} else {
		// first host sample blends the prior with the observation
		cell.Quality = 0.5*cell.Quality + 0.5*obs
	}
	if latencyMS > 0 {
		cell.LatencyMS = latencyMS
	}
	if costMicro > 0 {
		cell.CostMicro = costMicro
	}
	cell.Source = SourceHost
	cell.Samples++
	cell.UpdatedRFC = nowRFC
	m.Agents[agent][c] = cell
	return m.save()
}

// --- agent id helpers ------------------------------------------------------

// ToolOf returns the tool half of a tool:model agent id.
func ToolOf(agent string) string {
	tool, _, _ := strings.Cut(agent, ":")
	return tool
}

// ModelOf returns the model half of a tool:model agent id ("" if none).
func ModelOf(agent string) string {
	_, model, _ := strings.Cut(agent, ":")
	return model
}

// Operable reports whether a TOOL can be driven headless on this host, with a
// human note. This is the operability gate the router and meet share: a tool
// that is not installed is not routable; codex is routable but its shell is the
// /etc/passwd login shell (see docs/agent-adoption/matrix.md §F5).
func Operable(tool string) (bool, string) {
	if _, err := exec.LookPath(tool); err != nil {
		return false, "not installed"
	}
	if tool == "codex" && runtime.GOOS == "darwin" {
		return true, "drivable; shell = /bin/zsh login shell (run `bashy install-agent codex` to route through bashy)"
	}
	return true, "drivable; shell routed through bashy by the launcher"
}

// --- priors ----------------------------------------------------------------

// seedPriors builds the initial matrix from the fleet registry, encoding the
// taxonomy factorization capability ≈ harness(tool) ⊕ quality(model).
//
// The factor tables used to live here as Go literals, duplicated against the
// launch contracts in pkg/chat and pkg/weave. They now live in the registry
// (coreutils/pkg/fleet): a tool declares its harness scores, a model its
// quality/cost/specializations, and an agent declares the tool:model pair.
// One declaration, three consumers — so a model added with `bashy models add`
// is routable without editing Go.
//
// Values stay deliberately coarse and marked Source=prior; host outcomes
// refine them.
func seedPriors() *Matrix {
	return seedPriorsFrom(newCatalog())
}

// newCatalog is indirected so tests can pin the registry to a scratch store.
var newCatalog = func() *fleet.Catalog { return fleet.New() }

// Not every quality capability tracks the coding tier. Web-search and
// browser-use need live web integration (a coding-strong model is NOT
// automatically good at them), so they start from a low generic base and the
// per-model specializations decide the leader. deep-research / data-analysis
// sit just below the coding tier. Everything else tracks the tier directly.
var (
	lowBase   = map[Capability]float64{CapWebSearch: 0.50, CapBrowserUse: 0.50}
	tierMinus = map[Capability]float64{CapDeepResearch: 0.05, CapDataAnalysis: 0.05}
)

// defaultHarness is the score for a tool that declares none, and defaultTier
// for a model that declares no quality.
const (
	defaultHarness = 0.6
	defaultTier    = 0.7
)

func seedPriorsFrom(cat *fleet.Catalog) *Matrix {
	tools, _ := cat.Tools(true)
	harness := make(map[string]map[Capability]float64, len(tools))
	for _, t := range tools {
		if len(t.Harness) == 0 {
			continue
		}
		row := make(map[Capability]float64, len(t.Harness))
		for k, v := range t.Harness {
			row[Capability(k)] = v
		}
		harness[t.Name] = row
	}

	models, _ := cat.Models()
	byModel := make(map[string]fleet.Model, len(models))
	for _, m := range models {
		byModel[m.Name] = m
	}

	m := &Matrix{SchemaVersion: schemaVersion, Agents: map[string]map[Capability]Cell{}}
	agents, _ := cat.Agents()
	for _, a := range agents {
		// One matrix row per binding. Several nicknames may name the same
		// tool:model, and they must collapse to one row rather than
		// fragmenting the evidence the router accumulates.
		key := a.MatrixKey()
		if _, seen := m.Agents[key]; seen {
			continue
		}
		model := byModel[a.Model]
		cost := model.CostMicro
		mk := func(q float64) Cell { return Cell{Quality: clampQuality(q), CostMicro: cost, Source: SourcePrior} }

		row := map[Capability]Cell{}
		for _, hc := range HarnessCaps {
			q := defaultHarness
			if v, ok := harness[a.Tool][hc]; ok {
				q = v
			}
			row[hc] = mk(q)
		}
		tier := defaultTier
		if model.Quality > 0 {
			tier = model.Quality
		}
		for _, qc := range QualityCaps {
			var q float64
			switch {
			case lowBase[qc] > 0:
				q = lowBase[qc] // web/browser: integration-bound, not tier-bound
			case tierMinus[qc] > 0:
				q = tier - tierMinus[qc]
			default:
				q = tier
			}
			q += model.Spec[string(qc)]
			row[qc] = mk(q)
		}
		m.Agents[key] = row
	}
	return m
}

func clampQuality(x float64) float64 {
	if x > 0.99 {
		return 0.99
	}
	if x < 0.01 {
		return 0.01
	}
	return x
}

// NowRFC is the current time as RFC3339 (indirected for tests).
var NowRFC = func() string { return time.Now().UTC().Format(time.RFC3339) }
