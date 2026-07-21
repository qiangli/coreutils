package board

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/resources"
)

type panel struct {
	id    string
	build func(*Board) PanelView
}

func (p panel) ID() string               { return p.id }
func (p panel) Build(b *Board) PanelView { return p.build(b) }

func DefaultPanels() *Registry {
	return NewRegistry(agentPanel(), todoPanel(), sprintPanel(), runPanel(), salvagePanel(), fleetPanel(), resourcePanel(), utilizationPanel())
}

func agentPanel() Panel {
	return panel{id: "agents", build: func(b *Board) PanelView {
		bands := make([]int, 0, len(b.Summary.AgentLoadByBand))
		for level := range b.Summary.AgentLoadByBand {
			bands = append(bands, level)
		}
		sort.Ints(bands)
		load := make([]string, 0, len(bands))
		for _, level := range bands {
			load = append(load, fmt.Sprintf("L%d %s %d", level, strings.Repeat("█", b.Summary.AgentLoadByBand[level]), b.Summary.AgentLoadByBand[level]))
		}
		v := PanelView{ID: "agents", Title: "Agents",
			Collapsed: fmt.Sprintf("%d in flight; %s", b.Summary.InFlight, join(load, ", ")),
			Columns:   []string{"AGENT", "TOOL", "BAND", "MODEL", "RELIABILITY", "AVAILABILITY", "STATE"}}
		for _, a := range b.Agents {
			v.Rows = append(v.Rows, []string{a.Name, a.Tool, band(a.Band), dash(a.Model), dash(a.Reliability), dash(a.Availability), a.State})
		}
		return v
	}}
}

func todoPanel() Panel {
	return panel{id: "todo", build: func(b *Board) PanelView {
		counts := map[string]int{}
		for _, x := range b.Todos {
			counts[x.Status]++
		}
		v := PanelView{ID: "todo", Title: "Todo",
			Collapsed: fmt.Sprintf("%d total; %d open; %d blocked", len(b.Todos), len(b.Todos)-counts["done"], counts["blocked"]),
			Columns:   []string{"#", "STATUS", "PRIO", "AGE", "SCOPE", "TITLE"}}
		for _, x := range b.Todos {
			v.Rows = append(v.Rows, []string{fmt.Sprint(x.Number), x.Status, dash(x.Priority), ageSince(b.GeneratedAt, x.Created), x.Scope, x.Title})
		}
		return v
	}}
}

func sprintPanel() Panel {
	return panel{id: "sprints", build: func(b *Board) PanelView {
		counts := map[string]int{}
		for _, x := range b.Sprints {
			counts[x.Column]++
		}
		v := PanelView{ID: "sprints", Title: "Sprints",
			Collapsed: fmt.Sprintf("%d total; %d doing; %d awaiting converge", len(b.Sprints), counts["doing"], counts["review"]),
			Columns:   []string{"ID", "COLUMN", "GATE", "EPIC", "CONDUCTOR", "RUNS", "TITLE"}}
		for _, x := range b.Sprints {
			v.Rows = append(v.Rows, []string{"#" + itoa(x.ID), x.Column, dash(x.GateState), dash(x.Epic), dash(x.Conductor), fmt.Sprint(len(x.RunRefs)), x.Title})
		}
		return v
	}}
}

func runPanel() Panel {
	return panel{id: "runs", build: func(b *Board) PanelView {
		counts := map[string]int{}
		for _, x := range b.Runs {
			counts[x.State]++
		}
		v := PanelView{ID: "runs", Title: "Runs",
			Collapsed: fmt.Sprintf("%d total; %d working; %d ready to merge", len(b.Runs), counts["working"], counts["submitted"]),
			Columns:   []string{"ID", "STATE", "AGE", "UNMERGED", "TOOL", "BAND", "MODEL", "REPO", "TITLE"}}
		for _, x := range b.Runs {
			unmerged := "-"
			if x.Salvageable {
				unmerged = fmt.Sprintf("%d commits", x.UnmergedCommits)
			}
			v.Rows = append(v.Rows, []string{"#" + itoa(x.ID), x.State, duration(x.AgeSeconds), unmerged, dash(x.Tool), band(x.Band), dash(x.Model), x.Repo, x.Label})
		}
		return v
	}}
}

