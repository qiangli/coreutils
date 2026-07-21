package board

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type panel struct {
	id    string
	build func(*Board) PanelView
}

func (p panel) ID() string               { return p.id }
func (p panel) Build(b *Board) PanelView { return p.build(b) }

func DefaultPanels() *Registry {
	return NewRegistry(agentPanel(), todoPanel(), sprintPanel(), runPanel())
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
			Columns:   []string{"ID", "STATE", "TOOL", "BAND", "MODEL", "REPO", "TITLE"}}
		for _, x := range b.Runs {
			v.Rows = append(v.Rows, []string{"#" + itoa(x.ID), x.State, dash(x.Tool), band(x.Band), dash(x.Model), x.Repo, x.Label})
		}
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
