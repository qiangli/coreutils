// Package graphcmd registers the AgentOS code-knowledge-graph verbs as flat,
// first-class commands — graph-build, graph-stats, graph-neighbors, graph-impact,
// graph-path, graph-hotspots, graph-query — over the gfy-backed engine in
// coreutils/pkg/codegraph. No cryptic prefix; grouping is by the shared `graph-`
// stem (see dhnt/docs/bashy-code-graph-agentic-feature.md).
//
// This package is deliberately NOT blank-imported by cmds/all: it pulls in gfy's
// document-parsing dependency graph, which must stay out of the bare coreutils
// multicall binary (and out of the lean `bash` drop-in). It is imported only by
// bashy's internal/agentos, so the verbs are reachable at the `bashy graph-*`
// front door and in-shell via the ExecHandler, while cmd/coreutils and cmd/bash
// stay gfy-free.
//
// The engine builds a fully structural graph (tree-sitter AST extraction →
// Louvain communities → degree analytics) with NO LLM and NO graph database — a
// mid-size repo graphs in well under a second. The optional NL/DQL layers live
// behind later phases; every verb here is deterministic and model-free.
package graphcmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	gfygraph "github.com/qiangli/gfy/pkg/graph"
	"github.com/qiangli/gfy/pkg/search"

	"github.com/qiangli/coreutils/pkg/codegraph"
	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/qiangli/coreutils/tool"
)

// schemaVersion is the stable envelope tag agents key on; graph_sha lets a
// consumer cache results and detect staleness across invocations.
const schemaVersion = "bashy-graph-v1"

// cacheRel is a bashy-OWNED cache path, deliberately separate from ycode's
// .agents/ycode/graph.json so the two never contend for the same file / lock.
const cacheRel = ".agents/bashy/graph.json"

func init() {
	register("graph-build", "build/refresh the code knowledge graph and cache it",
		"graph-build [path] [--rebuild] [--json]", runBuild)
	register("graph-stats", "code-graph size: nodes/edges/communities/languages",
		"graph-stats [path] [--rebuild] [--json]", runStats)
	register("graph-neighbors", "direct neighbors (1-hop coupling) of a symbol",
		"graph-neighbors <symbol> [path] [--relation R] [--json]", runNeighbors)
	register("graph-impact", "blast radius: symbols coupled to a target within N hops",
		"graph-impact <file|symbol> [path] [--depth N] [--json]", runImpact)
	register("graph-path", "shortest path between two symbols in the code graph",
		"graph-path <a> <b> [path] [--max-hops N] [--json]", runPath)
	register("graph-hotspots", "most-connected entities (refactor/blast-radius centers)",
		"graph-hotspots [path] [--top N] [--raw] [--json]", runHotspots)
	register("graph-query", "keyword question → matching subgraph (model-free)",
		"graph-query <question> [path] [--depth N] [--json]", runQuery)
}

func register(name, synopsis, usage string, run func(*tool.RunContext, []string) int) {
	t := &tool.Tool{Name: name, Synopsis: synopsis, Usage: usage}
	t.Run = run
	tool.Register(t)
}

// --- shared envelope ---

type header struct {
	Schema    string `json:"schema"`
	GraphSHA  string `json:"graph_sha"`
	Generated string `json:"generated_at"`
	Root      string `json:"root"`
}

func newHeader(root, sha string) header {
	return header{
		Schema:    schemaVersion,
		GraphSHA:  sha,
		Generated: time.Now().UTC().Format(time.RFC3339),
		Root:      root,
	}
}

