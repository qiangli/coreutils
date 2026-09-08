package schedule

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/qiangli/coreutils/pkg/llmbudget"
)

// WorkBudget explicitly marks an owned expensive job. Ordinary/POSIX jobs have
// nil WorkBudget and retain their existing execution semantics. A model harness
// has opaque token usage; memory is unknown unless explicitly allocated.
type WorkBudget struct {
	Model       string  `json:"model,omitempty"`
	Agent       string  `json:"agent,omitempty"`
	MemoryBytes *uint64 `json:"memory_bytes,omitempty"`
}

type JobAdmission func(context.Context, *Job) (finish func(error) error, err error)

// BudgetAdmission can be embedded with an isolated Gate. Nil uses the normal
// process policy authority. It never applies to an unmarked job.
func BudgetAdmission(g *llmbudget.Gate) JobAdmission {
	if g == nil {
		g = llmbudget.DefaultGate()
	}
	return func(ctx context.Context, j *Job) (func(error) error, error) {
		if j.WorkBudget == nil {
			return func(error) error { return nil }, nil
		}
		newOwner := llmbudget.NewOwner
		reserve := llmbudget.Reserve
		renew := llmbudget.Renew
		release := llmbudget.Release
		reconcile := llmbudget.ReconcileTerminated
		if g != nil {
			newOwner = g.NewOwner
			reserve = g.Reserve
			renew = g.Renew
			release = g.Release
			reconcile = g.ReconcileTerminated
		}
		o, e := newOwner(ctx, "schedule "+j.ID)
		if e != nil {
			return nil, e
		}
		host, _ := os.Hostname()
		r := llmbudget.Request{ID: "schedule-" + o.ID(), Owner: o.ID(), Run: j.ID, Host: host, Model: j.WorkBudget.Model, Agent: j.WorkBudget.Agent, HostSlots: 1, UnknownMemory: j.WorkBudget.MemoryBytes == nil, TTL: 2 * time.Minute}
		if j.WorkBudget.MemoryBytes != nil {
			r.MemoryBytes = *j.WorkBudget.MemoryBytes
		}
		if r.Model != "" {
			r.Concurrency = 1
			r.UnknownTokens = true
		}
		a, e := reserve(ctx, r)
		if e != nil || a.Reservation == nil || a.Decision.Action != llmbudget.Allow {
			o.Close()
			if e != nil {
				return nil, e
			}
			return nil, fmt.Errorf("schedule: budget %s: %s", a.Decision.Action, a.Decision.Reason)
		}
		stop := make(chan struct{})
		go func() {
			t := time.NewTicker(30 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-stop:
					return
				case <-t.C:
					c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_ = renew(c, r.ID, r.Owner, 2*time.Minute)
					cancel()
				}
			}
		}()
		return func(runErr error) error {
			close(stop)
			defer o.Close()
			if runErr != nil {
				return nil
			} // uncertain failed work remains reserved for lifecycle reconciliation
			c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if r.Model == "" {
				return release(c, r.ID, r.Owner)
			}
			return reconcile(c, r.ID, llmbudget.TerminationProof{Owner: r.Owner, Run: r.Run, Host: r.Host, VerifiedAt: time.Now().UTC(), Evidence: "owned scheduled command returned successfully; usage remains unknown"})
		}, nil
	}
}

// FireJobWithAdmission is the embedding/test seam. The callback is consulted
// immediately before execution, and its finish callback runs after command exit.
func FireJobWithAdmission(j *Job, w interface{ Write([]byte) (int, error) }, deliver MailDelivery, admit JobAdmission) error {
	return j.fireWithAdmission(w, deliver, admit)
}
func combineAdmissionError(runErr, finishErr error) error { return errors.Join(runErr, finishErr) }
