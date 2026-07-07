// Package tsortcmd implements tsort(1) per POSIX and the GNU coreutils
// manual: write a totally ordered list of items consistent with the
// partial ordering given as whitespace-separated pairs in FILE (or
// standard input; "-" means standard input).
//
// Cycle handling matches GNU: each loop found is reported to standard
// error as "tsort: FILE: input contains a loop:" followed by one
// "tsort: ITEM" line per member, an edge of the loop is deleted, and
// the sort presses on — every item still appears in the output, and
// the exit status is 1. An odd number of input tokens is the GNU
// "input contains an odd number of tokens" error. Tie order among
// unconstrained items is first-seen input order (POSIX allows any
// valid total order; GNU's differs).
package tsortcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "tsort",
	Synopsis: "Write totally ordered list consistent with the partial ordering in FILE.",
	Usage:    "tsort [OPTION] [FILE]\n\nWith no FILE, or when FILE is -, read standard input.",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) > 1 {
		return tool.UsageError(rc, cmd, "extra operand '%s'", operands[1])
	}
	name := "-"
	if len(operands) == 1 {
		name = operands[0]
	}

	var r io.Reader = rc.In
	if name != "-" {
		f, err := os.Open(rc.Path(name))
		if err != nil {
			fmt.Fprintf(rc.Err, "tsort: %s: %v\n", name, pathErr(err))
			return 1
		}
		defer f.Close()
		r = f
	}

	g := newGraph()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	sc.Split(bufio.ScanWords)
	for sc.Scan() {
		a := sc.Text()
		if !sc.Scan() {
			if err := sc.Err(); err != nil {
				fmt.Fprintf(rc.Err, "tsort: %s: %v\n", name, err)
				return 1
			}
			g.addNode(a)
			fmt.Fprintf(rc.Err, "tsort: %s: input contains an odd number of tokens\n", name)
			return 1
		}
		b := sc.Text()
		if a == b {
			g.addNode(a)
		} else {
			g.addEdge(a, b)
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(rc.Err, "tsort: %s: %v\n", name, err)
		return 1
	}

	bw := bufio.NewWriter(rc.Out)
	exit := g.topoSort(bw, rc.Err, name)
	if err := bw.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "tsort: write failed: %v\n", err)
		return 1
	}
	return exit
}

// graph is a string digraph in insertion order. Duplicate edges are
// kept (GNU counts them per occurrence); they cancel out because the
// in-degree is decremented once per stored edge.
type graph struct {
	ids   map[string]int
	names []string
	succ  [][]int
	indeg []int
	done  []bool
}

func newGraph() *graph { return &graph{ids: map[string]int{}} }

func (g *graph) addNode(name string) int {
	if id, ok := g.ids[name]; ok {
		return id
	}
	id := len(g.names)
	g.ids[name] = id
	g.names = append(g.names, name)
	g.succ = append(g.succ, nil)
	g.indeg = append(g.indeg, 0)
	g.done = append(g.done, false)
	return id
}

func (g *graph) addEdge(a, b string) {
	u, v := g.addNode(a), g.addNode(b)
	g.succ[u] = append(g.succ[u], v)
	g.indeg[v]++
}

// removeEdge deletes one stored u→v edge and decrements v's in-degree.
func (g *graph) removeEdge(u, v int) {
	for i, s := range g.succ[u] {
		if s == v {
			g.succ[u] = append(g.succ[u][:i], g.succ[u][i+1:]...)
			g.indeg[v]--
			return
		}
	}
}

// topoSort is Kahn's algorithm with a FIFO frontier seeded in input
// order. On a stall (cycle), it reports the loop GNU-style, deletes
// the closing edge, and continues. Returns the exit code.
func (g *graph) topoSort(out *bufio.Writer, errw io.Writer, name string) int {
	exit := 0
	var queue []int
	for id := range g.names {
		if g.indeg[id] == 0 {
			queue = append(queue, id)
		}
	}
	remaining := len(g.names)
	for remaining > 0 {
		if len(queue) == 0 {
			cycle := g.findCycle()
			if len(cycle) == 0 {
				// Cannot happen: a stalled Kahn frontier implies a cycle
				// among the remaining nodes. Bail out rather than loop.
				for id := range g.names {
					if !g.done[id] {
						out.WriteString(g.names[id])
						out.WriteByte('\n')
						g.done[id] = true
						remaining--
					}
				}
				return 1
			}
			exit = 1
			fmt.Fprintf(errw, "tsort: %s: input contains a loop:\n", name)
			for _, id := range cycle {
				fmt.Fprintf(errw, "tsort: %s\n", g.names[id])
			}
			u, v := cycle[len(cycle)-1], cycle[0]
			g.removeEdge(u, v)
			if g.indeg[v] == 0 {
				queue = append(queue, v)
			}
			continue
		}
		id := queue[0]
		queue = queue[1:]
		out.WriteString(g.names[id])
		out.WriteByte('\n')
		g.done[id] = true
		remaining--
		for _, s := range g.succ[id] {
			if g.done[s] {
				continue
			}
			g.indeg[s]--
			if g.indeg[s] == 0 {
				queue = append(queue, s)
			}
		}
	}
	return exit
}

// findCycle runs an iterative DFS over the not-yet-output subgraph and
// returns one cycle in edge direction (c0→c1→…→cn→c0).
func (g *graph) findCycle() []int {
	const (
		white = 0
		grey  = 1
		black = 2
	)
	state := make([]int8, len(g.names))
	type frame struct {
		id  int
		idx int
	}
	for start := range g.names {
		if g.done[start] || state[start] != white {
			continue
		}
		stack := []frame{{start, 0}}
		state[start] = grey
		for len(stack) > 0 {
			fr := &stack[len(stack)-1]
			advanced := false
			for fr.idx < len(g.succ[fr.id]) {
				next := g.succ[fr.id][fr.idx]
				fr.idx++
				if g.done[next] || state[next] == black {
					continue
				}
				if state[next] == grey {
					// Found a back edge: the cycle is the stack suffix
					// starting at next.
					var cycle []int
					for i := range stack {
						if stack[i].id == next {
							for _, f := range stack[i:] {
								cycle = append(cycle, f.id)
							}
							break
						}
					}
					return cycle
				}
				state[next] = grey
				stack = append(stack, frame{next, 0})
				advanced = true
				break
			}
			if !advanced {
				state[fr.id] = black
				stack = stack[:len(stack)-1]
			}
		}
	}
	return nil
}

func pathErr(err error) error {
	return tool.SysErr(err)
}