func writeJSON(rc *tool.RunContext, v any) {
	enc := json.NewEncoder(rc.Out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// --- load-or-build with mtime staleness ---

// sourceExt is the code-file extension set used only for the cheap staleness
// walk (a stat-only pass; no parsing). It mirrors the languages gfy detects.
var sourceExt = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".rs": true, ".java": true, ".c": true, ".h": true, ".cpp": true, ".cc": true,
	".rb": true, ".swift": true, ".kt": true, ".cs": true, ".scala": true,
	".php": true, ".lua": true, ".zig": true, ".ex": true, ".jl": true,
	".m": true, ".dart": true, ".vue": true, ".svelte": true,
}

// loadOrBuild returns the code graph for root: it loads a fresh cache when one
// exists and no source file is newer, otherwise it rebuilds and best-effort
// caches. forceRebuild skips the cache entirely; quiet suppresses build
// progress (used in --json mode so stderr stays clean).
func loadOrBuild(rc *tool.RunContext, root string, forceRebuild, quiet bool) (*codegraph.GraphContext, error) {
	cp := filepath.Join(root, cacheRel)
	if !forceRebuild && cacheFresh(root, cp) {
		if gc, err := codegraph.Load(cp); err == nil && gc != nil {
			return gc, nil
		}
	}
	var progress codegraph.ProgressFunc
	if !quiet {
		progress = func(msg string) { fmt.Fprintln(rc.Err, msg) }
	}
	gc, err := codegraph.BuildWithProgress(root, progress)
	if err != nil {
		return nil, err
	}
	if err := gc.Save(cp); err != nil && !quiet {
		fmt.Fprintf(rc.Err, "graph: cache write skipped: %v\n", err)
	}
	return gc, nil
}

func cacheFresh(root, cachePath string) bool {
	ci, err := os.Stat(cachePath)
	if err != nil {
		return false
	}
	return !newestSourceMTime(root).After(ci.ModTime())
}

func newestSourceMTime(root string) time.Time {
	var newest time.Time
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".agents", ".weave", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !sourceExt[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}

// graphSHA is a stable content hash over the sorted node and edge sets (gfy's
// Nodes()/Edges() are sorted), so identical graphs hash identically regardless
// of build order — the cache key a consumer keys staleness on.
func graphSHA(gc *codegraph.GraphContext) string {
	h := sha256.New()
	for _, id := range gc.Graph.Nodes() {
		h.Write([]byte(id))
		h.Write([]byte{0})
	}
	h.Write([]byte("|edges|"))
	for _, e := range gc.Graph.Edges() {
		h.Write([]byte(e.Source))
		h.Write([]byte{0})
		h.Write([]byte(e.Target))
		h.Write([]byte{1})
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// --- shared flag/arg helpers ---

func resolveRoot(rc *tool.RunContext, target string) string {
	if target == "" {
		if rc.Dir != "" {
			return rc.Dir
		}
		return "."
	}
	return rc.Path(target)
}

func nodeLabel(g *gfygraph.Graph, id string) string {
	if s, ok := g.NodeAttrs(id)["label"].(string); ok {
		return s
	}
	return id
}

func edgeStr(attrs map[string]any, key string) string {
	if v, ok := attrs[key].(string); ok {
		return v
	}
	return ""
}

func usageErr(rc *tool.RunContext, name, msg string) int {
	fmt.Fprintf(rc.Err, "%s: %s\n", name, msg)
	return 2
}

// --- graph-build ---

type buildPayload struct {
	header
	Nodes       int      `json:"nodes"`
	Edges       int      `json:"edges"`
	Communities int      `json:"communities"`
	Files       int      `json:"files"`
	Languages   []string `json:"languages"`
	Cache       string   `json:"cache"`
}

func runBuild(rc *tool.RunContext, args []string) int {
	asJSON := weavecli.IsAgent()
	var target string
	for _, a := range args {
		switch a {
		case "--json", "--json=true":
			asJSON = true
		case "--json=false", "--plain":
			asJSON = false
		case "--rebuild": // graph-build always rebuilds; flag accepted for symmetry
		default:
			if strings.HasPrefix(a, "-") {
				return usageErr(rc, "graph-build", "unknown option "+a)
			}
			if target == "" {
				target = a
			}
		}
	}
	root := resolveRoot(rc, target)
	// graph-build forces a fresh build (its whole purpose) and caches it.
	gc, err := loadOrBuild(rc, root, true, asJSON)
	if err != nil {
		fmt.Fprintf(rc.Err, "graph-build: %v\n", err)
		return 1
	}
	cp := filepath.Join(root, cacheRel)
	if asJSON {
		writeJSON(rc, buildPayload{
			header:      newHeader(root, graphSHA(gc)),
			Nodes:       gc.Stats.NodeCount,
			Edges:       gc.Stats.EdgeCount,
			Communities: gc.Stats.CommunityCount,
			Files:       gc.Stats.FilesAnalyzed,
			Languages:   gc.Stats.Languages,
			Cache:       cp,
		})
		return 0
	}
	fmt.Fprintln(rc.Out, gc.GetGraphStats())
	fmt.Fprintf(rc.Out, "cached: %s (graph_sha %s)\n", cp, graphSHA(gc))
	return 0
}

// --- graph-stats ---

type statsPayload struct {
	header
	Nodes       int      `json:"nodes"`
	Edges       int      `json:"edges"`
	Communities int      `json:"communities"`
	Files       int      `json:"files"`
	Languages   []string `json:"languages"`
	Extracted   int      `json:"extracted"`
	Inferred    int      `json:"inferred"`
	Ambiguous   int      `json:"ambiguous"`
}

func runStats(rc *tool.RunContext, args []string) int {
	asJSON := weavecli.IsAgent()
	rebuild := false
	var target string
	for _, a := range args {
		switch a {
		case "--json", "--json=true":
			asJSON = true
		case "--json=false", "--plain":
			asJSON = false
		case "--rebuild":
			rebuild = true
		default:
			if strings.HasPrefix(a, "-") {
				return usageErr(rc, "graph-stats", "unknown option "+a)
			}
			if target == "" {
				target = a
			}
		}
	}
	root := resolveRoot(rc, target)
	gc, err := loadOrBuild(rc, root, rebuild, asJSON)
	if err != nil {
		fmt.Fprintf(rc.Err, "graph-stats: %v\n", err)
		return 1
	}
	if asJSON {
		ex, inf, amb := confidenceCounts(gc)
		writeJSON(rc, statsPayload{
			header:      newHeader(root, graphSHA(gc)),
			Nodes:       gc.Stats.NodeCount,
			Edges:       gc.Stats.EdgeCount,
			Communities: gc.Stats.CommunityCount,
			Files:       gc.Stats.FilesAnalyzed,
			Languages:   gc.Stats.Languages,
			Extracted:   ex,
			Inferred:    inf,
			Ambiguous:   amb,
		})
		return 0
	}
	fmt.Fprintln(rc.Out, gc.GetGraphStats())
	return 0
}

func confidenceCounts(gc *codegraph.GraphContext) (extracted, inferred, ambiguous int) {
	for _, e := range gc.Graph.Edges() {
		switch edgeStr(e.Attrs, "confidence") {
		case "EXTRACTED":
			extracted++
		case "INFERRED":
			inferred++
		case "AMBIGUOUS":
			ambiguous++
		}
	}
	return
}

// --- graph-neighbors ---

type neighbor struct {
	Label      string `json:"label"`
	FileType   string `json:"file_type"`
	Relation   string `json:"relation"`
	Confidence string `json:"confidence"`
}

type neighborsPayload struct {
	header
	Node      string     `json:"node"`
	Neighbors []neighbor `json:"neighbors"`
}

func runNeighbors(rc *tool.RunContext, args []string) int {
	asJSON := weavecli.IsAgent()
	var symbol, target, relation string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json" || a == "--json=true":
			asJSON = true
		case a == "--json=false" || a == "--plain":
			asJSON = false
		case a == "--relation":
			if i+1 < len(args) {
				i++
				relation = args[i]
			}
		case strings.HasPrefix(a, "--relation="):
			relation = a[len("--relation="):]
		case strings.HasPrefix(a, "-") && a != "-":
			return usageErr(rc, "graph-neighbors", "unknown option "+a)
		default:
			if symbol == "" {
				symbol = a
			} else if target == "" {
				target = a
			}
		}
	}
	if symbol == "" {
		return usageErr(rc, "graph-neighbors", "missing <symbol>")
	}
	root := resolveRoot(rc, target)
	gc, err := loadOrBuild(rc, root, false, asJSON)
	if err != nil {
		fmt.Fprintf(rc.Err, "graph-neighbors: %v\n", err)
		return 1
	}
	id := search.FindNode(gc.Graph, symbol)
	if id == "" {
		fmt.Fprintf(rc.Err, "graph-neighbors: symbol not found: %s\n", symbol)
		return 1
	}
	var nbrs []neighbor
	for _, nb := range gc.Graph.Neighbors(id) {
		eAttrs := gc.Graph.EdgeAttrs(id, nb)
		rel := edgeStr(eAttrs, "relation")
		if relation != "" && rel != relation {
			continue
		}
		nAttrs := gc.Graph.NodeAttrs(nb)
		nbrs = append(nbrs, neighbor{
			Label:      edgeStr(nAttrs, "label"),
			FileType:   edgeStr(nAttrs, "file_type"),
			Relation:   rel,
			Confidence: edgeStr(eAttrs, "confidence"),
		})
	}
	if asJSON {
		writeJSON(rc, neighborsPayload{
			header:    newHeader(root, graphSHA(gc)),
			Node:      nodeLabel(gc.Graph, id),
			Neighbors: nbrs,
		})
		return 0
	}
	if len(nbrs) == 0 {
		fmt.Fprintln(rc.Err, "(no neighbors)")
		return 0
	}
	for _, n := range nbrs {
		fmt.Fprintf(rc.Out, "- %s (%s) [%s, %s]\n", n.Label, n.FileType, n.Relation, n.Confidence)
	}
	return 0
}

// --- graph-impact ---

type sgNode struct {
	Label    string `json:"label"`
	FileType string `json:"file_type"`
}

type sgEdge struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"`
}

type impactPayload struct {
	header
	Node  string   `json:"node"`
	Depth int      `json:"depth"`
	Nodes []sgNode `json:"nodes"`
	Edges []sgEdge `json:"edges"`
}

func runImpact(rc *tool.RunContext, args []string) int {
	asJSON := weavecli.IsAgent()
	depth := 2
	var symbol, target string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json" || a == "--json=true":
			asJSON = true
		case a == "--json=false" || a == "--plain":
			asJSON = false
		case a == "--depth":
			if i+1 < len(args) {
				i++
				depth = atoiDefault(args[i], depth)
			}
		case strings.HasPrefix(a, "--depth="):
			depth = atoiDefault(a[len("--depth="):], depth)
		case strings.HasPrefix(a, "-") && a != "-":
			return usageErr(rc, "graph-impact", "unknown option "+a)
		default:
			if symbol == "" {
				symbol = a
			} else if target == "" {
				target = a
			}
		}
	}
	if symbol == "" {
		return usageErr(rc, "graph-impact", "missing <file|symbol>")
	}
	if depth < 1 {
		depth = 1
	}
	root := resolveRoot(rc, target)
	gc, err := loadOrBuild(rc, root, false, asJSON)
	if err != nil {
		fmt.Fprintf(rc.Err, "graph-impact: %v\n", err)
		return 1
	}
	id := search.FindNode(gc.Graph, symbol)
	if id == "" {
		fmt.Fprintf(rc.Err, "graph-impact: not found: %s\n", symbol)
		return 1
	}
	visited, edges := gc.Graph.BFS([]string{id}, depth)
	nodes := make([]sgNode, 0, len(visited))
	for _, nid := range visited {
		a := gc.Graph.NodeAttrs(nid)
		nodes = append(nodes, sgNode{Label: edgeStr(a, "label"), FileType: edgeStr(a, "file_type")})
	}
	sgEdges := make([]sgEdge, 0, len(edges))
	for _, e := range edges {
		sgEdges = append(sgEdges, sgEdge{
			Source:   nodeLabel(gc.Graph, e.Source),
			Target:   nodeLabel(gc.Graph, e.Target),
			Relation: edgeStr(e.Attrs, "relation"),
		})
	}
	if asJSON {
		writeJSON(rc, impactPayload{
			header: newHeader(root, graphSHA(gc)),
			Node:   nodeLabel(gc.Graph, id),
			Depth:  depth,
			Nodes:  nodes,
			Edges:  sgEdges,
		})
		return 0
	}
	fmt.Fprintf(rc.Out, "blast radius of %q within %d hop(s): %d symbols, %d edges\n",
		nodeLabel(gc.Graph, id), depth, len(nodes), len(sgEdges))
	for _, n := range nodes {
		fmt.Fprintf(rc.Out, "- %s (%s)\n", n.Label, n.FileType)
	}
	return 0
}

