package fanout

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Instance is one seat in the fan-out: an agent (tool:model or nick) running a
// per-instance instruction variant under a lens (scope).
type Instance struct {
	Agent       string
	Instruction string
	Scope       string
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

// Run fans the instances out concurrently against the shared board. Each
// instance gets the seed + a SCOPED view of the board + its instruction; its
// output is posted back to the board under its scope. Read-shared /
// write-to-board: no instance mutates another's workspace, so no isolation is
// needed. Returns one Result per instance, in input order.
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
	jobs := cfg.Jobs
	if jobs <= 0 || jobs > len(cfg.Instances) {
		jobs = len(cfg.Instances)
	}

	results := make([]Result, len(cfg.Instances))
	sem := make(chan struct{}, jobs)
	var wg sync.WaitGroup

	for i, inst := range cfg.Instances {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, inst Instance) {
			defer wg.Done()
			defer func() { <-sem }()

			prompt := cfg.Board.buildPrompt(inst)
			out, code, err := l.Launch(ctx, inst.Agent, prompt, cfg.Timeout)
			r := Result{Agent: inst.Agent, Scope: inst.Scope, Output: out, ExitCode: code, Err: err}
			// Post the instance's output back to the board as its contribution,
			// tagged by its scope, so later readers can find it by lens.
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
		}(i, inst)
	}
	wg.Wait()
	return results, nil
}

// buildPrompt assembles the shared context an instance sees at launch: the
// seed, a scoped snapshot of any prior contributions (its P2 view), and its
// instruction. Board-aware agents can additionally call `bashy fanout read
// --scope` mid-run for live state.
func (b *Board) buildPrompt(inst Instance) string {
	var sb strings.Builder
	view, _ := b.Read(inst.Scope, inst.Agent, 0)
	sb.WriteString(view.Render())
	sb.WriteString("\n")
	if inst.Scope != "" {
		sb.WriteString("Your lens: " + inst.Scope + "\n")
	}
	sb.WriteString("Your task:\n")
	sb.WriteString(inst.Instruction)
	return sb.String()
}
