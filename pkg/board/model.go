// Package board implements bashy's read-only steward/conductor projection.
// Sources collect the three work layers; renderers and panels are registries so
// the P2 conductor view and future statistics do not change the core model.
package board

import (
	"context"
	"sort"
	"time"

	"github.com/qiangli/coreutils/pkg/resources"
)

const SchemaVersion = "bashy-board-v1"

type Options struct {
	All    bool
	Expand map[string]bool
	Now    time.Time
}

type Board struct {
	SchemaVersion string      `json:"schema_version"`
	Role          string      `json:"role"`
	Scope         string      `json:"scope"`
	Title         string      `json:"title"`
	GeneratedAt   time.Time   `json:"generated_at"`
	Rows          []Row       `json:"rows"`
	Rollup        Rollup      `json:"rollup"`
	Summary       Summary     `json:"summary"`
	Lanes         []Lane      `json:"lanes"`
	Panels        []PanelView `json:"panels"`
	Agents        []Agent     `json:"agents"`
	Todos         []Todo      `json:"todos"`
	Sprints       []Sprint    `json:"sprints"`
	Runs          []Run       `json:"runs"`
	// Resources is the host reading (nil when the collector is not wired
	// into the source set, e.g. a test board).
	Resources *resources.System `json:"resources,omitempty"`
	// Utilization is the fleet-invariant verdict: idle capacity is only
	// acceptable when the board reads 0 open work.
	Utilization *resources.Utilization `json:"utilization,omitempty"`
	Warnings    []string               `json:"warnings,omitempty"`
}

// Row is the normalized record shared by every source. The richer typed
// slices remain available to panels; Rows is the stable cross-source wire view.
type Row struct {
	Source      string `json:"source"`
	Repo        string `json:"repo,omitempty"`
	ID          string `json:"id"`
	State       string `json:"state"`
	Tool        string `json:"tool,omitempty"`
	Band        int    `json:"band,omitempty"`
	Model       string `json:"model,omitempty"`
	Label       string `json:"label"`
	Points      int    `json:"points,omitempty"`
	ElapsedSecs int64  `json:"elapsed_secs,omitempty"`
	DurSecs     int64  `json:"dur_secs,omitempty"`
	SprintID    int64  `json:"sprint_id,omitempty"`
	Salvageable bool   `json:"salvageable,omitempty"`
	Unmerged    int    `json:"unmerged_commits,omitempty"`
	AgeSeconds  int64  `json:"age_seconds,omitempty"`
	Stale       bool   `json:"stale,omitempty"`
}

type Rollup struct {
	ByState       map[string]int `json:"by_state"`
	ByAgentBand   map[int]int    `json:"by_agent_band"`
	Merged        int            `json:"merged"`
	EtaMedianSecs int64          `json:"eta_median_secs,omitempty"`
}

type Summary struct {
	Todos            int         `json:"todos"`
	Sprints          int         `json:"sprints"`
	Runs             int         `json:"runs"`
	NeedsSteward     int         `json:"needs_steward"`
	Unattended       int         `json:"unattended"`
	InFlight         int         `json:"in_flight"`
	ETAMedianSeconds int64       `json:"eta_median_seconds,omitempty"`
	AgentLoadByBand  map[int]int `json:"agent_load_by_band"`
}

type Lane struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Cards   []Card `json:"cards"`
	Dropped int    `json:"dropped,omitempty"`
}

type Card struct {
	Layer       string `json:"layer"`
	ID          string `json:"id"`
	Label       string `json:"label"`
	State       string `json:"state"`
	Tool        string `json:"tool,omitempty"`
	Band        int    `json:"band,omitempty"`
	Model       string `json:"model,omitempty"`
	Elapsed     int64  `json:"elapsed_seconds,omitempty"`
	ETA         int64  `json:"eta_seconds,omitempty"`
	Scope       string `json:"scope,omitempty"`
	Salvageable bool   `json:"salvageable,omitempty"`
	Unmerged    int    `json:"unmerged_commits,omitempty"`
	AgeSeconds  int64  `json:"age_seconds,omitempty"`
	Stale       bool   `json:"stale,omitempty"`
}

type Agent struct {
	Name         string `json:"name"`
	Tool         string `json:"tool"`
	Model        string `json:"model,omitempty"`
	Band         int    `json:"band,omitempty"`
	Reliability  string `json:"reliability,omitempty"`
	Available    bool   `json:"available"`
	Found        bool   `json:"found"`
	Availability string `json:"availability"`
	State        string `json:"state"`
}

type Todo struct {
	ID       string     `json:"id"`
	Number   int        `json:"number,omitempty"`
	Title    string     `json:"title"`
	Status   string     `json:"status"`
	Priority string     `json:"priority,omitempty"`
	Scope    string     `json:"scope"`
	Due      *time.Time `json:"due,omitempty"`
	Overdue  bool       `json:"overdue,omitempty"`
	Created  time.Time  `json:"created,omitempty,omitzero"`
}