// --- graph-path ---

type pathPayload struct {
	header
	Source string   `json:"source"`
	Target string   `json:"target"`
	Hops   int      `json:"hops"`
	Path   []string `json:"path"`
}

func runPath(rc *tool.RunContext, args []string) int {
	asJSON := weavecli.IsAgent()
	maxHops := 6
	var a1, a2, target string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json" || a == "--json=true":
			asJSON = true
		case a == "--json=false" || a == "--plain":
			asJSON = false
		case a == "--max-hops":
			if i+1 < len(args) {
				i++
				maxHops = atoiDefault(args[i], maxHops)
			}
		case strings.HasPrefix(a, "--max-hops="):
			maxHops = atoiDefault(a[len("--max-hops="):], maxHops)
		case strings.HasPrefix(a, "-") && a != "-":
			return usageErr(rc, "graph-path", "unknown option "+a)
		default:
			switch {
			case a1 == "":
				a1 = a
			case a2 == "":
				a2 = a
			case target == "":
				target = a
			}
		}
	}
	if a1 == "" || a2 == "" {
		return usageErr(rc, "graph-path", "usage: graph-path <a> <b> [path]")
	}
	root := resolveRoot(rc, target)
	gc, err := loadOrBuild(rc, root, false, asJSON)
	if err != nil {
		fmt.Fprintf(rc.Err, "graph-path: %v\n", err)
		return 1
	}
	src := search.FindNode(gc.Graph, a1)
	tgt := search.FindNode(gc.Graph, a2)
	if src == "" {
		fmt.Fprintf(rc.Err, "graph-path: not found: %s\n", a1)
		return 1
	}
	if tgt == "" {
		fmt.Fprintf(rc.Err, "graph-path: not found: %s\n", a2)
		return 1
	}
	ids := gc.Graph.ShortestPath(src, tgt, maxHops)
	if ids == nil {
		if asJSON {
			writeJSON(rc, pathPayload{header: newHeader(root, graphSHA(gc)), Source: nodeLabel(gc.Graph, src), Target: nodeLabel(gc.Graph, tgt), Hops: -1, Path: []string{}})
			return 0
		}
		fmt.Fprintln(rc.Out, "no path found")
		return 1
	}
	labels := make([]string, len(ids))
	for i, nid := range ids {
		labels[i] = nodeLabel(gc.Graph, nid)
	}
	if asJSON {
		writeJSON(rc, pathPayload{
			header: newHeader(root, graphSHA(gc)),
			Source: labels[0],
			Target: labels[len(labels)-1],
			Hops:   len(labels) - 1,
			Path:   labels,
		})
		return 0
	}
	fmt.Fprintf(rc.Out, "path (%d hops): %s\n", len(labels)-1, strings.Join(labels, " → "))
	return 0
}

