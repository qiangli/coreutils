package foreman

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/chat"
	"github.com/qiangli/coreutils/pkg/dag"
)

type DAGOptions struct {
	Path       string
	Targets    []string
	SteerPause time.Duration
}

type DAGReport struct {
	Targets []string `json:"targets"`
}

func (s *Session) RunDAG(ctx context.Context, opt DAGOptions) (DAGReport, error) {
	doc, err := dag.ParseFile(opt.Path)
	if err != nil {
		return DAGReport{}, err
	}
	g, err := dag.BuildGraph(doc)
	if err != nil {
		return DAGReport{}, err
	}
	if len(opt.Targets) > 0 {
		g, err = g.Subgraph(opt.Targets...)
		if err != nil {
			return DAGReport{}, err
		}
	}
	order, err := g.TopoSort()
	if err != nil {
		return DAGReport{}, err
	}
	pause := opt.SteerPause
	if pause == 0 {
		pause = 100 * time.Millisecond
	}
	report := DAGReport{Targets: make([]string, 0, len(order))}
	for i, node := range order {
		if err := s.ProcessPending(ctx); err != nil {
			return report, err
		}
		if s.State().Stopped {
			return report, nil
		}
		if s.shouldSkip(node.Task.Name) {
			continue
		}
		if err := s.runDAGTarget(ctx, filepath.Dir(opt.Path), node.Task); err != nil {
			return report, err
		}
		report.Targets = append(report.Targets, node.Task.Name)
		if i != len(order)-1 {
			select {
			case <-ctx.Done():
				return report, ctx.Err()
			case <-time.After(pause):
			}
			if err := s.ProcessPending(ctx); err != nil {
				return report, err
			}
		}
	}
	return report, s.store.SaveState(s.State())
}

func (s *Session) shouldSkip(target string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.EqualFold(s.state.CurrentStep, "skip:"+target)
}

func (s *Session) runDAGTarget(ctx context.Context, dir string, task *dag.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Status = StatusWorking
	s.state.CurrentStep = task.Name
	prompt := s.composeDAGPrompt(task)
	res, err := chat.Invoke(ctx, chat.Options{
		Agent:       s.state.Agent,
		Role:        s.state.Role,
		Instruction: prompt,
		Cwd:         firstNonEmpty(s.state.Cwd, dir),
	}, s.runner)
	if res.Output != "" {
		s.history = append(s.history, "agent: "+strings.TrimSpace(res.Output))
	}
	if err != nil || res.ExitCode != 0 {
		s.state.Status = StatusBlocked
		if err != nil {
			return err
		}
		return fmt.Errorf("foreman: runner exited %d", res.ExitCode)
	}
	s.state.Status = StatusIdle
	return s.store.SaveState(s.state)
}

func (s *Session) composeDAGPrompt(task *dag.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Goal:\n%s\n\n", s.state.Goal)
	if note := s.kbPreamble(); note != "" {
		b.WriteString(note)
		b.WriteByte('\n')
	}
	if len(s.history) > 0 {
		b.WriteString("Session history:\n")
		for _, h := range s.history {
			b.WriteString(h)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "DAG target: %s\n", task.Name)
	if strings.TrimSpace(task.Desc) != "" {
		fmt.Fprintf(&b, "Description:\n%s\n\n", strings.TrimSpace(task.Desc))
	}
	if strings.TrimSpace(task.Body) != "" {
		fmt.Fprintf(&b, "Body:\n%s\n", strings.TrimSpace(task.Body))
	}
	return b.String()
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
