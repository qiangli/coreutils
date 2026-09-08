package schedule

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/qiangli/coreutils/pkg/llmbudget"
)

func TestExpensiveJobAdmissionRefusesBeforeExecution(t *testing.T) {
	zero := 0
	g := llmbudget.New(llmbudget.Config{StatePath: filepath.Join(t.TempDir(), "meter.json"), Policy: &llmbudget.Policy{Version: 1, Constraints: []llmbudget.Constraint{{HostSlots: &zero}}}})
	j := &Job{ID: "expensive", Command: []string{"a-command-that-must-never-launch"}, WorkBudget: &WorkBudget{}}
	if e := FireJobWithAdmission(j, io.Discard, nil, BudgetAdmission(g)); e == nil {
		t.Fatal("capacity refusal missing")
	}
	called := false
	e := FireJobWithAdmission(j, io.Discard, nil, func(ctx context.Context, j *Job) (func(error) error, error) {
		called = true
		return nil, context.Canceled
	})
	if !called || e != context.Canceled {
		t.Fatal("execution bypassed admission", e)
	}
}
func TestOrdinaryJobDoesNotConsumeWorkBudget(t *testing.T) {
	g := llmbudget.New(llmbudget.Config{StatePath: filepath.Join(t.TempDir(), "meter.json"), Policy: &llmbudget.Policy{Version: 1}})
	finish, e := BudgetAdmission(g)(context.Background(), &Job{ID: "posix", Kind: "at", POSIXCron: true})
	if e != nil {
		t.Fatal(e)
	}
	if e = finish(nil); e != nil {
		t.Fatal(e)
	}
	// Reporting a new gate must still find no reserved or metered rows.
	r, e := g.CollectReport(context.Background(), llmbudget.ReportOptions{Roster: []llmbudget.Binding{}})
	if e != nil || len(r.Accounts) != 0 {
		t.Fatal(r, e)
	}
}

func TestExpensiveJobHoldsCapacityUntilFinish(t *testing.T) {
	one := 1
	g := llmbudget.New(llmbudget.Config{StatePath: filepath.Join(t.TempDir(), "meter.json"), Policy: &llmbudget.Policy{Version: 1, Constraints: []llmbudget.Constraint{{HostSlots: &one}}}})
	j := &Job{ID: "running", WorkBudget: &WorkBudget{}}
	admit := BudgetAdmission(g)
	finish, e := admit(context.Background(), j)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = admit(context.Background(), &Job{ID: "second", WorkBudget: &WorkBudget{}}); e == nil {
		t.Fatal("running job capacity was free")
	}
	if e = finish(nil); e != nil {
		t.Fatal(e)
	}
	next, e := admit(context.Background(), &Job{ID: "next", WorkBudget: &WorkBudget{}})
	if e != nil {
		t.Fatal("ended job did not free capacity", e)
	}
	if e = next(nil); e != nil {
		t.Fatal(e)
	}
}