// --- graph-hotspots ---

type hotspot struct {
	Rank   int    `json:"rank"`
	Label  string `json:"label"`
	Degree int    `json:"degree"`
}

type hotspotsPayload struct {
	header
	Hotspots []hotspot `json:"hotspots"`
}

// ubiquitousLabels are accessor/utility method names that dominate raw degree
// ranking (a bare `.String()`/`.Len()` is on nearly every type) but carry no
// architectural signal. graph-hotspots drops them by default so real linchpins
// surface; --raw disables the filter (identical to gfy's god-nodes). This is a
// documented heuristic, not upstream behavior. Degree centrality is the cheap
// O(V) first-order god-object signal; betweenness/PageRank (a future --metric)
// add bottleneck/influence nuance at O(V·(V+E)).
var ubiquitousLabels = map[string]bool{
	"String": true, "Error": true, "Len": true, "Cap": true, "Bytes": true,
	"Append": true, "Reset": true, "Close": true, "Read": true, "Write": true,
	"Init": true, "Get": true, "Set": true, "contains": true, "Clone": true,
	"Equal": true, "Marshal": true, "Unmarshal": true, "MarshalJSON": true,
	"UnmarshalJSON": true, "Format": true, "Value": true, "Size": true,
	"Add": true, "Lock": true, "Unlock": true, "New": true,
}