func salvagePanel() Panel {
	return panel{id: "salvage", build: func(b *Board) PanelView {
		v := PanelView{ID: "salvage", Title: "Salvageable runs",
			Columns: []string{"ID", "STATE", "UNMERGED", "TOOL", "REPO", "TITLE"}}
		for _, x := range b.Runs {
			if !x.Salvageable {
				continue
			}
			v.Rows = append(v.Rows, []string{"#" + itoa(x.ID), x.State, fmt.Sprintf("%d commits", x.UnmergedCommits), dash(x.Tool), x.Repo, x.Label})
		}
		v.Collapsed = fmt.Sprintf("%d terminal run(s) hold unmerged commits", len(v.Rows))
		return v
	}}
}

func band(n int) string {
	if n <= 0 {
		return "L?"
	}
	return fmt.Sprintf("L%d", n)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func join(xs []string, sep string) string {
	if len(xs) == 0 {
		return "none"
	}
	return strings.Join(xs, sep)
}

func duration(seconds int64) string {
	if seconds <= 0 {
		return "-"
	}
	return (time.Duration(seconds) * time.Second).Round(time.Second).String()
}

func ageSince(now, created time.Time) string {
	if created.IsZero() {
		return "-"
	}
	d := now.Sub(created)
	if d < 0 {
		d = 0
	}
	if d >= 24*time.Hour {
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
	if d >= time.Hour {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	return fmt.Sprintf("%dm", int(d/time.Minute))
}

func fleetPanel() Panel {
	return panel{id: "fleet", build: func(b *Board) PanelView {
		var bAgents []resources.BoardAgent
		for _, a := range b.Agents {
			bAgents = append(bAgents, resources.BoardAgent{
				Name:         a.Name,
				Tool:         a.Tool,
				Model:        a.Model,
				Band:         a.Band,
				Available:    a.Available,
				Found:        a.Found,
				Availability: a.Availability,
				State:        a.State,
			})
		}
		var bRuns []resources.BoardRun
		for _, r := range b.Runs {
			bRuns = append(bRuns, resources.BoardRun{
				State: r.State,
				Tool:  r.Tool,
				Agent: r.Agent,
				Model: r.Model,
			})
		}
		fr, err := resources.CollectFleetResourcesFromBoard(context.Background(), b.GeneratedAt, bAgents, bRuns)
		if err != nil || fr == nil {
			return PanelView{ID: "fleet", Title: "Fleet resources", Collapsed: "unavailable"}
		}
		v := PanelView{
			ID:        "fleet",
			Title:     "Fleet resources",
			Collapsed: fmt.Sprintf("%d total; %d busy; %d idle; %d cooling; %d unavailable", fr.Totals.Total, fr.Totals.Busy, fr.Totals.Idle, fr.Totals.Cooling, fr.Totals.Unavailable),
			Columns:   []string{"PROVIDER", "BAND", "TOTAL", "BUSY", "IDLE", "COOLING", "UNAVAIL", "SUB", "API", "TOKENS", "COST"},
		}
		for _, g := range fr.Groups {
			tokStr, costStr := "N/A", "N/A"
			if g.MeterPresent && g.Tokens != nil && g.CostUSD != nil {
				tokStr = fmt.Sprint(*g.Tokens)
				costStr = fmt.Sprintf("$%.4f", *g.CostUSD)
			}
			v.Rows = append(v.Rows, []string{
				g.Provider, g.Band, fmt.Sprint(g.Total), fmt.Sprint(g.Busy),
				fmt.Sprint(g.Idle), fmt.Sprint(g.Cooling), fmt.Sprint(g.Unavailable),
				fmt.Sprint(g.Subscription), fmt.Sprint(g.APIKey), tokStr, costStr,
			})
		}
		return v
	}}
}
