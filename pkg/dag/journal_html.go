// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package dag

import (
	"bytes"
	_ "embed"
	"html/template"
	"strings"
	"time"
)

// runHTMLSource is the run view's template. It is embedded rather than inlined
// so it stays editable as HTML, and it is entirely self-contained: no CDN, no
// external stylesheet, no script. A viewer that needs the network is a viewer
// that does not work on the machine that ran the job.
//
//go:embed ui/run.html.tmpl
var runHTMLSource string

var runHTML = template.Must(template.New("run").Funcs(template.FuncMap{
	"dur": fmtMS,
	"ts":  func(t time.Time) string { return t.Format("2006-01-02 15:04:05") },
	"cls": statusClass,
}).Parse(runHTMLSource))

// statusClass maps a status onto a CSS class token. Statuses are a closed set
// produced by Status.String(), but this is defensive on purpose: the value
// reaches the page from report.json, which is a file on disk, and a class
// attribute is not a place to interpolate whatever a file happens to contain.
func statusClass(s string) string {
	for _, known := range []string{
		"pending", "running", "done", "failed",
		"skipped", "up-to-date", "condition-skipped",
	} {
		if s == known {
			return s
		}
	}
	return "unknown"
}

// htmlCell is one slot in the layered graph table. Task is nil where a layer
// has fewer targets than the widest one.
type htmlCell struct{ Task *RunTask }

// htmlRunView is the run page's template data.
type htmlRunView struct {
	Title  string
	Run    *RunEntry
	Tasks  []RunTask
	Levels []int
	Grid   [][]htmlCell
}

// renderRunHTML renders one journaled run as a standalone page: a layered graph
// table (one column per topological layer) over the target list.
//
// The layered table is deliberately NOT a drawn graph. Columns come straight
// from the layer each node already carries in graph.json, so there is no layout
// algorithm, no edge routing, and nothing to get wrong — and for the mostly
// linear pipelines dag runs it reads better than a spaghetti diagram would.
func renderRunHTML(entry *RunEntry, graph *RunGraph) ([]byte, error) {
	view := htmlRunView{
		Title: "dag run — " + runTitle(entry),
		Run:   entry,
		Tasks: entry.Tasks,
	}
	if graph != nil {
		view.Levels, view.Grid = layerGrid(entry.Tasks, graph)
	}
	var buf bytes.Buffer
	if err := runHTML.Execute(&buf, view); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// runTitle is the human label for a run: the DAG file's base name plus its
// targets, falling back to the run id.
func runTitle(entry *RunEntry) string {
	base := entry.File
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	if len(entry.Targets) > 0 {
		return base + " " + strings.Join(entry.Targets, " ")
	}
	if base != "" {
		return base
	}
	return entry.RunID
}

// layerGrid arranges the run's targets into columns by topological layer.
// Targets present in the graph but absent from the report (never reached) still
// occupy their slot, so the shape of what did NOT run stays visible.
func layerGrid(tasks []RunTask, graph *RunGraph) ([]int, [][]htmlCell) {
	byName := make(map[string]*RunTask, len(tasks))
	for i := range tasks {
		byName[tasks[i].Name] = &tasks[i]
	}
	cols := map[int][]htmlCell{}
	maxLayer, maxRows := -1, 0
	for _, n := range graph.Nodes {
		cols[n.Layer] = append(cols[n.Layer], htmlCell{Task: byName[n.Name]})
		if n.Layer > maxLayer {
			maxLayer = n.Layer
		}
		if len(cols[n.Layer]) > maxRows {
			maxRows = len(cols[n.Layer])
		}
	}
	if maxLayer < 0 {
		return nil, nil
	}
	levels := make([]int, 0, maxLayer+1)
	for l := 0; l <= maxLayer; l++ {
		levels = append(levels, l)
	}
	grid := make([][]htmlCell, maxRows)
	for r := range grid {
		row := make([]htmlCell, len(levels))
		for c := range levels {
			if r < len(cols[c]) {
				row[c] = cols[c][r]
			}
		}
		grid[r] = row
	}
	return levels, grid
}