func normalizeLabel(label string) string {
	s := strings.TrimPrefix(label, ".")
	s = strings.TrimSuffix(s, "()")
	return s
}

func runHotspots(rc *tool.RunContext, args []string) int {
	asJSON := weavecli.IsAgent()
	top, raw := 15, false
	var target string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json" || a == "--json=true":
			asJSON = true
		case a == "--json=false" || a == "--plain":
			asJSON = false
		case a == "--raw":
			raw = true
		case a == "--top":
			if i+1 < len(args) {
				i++
				top = atoiDefault(args[i], top)
			}
		case strings.HasPrefix(a, "--top="):
			top = atoiDefault(a[len("--top="):], top)
		case strings.HasPrefix(a, "-") && a != "-":
			return usageErr(rc, "graph-hotspots", "unknown option "+a)
		default:
			if target == "" {
				target = a
			}
		}
	}
	if top <= 0 {
		top = 15
	}
	root := resolveRoot(rc, target)
	gc, err := loadOrBuild(rc, root, false, asJSON)
	if err != nil {
		fmt.Fprintf(rc.Err, "graph-hotspots: %v\n", err)
		return 1
	}
	var spots []hotspot
	for _, gn := range gc.GodNodes {
		if !raw && ubiquitousLabels[normalizeLabel(gn.Label)] {
			continue
		}
		spots = append(spots, hotspot{Rank: len(spots) + 1, Label: gn.Label, Degree: gn.Degree})
		if len(spots) >= top {
			break
		}
	}
	if asJSON {
		writeJSON(rc, hotspotsPayload{header: newHeader(root, graphSHA(gc)), Hotspots: spots})
		return 0
	}
	if len(spots) == 0 {
		fmt.Fprintln(rc.Err, "(no hotspots)")
		return 0
	}
	for _, s := range spots {
		fmt.Fprintf(rc.Out, "%2d. %s (degree %d)\n", s.Rank, s.Label, s.Degree)
	}
	return 0
}

