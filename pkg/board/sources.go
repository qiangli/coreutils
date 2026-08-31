package board

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/dag"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/todo"
	"github.com/qiangli/coreutils/pkg/weave"
)

func DefaultSources() []Source {
	// Runs intentionally load first: the todo source uses their repo roots to
	// discover every checked-in todo scope known to this machine.
	return []Source{weaveSource{}, todoSource{}, sprintSource{}, fleetSource{}, resourceSource{}, dagSource{}}
}

// NewDagSource exposes the dag run-journal source for callers assembling a
// custom source set.
func NewDagSource() Source { return dagSource{} }

// dagRunsPerFile bounds how many runs each dag document contributes to the
// board. The board is a glance across the machine, not a history browser —
// `dag --runs` is the history browser.
const dagRunsPerFile = 5

type dagSource struct{}

func (dagSource) Name() string { return "dag" }

func (dagSource) Load(_ context.Context, b *Board, _ Options) error {
	entries, err := dag.ListAllRuns("", dagRunsPerFile)
	if err != nil {
		return err
	}
	for _, e := range entries {
		r := DagRun{
			RunID: e.RunID, File: e.File, Targets: strings.Join(e.Targets, " "),
			StartedAt: e.StartedAt, DurationMS: e.DurationMS,
			Failed: e.Failed, Total: len(e.Tasks),
		}
		for _, t := range e.Tasks {
			switch t.Status {
			case "done", "up-to-date":
				r.OK++
			case "failed":
				r.FailedN++
			}
		}
		b.DagRuns = append(b.DagRuns, r)
	}
	return nil
}

// NewTodoSource, NewSprintSource, NewWeaveSource, and NewFleetSource expose
// the standard collectors individually so another role can compose a scoped
// board without knowing their implementations.
func NewTodoSource() Source   { return todoSource{} }
func NewSprintSource() Source { return sprintSource{} }
func NewWeaveSource() Source  { return weaveSource{} }
func NewFleetSource() Source  { return fleetSource{} }

func executeJSON(cmd *cobra.Command, args ...string) ([]byte, error) {
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, bytes.TrimSpace(out.Bytes()))
	}
	return out.Bytes(), nil
}

type wireEnvelope struct {
	Status string          `json:"status"`
	Result json.RawMessage `json:"result"`
}

type weaveSource struct{}

func (weaveSource) Name() string { return "weave" }
func (weaveSource) Load(ctx context.Context, b *Board, o Options) error {
	args := []string{"list", "--all", "--json"}
	if o.All {
		args = append(args, "--history")
	}
	raw, err := executeJSON(weave.NewWeaveCmd(), args...)
	if err != nil {
		return err
	}
	var env wireEnvelope
	if err = json.Unmarshal(raw, &env); err != nil {
		return err
	}
	var result struct {
		Queues []struct {
			Root  string `json:"root"`
			Items []struct {
				ID              int64     `json:"id"`
				Title           string    `json:"title"`
				State           string    `json:"state"`
				Tool            string    `json:"tool"`
				Owner           string    `json:"owner"`
				Points          int       `json:"points"`
				StartedAt       time.Time `json:"started_at"`
				FinishedAt      time.Time `json:"finished_at"`
				Blocked         bool      `json:"blocked"`
				Salvageable     bool      `json:"salvageable"`
				UnmergedCommits int       `json:"unmerged_commits"`
				AgeSeconds      int64     `json:"age_seconds"`
				Stale           bool      `json:"stale"`
				Launch          *struct {
					Agent      string        `json:"agent"`
					Model      string        `json:"model"`
					MaxRuntime time.Duration `json:"max_runtime"`
				} `json:"launch_spec"`
			} `json:"items"`
		} `json:"queues"`
	}
	if err = json.Unmarshal(env.Result, &result); err != nil {
		return err
	}
	for _, q := range result.Queues {
		for _, x := range q.Items {
			// Owner is the conductor principal and may be stale; it is not the
			// launched agent. Agent identity comes only from launch_spec.
			r := Run{ID: x.ID, Label: x.Title, Repo: q.Root, State: x.State, Tool: x.Tool, Points: x.Points, StartedAt: x.StartedAt, FinishedAt: x.FinishedAt, Blocked: x.Blocked, Salvageable: x.Salvageable, UnmergedCommits: x.UnmergedCommits, AgeSeconds: x.AgeSeconds, Stale: x.Stale}
			if x.Launch != nil {
				if x.Launch.Agent != "" {
					r.Agent = x.Launch.Agent
				}
				r.Model = x.Launch.Model
				r.MaxRuntime = int64(x.Launch.MaxRuntime / time.Second)
			}
			r.Band, r.Model = fleet.ResolveLaunchModel(r.Tool, r.Model)
			b.Runs = append(b.Runs, r)
		}
	}
	return nil
}

