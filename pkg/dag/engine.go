// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package dag

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
)

// Engine executes a graph. Concurrency==1 runs targets in topological serial
// order (output streamed live); Concurrency>1 runs the dependency-respecting
// parallel scheduler (output captured per target, flushed in order). A Cache
// (when set, and unless Force) skips targets whose fingerprint is unchanged.
type Engine struct {
	Graph *Graph
	Dir   string   // working directory for every target body
	Env   []string // os.Environ() shape; passed to each interpreter

	Concurrency int    // 1 = serial; >1 = parallel worker pool (make -j N)
	FailFast    bool   // stop scheduling new targets after the first failure
	Verbose     bool   // print "==> target" banners
	Capture     bool   // buffer each target's output into its TaskResult (JSON)
	Force       bool   // ignore the fingerprint cache (make's unconditional run)
	Cache       *Cache // nil = no incremental skip

	Stdout io.Writer
	Stderr io.Writer
}

// RunReport aggregates per-target results.
type RunReport struct {
	Results []TaskResult
	Failed  bool
}

// Run executes the transitive closure of targets (or the whole graph if none
// given) in dependency order. Returns a non-nil error only for graph-level
// failures (unknown target, cycle); per-target failures live in RunReport.
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

	// Precompute fingerprints in topological order so each node's fingerprint
	// can fold in its (already-computed) dependency fingerprints.
	fp := map[string]string{}
	if e.Cache != nil {
		for _, n := range order {
			fp[n.Task.Name] = e.Cache.Fingerprint(n, e.Dir, fp)
		}
	}

	var report RunReport
	if e.Concurrency > 1 {
		report = e.runParallel(ctx, order, fp)
	} else {
		report = e.runSerial(ctx, order, fp)
	}
	if e.Cache != nil {
		e.Cache.Save()
	}
	return report, nil
}

func (e *Engine) runSerial(ctx context.Context, order []*Node, fp map[string]string) RunReport {
	var report RunReport
	for _, node := range order {
		if blocker, blocked := firstUnmetDep(node); blocked {
			report.add(e.markSkipped(node, blocker))
			continue
		}
		if e.upToDate(node, fp) {
			node.Status = StatusUpToDate
			res := TaskResult{Name: node.Task.Name, Status: StatusUpToDate, UpToDate: true}
			node.Result = &res
			report.add(res)
			if e.Verbose {
				fmt.Fprintf(e.Stderr, "==> %s (up to date)\n", node.Task.Name)
			}
			continue
		}
		if e.Verbose {
			fmt.Fprintf(e.Stderr, "==> %s\n", node.Task.Name)
		}
		node.Status = StatusRunning
		res := e.runOne(ctx, node, e.Capture)
		node.Status = res.Status
		node.Result = &res
		report.add(res)
		if res.Status == StatusFailed {
			if e.Verbose {
				fmt.Fprintf(e.Stderr, "==> %s FAILED (exit %d)\n", node.Task.Name, res.ExitCode)
			}
			if e.FailFast {
				return report
			}
		} else if e.Cache != nil {
			e.Cache.Record(node.Task.Name, fp[node.Task.Name])
		}
	}
	return report
}

