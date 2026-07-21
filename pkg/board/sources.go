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

	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/todo"
	"github.com/qiangli/coreutils/pkg/weave"
)

func DefaultSources() []Source {
	// Runs intentionally load first: the todo source uses their repo roots to
	// discover every checked-in todo scope known to this machine.
	return []Source{weaveSource{}, todoSource{}, sprintSource{}, fleetSource{}}
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
func (weaveSource) Load(_ context.Context, b *Board, o Options) error {
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
				ID         int64     `json:"id"`
				Title      string    `json:"title"`
				State      string    `json:"state"`
				Tool       string    `json:"tool"`
				Owner      string    `json:"owner"`
				Points     int       `json:"points"`
				StartedAt  time.Time `json:"started_at"`
				FinishedAt time.Time `json:"finished_at"`
				Blocked    bool      `json:"blocked"`
				Launch     *struct {
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
			r := Run{ID: x.ID, Label: x.Title, Repo: q.Root, State: x.State, Tool: x.Tool, Agent: x.Owner, Points: x.Points, StartedAt: x.StartedAt, FinishedAt: x.FinishedAt, Blocked: x.Blocked}
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
		var env struct {
			SchemaVersion string `json:"schema_version"`
			Items         []struct {
				ID, Title, Status, Priority string
				Seq                         int        `json:"seq"`
				Due                         *time.Time `json:"due"`
				Overdue                     bool       `json:"overdue"`
				Created                     time.Time  `json:"created"`
			} `json:"items"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode %s: %w", sc.scope, err)
		}
		if env.SchemaVersion == "" {
			return fmt.Errorf("%s returned unversioned todo JSON", sc.scope)
		}
		for _, x := range env.Items {
			b.Todos = append(b.Todos, Todo{ID: x.ID, Number: x.Seq, Title: x.Title, Status: x.Status, Priority: x.Priority, Scope: sc.scope, Due: x.Due, Overdue: x.Overdue, Created: x.Created})
		}
	}
	sort.SliceStable(b.Todos, func(i, j int) bool {
		if b.Todos[i].Scope != b.Todos[j].Scope {
			return b.Todos[i].Scope < b.Todos[j].Scope
		}
		return b.Todos[i].Number < b.Todos[j].Number
	})
	return nil
}

type fleetSource struct{}

func (fleetSource) Name() string { return "fleet" }
func (fleetSource) Load(_ context.Context, b *Board, _ Options) error {
	raw, fleetErr := executeJSON(weave.NewWeaveCmd(), "fleet", "--agents", "--json")
	type availability struct {
		Agent        string `json:"agent"`
		Tool         string `json:"tool"`
		Model        string `json:"model"`
		Reason       string `json:"reason"`
		CoolingUntil string `json:"cooling_until"`
		Available    bool   `json:"available"`
		Found        bool   `json:"found"`
	}
	available := map[string]availability{}
	if fleetErr == nil {
		var env wireEnvelope
		var result struct {
			Tools []availability `json:"tools"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			fleetErr = err
		} else if err := json.Unmarshal(env.Result, &result); err != nil {
			fleetErr = err
		} else {
			for _, row := range result.Tools {
				available[row.Agent] = row
			}
		}
	}
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
