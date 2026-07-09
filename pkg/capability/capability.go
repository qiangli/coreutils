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
	CapCoding         Capability = "coding"
	CapBugFixing      Capability = "bug-fixing"
	CapCodeReview     Capability = "code-review"
	CapTestGen        Capability = "test-generation"
	CapDeepResearch   Capability = "deep-research"
	CapWebSearch      Capability = "web-search"
	CapBrowserUse     Capability = "browser-use"
	CapDataAnalysis   Capability = "data-analysis"
	CapPlanning       Capability = "planning"
	CapDecisionSupport Capability = "decision-support"
	CapOrchestration  Capability = "orchestration"
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
	Quality    float64 `json:"quality"`               // 0..1 fitness
	LatencyMS  int64   `json:"latency_ms,omitempty"`  // typical, observed
	CostMicro  int64   `json:"cost_micro,omitempty"`  // millionths of a unit, observed
	Source     Source  `json:"source"`                // prior | host
	Samples    int     `json:"samples"`               // host observations folded in
	UpdatedRFC string  `json:"updated,omitempty"`     // RFC3339 of last update
}

// Matrix is the persisted capability matrix: agent (tool:model) -> capability -> cell.
type Matrix struct {
	SchemaVersion string                            `json:"schema_version"`
	Agents        map[string]map[Capability]Cell    `json:"agents"`
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

// seedPriors builds the initial matrix from two coarse factor tables — tool
// harness scores and model quality tiers — encoding the taxonomy factorization
// capability ≈ harness(tool) ⊕ quality(model). Values are deliberately coarse
// and marked Source=prior; host outcomes refine them. Seeds the tool:model pairs
// in local use; add rows as agents appear.
func seedPriors() *Matrix {
	// tool -> harness capability -> score
	toolHarness := map[string]map[Capability]float64{
		"claude":   {CapOperability: 0.95, CapShell: 0.95, CapToolUse: 0.90, CapIsolation: 0.80},
		"codex":    {CapOperability: 0.70, CapShell: 0.65, CapToolUse: 0.85, CapIsolation: 0.80}, // login-shell caveat on macOS
		"opencode": {CapOperability: 0.90, CapShell: 0.90, CapToolUse: 0.80, CapIsolation: 0.75},
		"aider":    {CapOperability: 0.80, CapShell: 0.85, CapToolUse: 0.70, CapIsolation: 0.70},
		"agy":      {CapOperability: 0.85, CapShell: 0.85, CapToolUse: 0.80, CapIsolation: 0.75},
	}
	// model -> overall quality tier (applied to quality caps; a few specializations below)
	modelTier := map[string]float64{
		"opus": 0.92, "fable": 0.90, "sonnet": 0.85, "gpt-5.5": 0.90,
		"kimi-k2.7-code": 0.82, "kimi-k2.6": 0.80, "deepseek-v4": 0.80,
		"deepseek-chat": 0.75, "gemini3.1": 0.80, "gemini": 0.78,
	}
	// per (model, capability) bumps where a model is notably stronger/weaker.
	// The gpt-5.5 (codex) profile encodes the 2026-07-08 validation-meeting
	// correction: codex leads gated repo-local implementation, trails on broad/
	// ambiguous strategy (see docs/capability-routed-delegation.md §Refinements).
	spec := map[string]map[Capability]float64{
		"gemini3.1": {CapDeepResearch: +0.10, CapWebSearch: +0.30, CapBrowserUse: +0.25},
		"gemini":    {CapDeepResearch: +0.08, CapWebSearch: +0.28, CapBrowserUse: +0.22},
		"gpt-5.5":   {CapCoding: +0.06, CapBugFixing: +0.06, CapTestGen: +0.04, CapPlanning: -0.05, CapDecisionSupport: -0.05, CapOrchestration: -0.05},
		"opus":      {CapCodeReview: +0.03, CapPlanning: +0.03, CapOrchestration: +0.04},
		"fable":     {CapCodeReview: +0.03, CapDeepResearch: +0.05, CapOrchestration: +0.04},
	}
	// Per-model per-turn cost tier (relative micro-units) — the routing objective's
	// cost term / the dishwasher rule. Locally-hostable commodity models (deepseek,
	// kimi) are cheapest; premium hosted models are dearest.
	modelCost := map[string]int64{
		"opus": 15000, "gpt-5.5": 12000, "fable": 12000, "sonnet": 6000,
		"gemini3.1": 5000, "gemini": 4000,
		"kimi-k2.7-code": 2000, "kimi-k2.6": 1500, "deepseek-v4": 1500, "deepseek-chat": 800,
	}
	// Not every quality capability tracks the coding tier. Web-search and
	// browser-use need live web integration (a coding-strong model is NOT
	// automatically good at them), so they start from a low generic base and the
	// specialization bumps decide the leader. deep-research / data-analysis sit
	// just below the coding tier. Everything else tracks the tier directly.
	lowBase := map[Capability]float64{CapWebSearch: 0.50, CapBrowserUse: 0.50}
	tierMinus := map[Capability]float64{CapDeepResearch: 0.05, CapDataAnalysis: 0.05}
	// The concrete tool:model rows seeded (extend as agents appear on the host).
	pairs := []string{
		"claude:opus", "claude:fable", "codex:gpt-5.5",
		"opencode:deepseek-v4", "opencode:kimi-k2.7-code",
		"aider:deepseek-v4", "aider:kimi-k2.7-code", "agy:gemini3.1",
	}
	clamp := func(x float64) float64 {
		if x > 0.99 {
			return 0.99
		}
		if x < 0.01 {
			return 0.01
		}
		return x
	}
	m := &Matrix{SchemaVersion: schemaVersion, Agents: map[string]map[Capability]Cell{}}
	for _, agent := range pairs {
		tool, model := ToolOf(agent), ModelOf(agent)
		cost := modelCost[model] // 0 if unknown
		mk := func(q float64) Cell { return Cell{Quality: clamp(q), CostMicro: cost, Source: SourcePrior} }
		row := map[Capability]Cell{}
		for _, hc := range HarnessCaps {
			q := 0.6
			if v, ok := toolHarness[tool][hc]; ok {
				q = v
			}
			row[hc] = mk(q)
		}
		tier := 0.7
		if v, ok := modelTier[model]; ok {
			tier = v
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
			if b, ok := spec[model][qc]; ok {
				q += b
			}
			row[qc] = mk(q)
		}
		m.Agents[agent] = row
	}
	return m
}

// NowRFC is the current time as RFC3339 (indirected for tests).
var NowRFC = func() string { return time.Now().UTC().Format(time.RFC3339) }