type Sprint struct {
	ID            int64    `json:"id"`
	Title         string   `json:"title"`
	Epic          string   `json:"epic,omitempty"`
	Column        string   `json:"column"`
	Continuity    string   `json:"continuity,omitempty"`
	Conductor     string   `json:"conductor,omitempty"`
	LeaseStale    bool     `json:"lease_stale,omitempty"`
	GateState     string   `json:"gate_state,omitempty"`
	LeaseHolder   string   `json:"lease_holder,omitempty"`
	ContinuityRef string   `json:"continuity_ref,omitempty"`
	RunRefs       []RunRef `json:"run_refs,omitempty"`
}

type RunRef struct {
	Repo string `json:"repo"`
	ID   int64  `json:"id"`
}

type Run struct {
	ID              int64     `json:"id"`
	Label           string    `json:"label"`
	Repo            string    `json:"repo"`
	State           string    `json:"state"`
	Tool            string    `json:"tool,omitempty"`
	Agent           string    `json:"agent,omitempty"`
	Model           string    `json:"model,omitempty"`
	Band            int       `json:"band,omitempty"`
	StartedAt       time.Time `json:"started_at,omitempty,omitzero"`
	MaxRuntime      int64     `json:"max_runtime_seconds,omitempty"`
	FinishedAt      time.Time `json:"finished_at,omitempty,omitzero"`
	Points          int       `json:"points,omitempty"`
	SprintID        int64     `json:"sprint_id,omitempty"`
	Blocked         bool      `json:"blocked,omitempty"`
	Salvageable     bool      `json:"salvageable,omitempty"`
	UnmergedCommits int       `json:"unmerged_commits,omitempty"`
	AgeSeconds      int64     `json:"age_seconds,omitempty"`
	Stale           bool      `json:"stale,omitempty"`
}

type Source interface {
	Name() string
	Load(context.Context, *Board, Options) error
}

type SourceFunc struct {
	SourceName string
	Func       func(context.Context, *Board, Options) error
}

func (s SourceFunc) Name() string                                        { return s.SourceName }
func (s SourceFunc) Load(ctx context.Context, b *Board, o Options) error { return s.Func(ctx, b, o) }

type Panel interface {
	ID() string
	Build(*Board) PanelView
}

type PanelView struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Collapsed string     `json:"collapsed"`
	Columns   []string   `json:"columns,omitempty"`
	Rows      [][]string `json:"rows,omitempty"`
}

type Registry struct{ panels []Panel }

func NewRegistry(panels ...Panel) *Registry {
	return &Registry{panels: append([]Panel(nil), panels...)}
}
func (r *Registry) Register(p Panel) { r.panels = append(r.panels, p) }
func (r *Registry) Build(b *Board) []PanelView {
	out := make([]PanelView, 0, len(r.panels))
	for _, p := range r.panels {
		out = append(out, p.Build(b))
	}
	return out
}

type Renderer interface {
	Render(*Board, Options) ([]byte, error)
}

func Collect(ctx context.Context, opts Options, sources []Source, panels *Registry) (*Board, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	b := &Board{SchemaVersion: SchemaVersion, Role: "steward", Scope: "machine-global", Title: "Bashy Steward Board", GeneratedAt: opts.Now.UTC()}
	for _, s := range sources {
		if err := s.Load(ctx, b, opts); err != nil {
			b.Warnings = append(b.Warnings, s.Name()+": "+err.Error())
		}
	}
	b.finalize(opts.Now)
	if panels == nil {
		panels = DefaultPanels()
	}
	b.Panels = panels.Build(b)
	return b, nil
}

