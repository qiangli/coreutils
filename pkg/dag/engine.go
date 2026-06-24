// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package dag

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

// Engine executes a graph. P1 runs targets in topological SERIAL order; the
// Concurrency / Cache / Attestor seams are reserved for later phases.
type Engine struct {
	Graph *Graph
	Dir   string   // working directory for every target body
	Env   []string // os.Environ() shape; passed to each interpreter

	Concurrency int  // P1: 1 (serial). P1.5 swaps in the parallel scheduler.
	FailFast    bool // stop after the first failure (default true)
	Verbose     bool // print "==> target" banners to Stderr
	Capture     bool // buffer each target's output into its TaskResult (JSON mode)

	Stdout io.Writer
	Stderr io.Writer
}

// RunReport aggregates per-target results.
type RunReport struct {
	Results []TaskResult
	Failed  bool
}

// Run executes the transitive closure of targets (or the whole graph if none
// given) in dependency order. A target whose dependency failed or was skipped
// is itself skipped. Returns a non-nil error only for graph-level failures
// (unknown target, cycle); per-target failures are reported via RunReport.
func (e *Engine) Run(ctx context.Context, targets ...string) (RunReport, error) {
	sub := e.Graph
	if len(targets) > 0 {
		var err error
		if sub, err = e.Graph.Subgraph(targets...); err != nil {
			return RunReport{}, err
		}
	}
	order, err := sub.TopoSort()
	if err != nil {
		return RunReport{}, err
	}

	var report RunReport
	for _, node := range order {
		if blocker, blocked := firstUnmetDep(node); blocked {
			node.Status = StatusSkipped
			res := TaskResult{Name: node.Task.Name, Status: StatusSkipped}
			node.Result = &res
			report.Results = append(report.Results, res)
			report.Failed = true
			if e.Verbose {
				fmt.Fprintf(e.Stderr, "==> skip %s (dependency %s did not succeed)\n",
					node.Task.Name, blocker)
			}
			continue
		}

		ip, err := interpFor(node.Task.Lang)
		if err != nil {
			node.Status = StatusFailed
			res := TaskResult{Name: node.Task.Name, Status: StatusFailed, ExitCode: 2, Err: err}
			node.Result = &res
			report.Results = append(report.Results, res)
			report.Failed = true
			if e.FailFast {
				return report, nil
			}
			continue
		}

		if e.Verbose {
			fmt.Fprintf(e.Stderr, "==> %s\n", node.Task.Name)
		}
		node.Status = StatusRunning
		stdout, stderr := e.Stdout, e.Stderr
		var ob, eb *bytes.Buffer
		if e.Capture {
			ob, eb = new(bytes.Buffer), new(bytes.Buffer)
			stdout, stderr = ob, eb
		}
		res := ip.Run(ctx, node.Task, TaskIO{
			Dir:    e.Dir,
			Env:    e.Env,
			Stdout: stdout,
			Stderr: stderr,
		})
		if e.Capture {
			res.Stdout, res.Stderr = ob.String(), eb.String()
		}
		node.Status = res.Status
		node.Result = &res
		report.Results = append(report.Results, res)
		if res.Status == StatusFailed {
			report.Failed = true
			if e.Verbose {
				fmt.Fprintf(e.Stderr, "==> %s FAILED (exit %d)\n", node.Task.Name, res.ExitCode)
			}
			if e.FailFast {
				return report, nil
			}
		}
	}
	return report, nil
}

func firstUnmetDep(n *Node) (string, bool) {
	for _, d := range n.Deps {
		if d.Status == StatusFailed || d.Status == StatusSkipped {
			return d.Task.Name, true
		}
	}
	return "", false
}