// --- graph-query ---

type queryPayload struct {
	header
	Question string   `json:"question"`
	Nodes    []sgNode `json:"nodes"`
	Edges    []sgEdge `json:"edges"`
}

func runQuery(rc *tool.RunContext, args []string) int {
	asJSON := weavecli.IsAgent()
	depth := 2
	var question, target string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json" || a == "--json=true":
			asJSON = true
		case a == "--json=false" || a == "--plain":
			asJSON = false
		case a == "--depth":
			if i+1 < len(args) {
				i++
				depth = atoiDefault(args[i], depth)
			}
		case strings.HasPrefix(a, "--depth="):
			depth = atoiDefault(a[len("--depth="):], depth)
		case strings.HasPrefix(a, "-") && a != "-":
			return usageErr(rc, "graph-query", "unknown option "+a)
		default:
			if question == "" {
				question = a
			} else if target == "" {
				target = a
			}
		}
	}
	if question == "" {
		return usageErr(rc, "graph-query", "missing <question>")
	}
	if depth < 1 {
		depth = 2
	}
	root := resolveRoot(rc, target)
	gc, err := loadOrBuild(rc, root, false, asJSON)
	if err != nil {
		fmt.Fprintf(rc.Err, "graph-query: %v\n", err)
		return 1
	}
	results := search.ScoreNodes(gc.Graph, question)
	if len(results) > 5 {
		results = results[:5]
	}
	if len(results) == 0 {
		if asJSON {
			writeJSON(rc, queryPayload{header: newHeader(root, graphSHA(gc)), Question: question, Nodes: []sgNode{}, Edges: []sgEdge{}})
			return 1
		}
		fmt.Fprintf(rc.Out, "no matching nodes for: %s\n", question)
		return 1
	}
	startNodes := make([]string, len(results))
	for i, r := range results {
		startNodes[i] = r.ID
	}
	visited, edges := gc.Graph.BFS(startNodes, depth)
	nodes := make([]sgNode, 0, len(visited))
	for _, nid := range visited {
		a := gc.Graph.NodeAttrs(nid)
		nodes = append(nodes, sgNode{Label: edgeStr(a, "label"), FileType: edgeStr(a, "file_type")})
	}
	sgEdges := make([]sgEdge, 0, len(edges))
	for _, e := range edges {
		sgEdges = append(sgEdges, sgEdge{
			Source:   nodeLabel(gc.Graph, e.Source),
			Target:   nodeLabel(gc.Graph, e.Target),
			Relation: edgeStr(e.Attrs, "relation"),
		})
	}
	if asJSON {
		writeJSON(rc, queryPayload{header: newHeader(root, graphSHA(gc)), Question: question, Nodes: nodes, Edges: sgEdges})
		return 0
	}
	fmt.Fprintf(rc.Out, "subgraph for %q: %d nodes, %d edges\n", question, len(nodes), len(sgEdges))
	for _, n := range nodes {
		fmt.Fprintf(rc.Out, "- %s (%s)\n", n.Label, n.FileType)
	}
	return 0
}

func atoiDefault(s string, def int) int {
	n := 0
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return def
	}
	return n
}