func (b *Board) finalize(now time.Time) {
	linked := map[string]int64{}
	for _, sprint := range b.Sprints {
		for _, ref := range sprint.RunRefs {
			linked[ref.Repo+"\x00"+itoa(ref.ID)] = sprint.ID
		}
	}
	loads := map[int]int{}
	completed := map[string][]int64{}
	var allCompleted []int64
	for _, r := range b.Runs {
		if !r.StartedAt.IsZero() && !r.FinishedAt.IsZero() && r.FinishedAt.After(r.StartedAt) {
			d := int64(r.FinishedAt.Sub(r.StartedAt).Seconds())
			completed[r.Tool] = append(completed[r.Tool], d)
			allCompleted = append(allCompleted, d)
		}
	}
	lanes := map[string][]Card{"backlog": {}, "working": {}, "review": {}, "needs-steward": {}, "done": {}}
	for i := range b.Runs {
		r := &b.Runs[i]
		if r.SprintID == 0 {
			r.SprintID = linked[r.Repo+"\x00"+itoa(r.ID)]
		}
		elapsed, eta := int64(0), int64(0)
		if !r.StartedAt.IsZero() {
			elapsed = int64(now.Sub(r.StartedAt).Seconds())
			if elapsed < 0 {
				elapsed = 0
			}
			if estimate, ok := durationMedian(completed[r.Tool]); ok && estimate > elapsed {
				eta = estimate - elapsed
			}
		}
		c := Card{Layer: "run", ID: itoa(r.ID), Label: r.Label, State: r.State, Tool: r.Tool, Band: r.Band, Model: r.Model, Elapsed: elapsed, ETA: eta, Scope: r.Repo, Salvageable: r.Salvageable, Unmerged: r.UnmergedCommits, AgeSeconds: r.AgeSeconds, Stale: r.Stale}
		dur := int64(0)
		if !r.StartedAt.IsZero() && !r.FinishedAt.IsZero() && r.FinishedAt.After(r.StartedAt) {
			dur = int64(r.FinishedAt.Sub(r.StartedAt).Seconds())
		}
		b.Rows = append(b.Rows, Row{Source: "run", Repo: r.Repo, ID: itoa(r.ID), State: r.State, Tool: r.Tool, Band: r.Band, Model: r.Model, Label: r.Label, Points: r.Points, ElapsedSecs: elapsed, DurSecs: dur, SprintID: r.SprintID, Salvageable: r.Salvageable, Unmerged: r.UnmergedCommits, AgeSeconds: r.AgeSeconds, Stale: r.Stale})
		lane := runLane(r.State)
		if r.State == "submitted" || r.Salvageable || r.Stale {
			lane = "needs-steward"
		}
		lanes[lane] = append(lanes[lane], c)
		if r.Stale {
			b.Summary.Unattended++
		}
		if r.State == "working" {
			loads[r.Band]++
			b.Summary.InFlight++
		}
	}
	for _, t := range b.Todos {
		b.Rows = append(b.Rows, Row{Source: "todo", Repo: t.Scope, ID: t.ID, State: t.Status, Label: t.Title})
		if t.Status == "blocked" {
			lanes["needs-steward"] = append(lanes["needs-steward"], Card{Layer: "todo", ID: t.ID, Label: t.Title, State: t.Status, Scope: t.Scope})
		}
	}
	for _, s := range b.Sprints {
		b.Rows = append(b.Rows, Row{Source: "sprint", ID: itoa(s.ID), State: s.Column, Label: s.Title, SprintID: s.ID})
		if s.Column == "review" {
			lanes["needs-steward"] = append(lanes["needs-steward"], Card{Layer: "sprint", ID: itoa(s.ID), Label: s.Title, State: s.Column})
		}
	}
	if estimate, ok := durationMedian(allCompleted); ok {
		b.Summary.ETAMedianSeconds = estimate
	}
	b.Summary.Todos = len(b.Todos)
	b.Summary.Sprints = len(b.Sprints)
	b.Summary.Runs = len(b.Runs)
	b.Summary.NeedsSteward = len(lanes["needs-steward"])
	b.Summary.AgentLoadByBand = loads
	b.Rollup = Rollup{ByState: map[string]int{}, ByAgentBand: loads, EtaMedianSecs: b.Summary.ETAMedianSeconds}
	for _, row := range b.Rows {
		b.Rollup.ByState[row.State]++
		if row.Source == "run" && row.State == "done" {
			b.Rollup.Merged++
		}
	}
	b.evaluateUtilization()
	for _, id := range []string{"needs-steward", "working", "review", "backlog", "done"} {
		if len(lanes[id]) > 0 || id != "done" {
			b.Lanes = append(b.Lanes, Lane{ID: id, Title: laneTitle(id), Cards: lanes[id]})
		}
	}
}

func durationMedian(samples []int64) (int64, bool) {
	// Fewer than three observations is anecdote, not an ETA.
	if len(samples) < 3 {
		return 0, false
	}
	values := append([]int64(nil), samples...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	mid := len(values) / 2
	if len(values)%2 == 0 {
		return (values[mid-1] + values[mid]) / 2, true
	}
	return values[mid], true
}

func runLane(s string) string {
	switch s {
	case "working", "allocated", "paused":
		return "working"
	case "submitted":
		return "review"
	case "done", "abandoned", "killed", "failed":
		return "done"
	default:
		return "backlog"
	}
}
func laneTitle(s string) string {
	switch s {
	case "needs-steward":
		return "Needs steward"
	case "working":
		return "In flight"
	case "review":
		return "Review"
	case "done":
		return "History"
	default:
		return "Backlog"
	}
}
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var a [24]byte
	i := len(a)
	for n > 0 {
		i--
		a[i] = byte('0' + n%10)
		n /= 10
	}
	return string(a[i:])
}