type weaveDoctorFinding struct {
	AgeSeconds int64
	Stale      bool
}

// loadWeaveDoctorFindings deliberately crosses the package boundary through
// the command's JSON contract. Doctor owns lifecycle policy; board only knows
// that an open row has an age and may carry the "stale" flag.
func loadWeaveDoctorFindings(ctx context.Context, root string) (map[int64]weaveDoctorFinding, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	c := exec.CommandContext(ctx, self, "weave", "doctor", "--json")
	c.Dir = root
	raw, err := c.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("weave doctor --json: %w: %s", err, bytes.TrimSpace(raw))
	}
	return decodeWeaveDoctorFindings(raw)
}

func decodeWeaveDoctorFindings(raw []byte) (map[int64]weaveDoctorFinding, error) {
	var env wireEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decode weave doctor envelope: %w", err)
	}
	var result struct {
		Open []struct {
			Issue      int64    `json:"issue"`
			AgeSeconds int64    `json:"age_seconds"`
			Flags      []string `json:"flags"`
		} `json:"open"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return nil, fmt.Errorf("decode weave doctor result: %w", err)
	}
	findings := make(map[int64]weaveDoctorFinding, len(result.Open))
	for _, row := range result.Open {
		finding := weaveDoctorFinding{AgeSeconds: row.AgeSeconds}
		for _, flag := range row.Flags {
			finding.Stale = finding.Stale || flag == "stale"
		}
		findings[row.Issue] = finding
	}
	return findings, nil
}

type sprintSource struct{}

func (sprintSource) Name() string { return "sprint" }
func (sprintSource) Load(_ context.Context, b *Board, o Options) error {
	raw, err := executeJSON(weave.NewSprintCmd(), "board", "--json")
	if err != nil {
		return err
	}
	var env wireEnvelope
	if err = json.Unmarshal(raw, &env); err != nil {
		return err
	}
	var result struct {
		Stories []struct {
			ID         int64    `json:"id"`
			Title      string   `json:"title"`
			Epic       string   `json:"epic"`
			Column     string   `json:"column"`
			Continuity string   `json:"continuity"`
			Acceptance string   `json:"acceptance"`
			Runs       []RunRef `json:"runs"`
			Lease      *struct {
				Holder string
				At     time.Time
			} `json:"lease"`
		} `json:"stories"`
	}
	if err = json.Unmarshal(env.Result, &result); err != nil {
		return err
	}
	for _, x := range result.Stories {
		if !o.All && x.Column == "done" {
			continue
		}
		s := Sprint{ID: x.ID, Title: x.Title, Epic: x.Epic, Column: x.Column, Continuity: x.Continuity, ContinuityRef: x.Continuity, RunRefs: x.Runs}
		if x.Acceptance != "" {
			s.GateState = "pending"
		}
		if x.Column == "review" {
			s.GateState = "awaiting-converge"
		} else if x.Column == "done" {
			s.GateState = "complete"
		}
		if x.Lease != nil {
			s.Conductor, s.LeaseHolder = x.Lease.Holder, x.Lease.Holder
			s.LeaseStale = o.Now.Sub(x.Lease.At) > 30*time.Minute
		}
		b.Sprints = append(b.Sprints, s)
	}
	return nil
}

// todoItem is one row of `todo list --json`, tolerant of both envelope
// generations. `state` is the current field name; `status` was the pre-loom-v2
// spelling and is kept as a fallback so a downgrade does not blank the column.
type todoItem struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	State    string     `json:"state"`
	Status   string     `json:"status"`
	Priority string     `json:"priority"`
	Seq      int        `json:"seq"`
	Due      *time.Time `json:"due"`
	Overdue  bool       `json:"overdue"`
	Created  time.Time  `json:"created"`
	Sprint   int64      `json:"sprint"`
}

func (t todoItem) status() string {
	if t.State != "" {
		return t.State
	}
	return t.Status
}

// todoEnvelope reads `todo list --json` in either shape: loom-v2 wraps the rows
// in `result`, earlier builds put them at the top level. Both are declared so
// the board keeps working across a version skew between the two binaries rather
// than silently reporting an empty queue.
type todoEnvelope struct {
	SchemaVersion string     `json:"schema_version"`
	Items         []todoItem `json:"items"`
	Result        struct {
		Count int        `json:"count"`
		Items []todoItem `json:"items"`
	} `json:"result"`
}

func (e todoEnvelope) items() []todoItem {
	if len(e.Result.Items) > 0 {
		return e.Result.Items
	}
	return e.Items
}

// decodeTodoList parses one `todo list --json` payload into board rows.
//
// It fails LOUDLY on an envelope whose shape no longer matches what it declares,
// because the alternative is what shipped: the rows moved under `result` in
// loom-v2, the top-level decode yielded nothing, Load returned nil, and the
// board reported zero todos on a host with 49 — no error, no warning.
//
// The count cross-check is the guard, not the version string. Comparing
// schema_version against a list of known values would only catch drift someone
// remembered to enumerate; the payload declaring its own length catches ANY
// reshape, including one that keeps the version. The bare non-empty version test
// this replaces could not catch anything at all: "loom-v2" is non-empty, so the
// check that existed to detect exactly this drift passed it through.
func decodeTodoList(raw []byte, scope string) ([]todoItem, error) {
	var env todoEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decode %s: %w", scope, err)
	}
	if env.SchemaVersion == "" {
		return nil, fmt.Errorf("%s returned unversioned todo JSON", scope)
	}
	items := env.items()
	if env.Result.Count > len(items) {
		return nil, fmt.Errorf(
			"%s: decoded %d of %d todo item(s); todo JSON envelope changed shape (schema_version %q)",
			scope, len(items), env.Result.Count, env.SchemaVersion,
		)
	}
	return items, nil
}

type todoSource struct{}

func (todoSource) Name() string { return "todo" }
func (todoSource) Load(_ context.Context, b *Board, o Options) error {
	type scoped struct {
		scope string
		args  []string
	}
	var stores []scoped
	seen := map[string]bool{}
	add := func(scope, p string, args ...string) {
		p = filepath.Clean(p)
		if !seen[p] {
			seen[p] = true
			stores = append(stores, scoped{scope, args})
		}
	}
	if root, err := todo.Root(); err == nil {
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			if e.IsDir() {
				add("user "+e.Name(), filepath.Join(root, e.Name()), "--user", "--owner", e.Name())
			}
		}
		if len(entries) == 0 {
			add("user "+todo.DefaultOwner, filepath.Join(root, todo.DefaultOwner), "--user", "--owner", todo.DefaultOwner)
		}
	}
	if cwd, ok := todo.FindGitRoot(); ok {
		add("repo "+filepath.Base(cwd), filepath.Join(cwd, "docs", "todo"), "--base-dir", cwd)
	}
	for _, r := range b.Runs {
		if r.Repo != "" {
			add("repo "+filepath.Base(r.Repo), filepath.Join(r.Repo, "docs", "todo"), "--base-dir", r.Repo)
		}
	}
	for _, sc := range stores {
		args := append(append([]string(nil), sc.args...), "list", "--json")
		if o.All {
			args = append(args, "--all")
		}
		raw, err := executeJSON(todo.NewTodoCmd(), args...)
		if err != nil {
			return err
		}
		items, err := decodeTodoList(raw, sc.scope)
		if err != nil {
			return err
		}
		for _, x := range items {
			b.Todos = append(b.Todos, Todo{ID: x.ID, Number: x.Seq, Title: x.Title, Status: x.status(), Priority: x.Priority, Scope: sc.scope, Due: x.Due, Overdue: x.Overdue, Created: x.Created, SprintID: x.Sprint, Store: sc.args})
		}
	}
	sort.SliceStable(b.Todos, func(i, j int) bool {
		if b.Todos[i].Scope != b.Todos[j].Scope {
			return b.Todos[i].Scope < b.Todos[j].Scope
		}
		if a, z := todo.PriorityRank(b.Todos[i].Priority), todo.PriorityRank(b.Todos[j].Priority); a != z {
			return a < z
		}
		return b.Todos[i].Number < b.Todos[j].Number
	})
	return nil
}

type fleetAvailability struct {
	Agent        string `json:"agent"`
	Tool         string `json:"tool"`
	Model        string `json:"model"`
	Reason       string `json:"reason"`
	CoolingUntil string `json:"cooling_until"`
	Available    bool   `json:"available"`
	Found        bool   `json:"found"`
}

type fleetSource struct {
	// loadAvailability is a test seam. Production uses
	// loadWeaveFleetAvailability; keeping the seam here lets a test distinguish
	// an expected non-repo snapshot from a real collector failure without
	// replacing PATH or spawning a fake executable.
	loadAvailability func() (map[string]fleetAvailability, error)
}

func (fleetSource) Name() string { return "fleet" }
func (s fleetSource) Load(_ context.Context, b *Board, _ Options) error {
	load := s.loadAvailability
	if load == nil {
		load = loadWeaveFleetAvailability
	}
	available, fleetErr := load()

	cat := fleet.New()
	agents, errs := cat.Agents()
	if len(errs) > 0 {
		return errs[0]
	}
	working := map[string]bool{}
	for _, r := range b.Runs {
		if r.State == "working" && r.Agent != "" {
			working[r.Agent] = true
		}
	}
	for _, a := range agents {
		row := Agent{Name: a.Name, Tool: a.Tool, Model: a.Model, Reliability: "unmeasured", State: "idle"}
		if a.Ledger != nil && a.Ledger.Reliability != "" {
			row.Reliability = a.Ledger.Reliability
		}
		_, tool, model, err := cat.Binding(a.Name)
		if err != nil {
			row.Availability = err.Error()
		} else {
			row.Band = model.Band
			if a.Band > 0 {
				row.Band = a.Band
			}
			binary := tool.CLI.Binary
			if binary == "" {
				binary = tool.Name
			}
			if live, ok := available[a.Name]; ok {
				row.Found, row.Available = live.Found, live.Available
				switch {
				case live.Reason != "":
					row.Availability = live.Reason
				case live.CoolingUntil != "":
					row.Availability = "cooling until " + live.CoolingUntil
					row.State = "cooling"
				case live.Available:
					row.Availability = "available"
				default:
					row.Availability = "unavailable"
				}
			} else {
				_, lookErr := exec.LookPath(binary)
				row.Found, row.Available = lookErr == nil, lookErr == nil
				if row.Found {
					row.Availability = "available (PATH only)"
				} else {
					row.Availability = "not on PATH"
				}
			}
		}
		if working[a.Name] {
			row.State = "working"
		}
		b.Agents = append(b.Agents, row)
	}
	if fleetErr != nil {
		return fmt.Errorf("weave fleet availability unavailable; PATH fallback shown: %s", strings.TrimSpace(fleetErr.Error()))
	}
	return nil
}

// loadWeaveFleetAvailability enriches the machine roster with weave's
// repository-scoped cooldown/probe evidence when the board is running inside a
// clone. A web console is normally launched from a service directory or the
// user's home, where no repository exists. That is an expected scope, not a
// broken source: the board still has honest host-scoped PATH/catalog evidence,
// so it returns that fallback without emitting a warning.
func loadWeaveFleetAvailability() (map[string]fleetAvailability, error) {
	available := map[string]fleetAvailability{}
	if _, ok := todo.FindGitRoot(); !ok {
		return available, nil
	}

	raw, fleetErr := executeJSON(weave.NewWeaveCmd(), "fleet", "--agents", "--json")
	if fleetErr != nil {
		return available, fleetErr
	}
	var env wireEnvelope
	var result struct {
		Tools []fleetAvailability `json:"tools"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return available, err
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return available, err
	}
	for _, row := range result.Tools {
		available[row.Agent] = row
	}
	return available, nil
}

// StoryDetail asks todo for one item's full record, in the register the board
// found it in.
//
// It re-runs `todo show <id> --json` through the same in-process seam the
// sources use rather than reading the item file itself: the on-disk format is
// todo's business, and a reader that parses another tool's storage becomes a
// second implementation of it that nobody remembers to update.
func StoryDetail(t Todo) (*Story, error) {
	if t.ID == "" {
		return nil, fmt.Errorf("story: no id")
	}
	args := append(append([]string(nil), t.Store...), "show", t.ID, "--json")
	raw, err := executeJSON(todo.NewTodoCmd(), args...)
	if err != nil {
		return nil, err
	}
	var st Story
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("story %s: %w", t.ID, err)
	}
	if st.ID == "" {
		return nil, fmt.Errorf("story %s: todo returned no record", t.ID)
	}
	st.Scope = t.Scope
	return &st, nil
}