// runParallel schedules ready nodes (all deps complete) across a worker pool of
// size Concurrency. Output is captured per node and flushed in topological order
// after completion, so concurrency never interleaves a target's output.
func (e *Engine) runParallel(ctx context.Context, order []*Node, fp map[string]string) RunReport {
	inSet := make(map[string]bool, len(order))
	remaining := make(map[string]int, len(order))
	for _, n := range order {
		inSet[n.Task.Name] = true
	}
	for _, n := range order {
		for _, d := range n.Deps {
			if inSet[d.Task.Name] {
				remaining[n.Task.Name]++
			}
		}
	}

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		queued  = make(map[string]bool, len(order))
		results = make(map[string]*TaskResult, len(order))
		failed  bool
		stopped bool
		sem     = make(chan struct{}, e.Concurrency)
		done    = make(chan *Node, len(order))
	)

	pushSkipped := func(n *Node, status Status) {
		queued[n.Task.Name] = true
		r := TaskResult{Name: n.Task.Name, Status: status}
		n.Status, n.Result, results[n.Task.Name] = status, &r, &r
		go func() { done <- n }()
	}

	var schedule func()
	schedule = func() { // caller holds mu
		for _, n := range order {
			if queued[n.Task.Name] || remaining[n.Task.Name] != 0 {
				continue
			}
			if _, blocked := firstUnmetDep(n); blocked {
				failed = true
				pushSkipped(n, StatusSkipped)
				continue
			}
			if stopped {
				continue // fail-fast: leave for the flush
			}
			queued[n.Task.Name] = true
			wg.Add(1)
			go func(node *Node) {
				defer wg.Done()
				sem <- struct{}{}
				var r TaskResult
				if e.upToDate(node, fp) {
					r = TaskResult{Name: node.Task.Name, Status: StatusUpToDate, UpToDate: true}
				} else {
					r = e.runOne(ctx, node, true)
				}
				<-sem
				mu.Lock()
				node.Status, node.Result, results[node.Task.Name] = r.Status, &r, &r
				mu.Unlock()
				done <- node
			}(n)
		}
	}
	flushRemaining := func() { // caller holds mu; fail-fast: skip everything unstarted
		for _, n := range order {
			if !queued[n.Task.Name] {
				pushSkipped(n, StatusSkipped)
			}
		}
	}

	mu.Lock()
	schedule()
	mu.Unlock()

	for completed := 0; completed < len(order); completed++ {
		n := <-done
		mu.Lock()
		switch n.Result.Status {
		case StatusFailed:
			failed = true
			if e.FailFast && !stopped {
				stopped = true
				flushRemaining()
			}
		case StatusSkipped:
			failed = true
		case StatusDone, StatusUpToDate:
			if n.Result.Status == StatusDone && e.Cache != nil {
				e.Cache.Record(n.Task.Name, fp[n.Task.Name])
			}
		}
		for _, dep := range n.Dependents {
			if _, ok := remaining[dep.Task.Name]; ok && remaining[dep.Task.Name] > 0 {
				remaining[dep.Task.Name]--
			}
		}
		schedule()
		mu.Unlock()
	}
	wg.Wait()

	// Deterministic output + report assembly in topological order.
	var report RunReport
	report.Failed = failed
	for _, n := range order {
		r := results[n.Task.Name]
		if r == nil {
			continue
		}
		if e.Verbose {
			e.flushBanner(n, *r)
		}
		report.Results = append(report.Results, *r)
	}
	return report
}

func (e *Engine) flushBanner(n *Node, r TaskResult) {
	switch r.Status {
	case StatusUpToDate:
		fmt.Fprintf(e.Stderr, "==> %s (up to date)\n", n.Task.Name)
		return
	case StatusSkipped:
		fmt.Fprintf(e.Stderr, "==> %s (skipped: dependency did not succeed)\n", n.Task.Name)
		return
	}
	fmt.Fprintf(e.Stderr, "==> %s\n", n.Task.Name)
	if r.Stdout != "" {
		io.WriteString(e.Stdout, r.Stdout)
	}
	if r.Stderr != "" {
		io.WriteString(e.Stderr, r.Stderr)
	}
	if r.Status == StatusFailed {
		fmt.Fprintf(e.Stderr, "==> %s FAILED (exit %d)\n", n.Task.Name, r.ExitCode)
	}
}

// runOne executes a single node's body through its interpreter. When capture is
// set, stdout/stderr are buffered into the result instead of streamed.
func (e *Engine) runOne(ctx context.Context, node *Node, capture bool) TaskResult {
	ip, err := interpFor(node.Task.Lang)
	if err != nil {
		return TaskResult{Name: node.Task.Name, Status: StatusFailed, ExitCode: 2, Err: err}
	}
	stdout, stderr := e.Stdout, e.Stderr
	var ob, eb *bytes.Buffer
	if capture {
		ob, eb = new(bytes.Buffer), new(bytes.Buffer)
		stdout, stderr = ob, eb
	}
	res := ip.Run(ctx, node.Task, TaskIO{
		Dir:    e.Dir,
		Env:    e.envFor(node),
		Stdout: stdout,
		Stderr: stderr,
	})
	if capture {
		res.Stdout, res.Stderr = ob.String(), eb.String()
	}
	return res
}

// envFor builds the environment for a node: the base env plus any per-target
// Env overrides (make's target-specific variables).
func (e *Engine) envFor(node *Node) []string {
	if len(node.Task.Env) == 0 {
		return e.Env
	}
	return append(append([]string{}, e.Env...), node.Task.Env...)
}

func (e *Engine) upToDate(node *Node, fp map[string]string) bool {
	return e.Cache != nil && !e.Force && e.Cache.UpToDate(node, e.Dir, fp[node.Task.Name])
}

func (e *Engine) markSkipped(node *Node, blocker string) TaskResult {
	node.Status = StatusSkipped
	res := TaskResult{Name: node.Task.Name, Status: StatusSkipped}
	node.Result = &res
	if e.Verbose {
		fmt.Fprintf(e.Stderr, "==> skip %s (dependency %s did not succeed)\n", node.Task.Name, blocker)
	}
	return res
}

func (r *RunReport) add(res TaskResult) {
	r.Results = append(r.Results, res)
	if res.Status == StatusFailed || res.Status == StatusSkipped {
		r.Failed = true
	}
}

func firstUnmetDep(n *Node) (string, bool) {
	for _, d := range n.Deps {
		if d.Status == StatusFailed || d.Status == StatusSkipped {
			return d.Task.Name, true
		}
	}
	return "", false
}
