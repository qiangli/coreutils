package fanout

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Instance is one seat in the fan-out: an agent (tool:model or nick) running a
// per-instance instruction variant under a lens (scope). Needs names the scopes
// this instance depends on — it runs only after every instance producing those
// scopes has completed, and its prompt is fed their contributions. Empty Needs
// = wave 0 (independent, runs immediately).
type Instance struct {
	Agent       string
	Instruction string
	Scope       string
	Needs       []string
}

// Launcher runs one agent against a prompt and returns its output. It is
// injected so the orchestrator is testable without spawning real CLIs; the CLI
// wires a chat.Invoke-backed implementation.
type Launcher interface {
	Launch(ctx context.Context, agent, prompt string, timeout time.Duration) (output string, exitCode int, err error)
}

// Result is one instance's outcome.
type Result struct {
	Agent    string
	Scope    string
	Output   string
	ExitCode int
	Err      error
	Posted   bool
}

// Config drives a fan-out run.
type Config struct {
	Board     *Board
	Instances []Instance
	Jobs      int // max concurrent instances; <=0 means len(Instances)
	Timeout   time.Duration
}

// Run fans the instances out against the shared board in dependency WAVES. All
// instances in a wave run concurrently (bounded by Jobs); a wave starts only
// after every earlier wave has completed and posted, so a dependent instance
// (Needs set) sees its dependencies' contributions. With no dependencies this
// is a single wave — pure concurrent fan-out. Read-shared / write-to-board:
// instances post to the board, not each other's workspace. Returns one Result
// per instance, in input order.
func Run(ctx context.Context, cfg Config, l Launcher) ([]Result, error) {
	if cfg.Board == nil {
		return nil, fmt.Errorf("fanout: nil board")
	}
	if l == nil {
		return nil, fmt.Errorf("fanout: nil launcher")
	}
	if len(cfg.Instances) == 0 {
		return nil, fmt.Errorf("fanout: no instances")
	}
	waves, err := computeWaves(cfg.Instances)
	if err != nil {
		return nil, err
	}
	jobs := cfg.Jobs
	if jobs <= 0 {
		jobs = len(cfg.Instances)
	}

	results := make([]Result, len(cfg.Instances))
	for _, wave := range waves {
		runWave(ctx, cfg, l, wave, jobs, results)
	}
	return results, nil
}

// runWave launches one wave's instances concurrently, bounded by jobs, and
// posts each output back to the board.
func runWave(ctx context.Context, cfg Config, l Launcher, wave []int, jobs int, results []Result) {
	sem := make(chan struct{}, jobs)
	var wg sync.WaitGroup
	for _, idx := range wave {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			inst := cfg.Instances[i]
			prompt := cfg.Board.buildPrompt(inst)
			out, code, err := l.Launch(ctx, inst.Agent, prompt, cfg.Timeout)
			r := Result{Agent: inst.Agent, Scope: inst.Scope, Output: out, ExitCode: code, Err: err}
			if err == nil && strings.TrimSpace(out) != "" {
				tags := []string(nil)
				if inst.Scope != "" {
					tags = []string{inst.Scope}
				}
				if perr := cfg.Board.Post(out, inst.Agent, inst.Scope, tags, ""); perr == nil {
					r.Posted = true
				}
			}
			results[i] = r
		}(idx)
	}
	wg.Wait()
}

// computeWaves topologically levels the instances by their scope dependencies.
// Each wave is a set of instance indices whose Needs are all satisfied by
// earlier waves. Errors on an unknown dependency scope or a cycle.
func computeWaves(instances []Instance) ([][]int, error) {
	producers := map[string][]int{}
	for i, inst := range instances {
		if inst.Scope != "" {
			producers[inst.Scope] = append(producers[inst.Scope], i)
		}
	}
	deps := make([]map[int]bool, len(instances))
	for i, inst := range instances {
		deps[i] = map[int]bool{}
		for _, need := range inst.Needs {
			ps, ok := producers[need]
			if !ok {
				return nil, fmt.Errorf("fanout: instance %d (%s) needs scope %q, but no instance produces it", i, inst.Scope, need)
			}
			for _, p := range ps {
				if p != i {
					deps[i][p] = true
				}
			}
		}
	}
	done := make([]bool, len(instances))
	var waves [][]int
	placed := 0
	for placed < len(instances) {
		var wave []int
		for i := 0; i < len(instances); i++ {
			if done[i] {
				continue
			}
			ready := true
			for d := range deps[i] {
				if !done[d] {
					ready = false
					break
				}
			}
			if ready {
				wave = append(wave, i)
			}
		}
		if len(wave) == 0 {
			return nil, fmt.Errorf("fanout: dependency cycle among instances")
		}
		for _, i := range wave {
			done[i] = true
		}
		placed += len(wave)
		waves = append(waves, wave)
	}
	return waves, nil
}

// buildPrompt assembles the shared context an instance sees at launch: the
// seed; the contributions it depends on (its Needs scopes, populated by earlier
// waves) or — for an independent instance — a scoped snapshot of its own lens;
// and its instruction. Board-aware agents can additionally call `bashy fanout
// read --scope` mid-run for live state.
func (b *Board) buildPrompt(inst Instance) string {
	var sb strings.Builder
	if len(inst.Needs) > 0 {
		// Dependent instance: show the prior work it builds on.
		seed, _, refs, _ := b.SeedText()
		sb.WriteString("=== BOARD: " + b.name + " ===\n")
		if seed != "" {
			sb.WriteString("SEED:\n")
			sb.WriteString(seed)
			sb.WriteString("\n")
		}
		if len(refs) > 0 {
			sb.WriteString("REFS: ")
			sb.WriteString(strings.Join(refs, ", "))
			sb.WriteString("\n")
		}
		need := map[string]bool{}
		for _, n := range inst.Needs {
			need[n] = true
		}
		posts, _ := b.Contributions()
		sb.WriteString("--- prior work you depend on (")
		sb.WriteString(strings.Join(inst.Needs, ", "))
		sb.WriteString(") ---\n")
		for _, c := range posts {
			if need[c.Scope] || anyTag(c.Tags, need) {
				sb.WriteString("• [")
				sb.WriteString(c.By)
				sb.WriteString("] ")
				sb.WriteString(c.Text)
				sb.WriteString("\n")
			}
		}
	} else {
		view, _ := b.Read(inst.Scope, inst.Agent, 0)
		sb.WriteString(view.Render())
	}
	sb.WriteString("\n")
	if inst.Scope != "" {
		sb.WriteString("Your lens: " + inst.Scope + "\n")
	}
	sb.WriteString("Your task:\n")
	sb.WriteString(inst.Instruction)
	return sb.String()
}

func anyTag(tags []string, want map[string]bool) bool {
	for _, t := range tags {
		if want[t] {
			return true
		}
	}
	return false
}
