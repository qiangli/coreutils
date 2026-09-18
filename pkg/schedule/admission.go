package schedule

import (
	"context"
	"errors"
)

// WorkBudget explicitly marks an owned expensive job. Ordinary/POSIX jobs have
// nil WorkBudget and retain their existing execution semantics. A model harness
// has opaque token usage; memory is unknown unless explicitly allocated.
type WorkBudget struct {
	Model       string  `json:"model,omitempty"`
	Agent       string  `json:"agent,omitempty"`
	MemoryBytes *uint64 `json:"memory_bytes,omitempty"`
}

// ErrBudgetJobNotStarted and ErrBudgetJobLifetime are the finish reasons an
// admission callback distinguishes: a reservation for a job that never
// launched is released outright; one whose child lifetime could not be
// verified stays reserved for lifecycle reconciliation.
var ErrBudgetJobNotStarted = errors.New("budget job never started")
var ErrBudgetJobLifetime = errors.New("budget child lifetime unverified")

type JobAdmission func(context.Context, *Job) (finish func(error) error, err error)

// DefaultAdmission is consulted by every job fire that does not name its own
// JobAdmission. It is nil here — an ordinary POSIX at/batch/cron job needs no
// admission — and github.com/qiangli/yoke/pkg/llmbudget sets it at init to the
// LLM-budget gate, so a bashy that links yoke admits a WorkBudget-marked job
// through the meter while the bare coreutils multicall never links the meter.
// This is the seam that keeps the certified package free of the agentic
// budget stack; it is a variable, not an interface.
var DefaultAdmission JobAdmission

// FireJobWithAdmission is the embedding/test seam. The callback is consulted
// immediately before execution, and its finish callback runs after command exit.
func FireJobWithAdmission(j *Job, w interface{ Write([]byte) (int, error) }, deliver MailDelivery, admit JobAdmission) error {
	return j.fireWithAdmission(w, deliver, admit)
}
func combineAdmissionError(runErr, finishErr error) error { return errors.Join(runErr, finishErr) }
