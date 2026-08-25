// Package schedule is bashy's modern cron: `bashy schedule` runs commands on a
// cron expression, a fixed interval, or at a one-shot time, from a self-contained
// JSON store + an optional in-process daemon — no host crontab required (the
// host `cron`/`crontab` are left untouched and reachable as before).
//
// Agentic twist: every job may carry a `--prompt` (instruction) and `--context`.
// When the job fires they are passed to the command as `BASHY_SCHEDULE_PROMPT` /
// `BASHY_SCHEDULE_CONTEXT` (plus `BASHY_SCHEDULE_JOB`), so a scheduled agent
// wakes up *with a task in hand* — the primitive a conductor uses to self-wake a
// long-running campaign, e.g.
//
//	bashy schedule add --every 30m --prompt "re-drive stalled stories" \
//	  -- bashy weave autopilot
package schedule

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/weavecli"
)

// Job is one scheduled command.
type Job struct {
	ID       string   `json:"id"`
	Name     string   `json:"name,omitempty"`
	Kind     string   `json:"kind"` // cron | every | at
	Spec     string   `json:"spec"`
	Command  []string `json:"command"`
	Dir      string   `json:"dir,omitempty"`
	Queue    string   `json:"queue,omitempty"`
	Stdin    string   `json:"stdin,omitempty"`
	StdinSet bool     `json:"stdin_set,omitempty"`
	// POSIXCron marks jobs whose shell and umask semantics must be enforced.
	// It makes a moved store fail closed on platforms that cannot provide them.
	POSIXCron bool `json:"posix_cron,omitempty"`
	// Env and Umask preserve the submission context for POSIX at jobs.  The
	// corresponding Set bits distinguish an intentionally empty value from a
	// legacy/general scheduler job, which continues to inherit daemon state.
	// The state file is private (0600), and list output must never expose Env.
	Env        []string `json:"env,omitempty"`
	EnvSet     bool     `json:"env_set,omitempty"`
	Umask      uint32   `json:"umask,omitempty"`
	UmaskSet   bool     `json:"umask_set,omitempty"`
	MailOutput bool     `json:"mail_output,omitempty"`
	// MailCompletion records POSIX at -m / batch behavior: completion mail is
	// required even when the job produced no output. It fails closed unless the
	// runner supplies an explicit MailDelivery; output-only mail fails only when
	// output was produced and no provider exists.
	MailCompletion bool   `json:"mail_completion,omitempty"`
	MailTo         string `json:"mail_to,omitempty"`
	// BatchLoad marks jobs submitted through batch(1). The scheduler currently
	// records the required load-governed semantics but does not claim a precise
	// host load implementation.
	BatchLoad bool      `json:"batch_load,omitempty"`
	Prompt    string    `json:"prompt,omitempty"`
	Context   string    `json:"context,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	LastRun   time.Time `json:"last_run,omitempty"`
	NextRun   time.Time `json:"next_run,omitempty"`
}

var ErrMailDeliveryUnsupported = errors.New("scheduled output mail delivery is not supported by this host")

type MailDelivery func(recipient string, content []byte) error

type store struct {
	Jobs          []*Job `json:"jobs"`
	CronSource    []byte `json:"cron_source,omitempty"`
	CronSourceSet bool   `json:"cron_source_set,omitempty"`
}

func statePath() string {
	if p := os.Getenv("BASHY_SCHEDULE_STATE"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "bashy", "schedule.json")
}

func load() (*store, error) { return loadPath(statePath()) }

func loadPath(path string) (*store, error) {
	s := &store{}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return s, err
	}
	return s, nil
}

func (s *store) save() error { return s.savePath(statePath()) }

func (s *store) savePath(p string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".schedule-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), p)
}

func (s *store) find(id string) *Job {
	for _, j := range s.Jobs {
		if j.ID == id || j.Name == id {
			return j
		}
	}
	return nil
}

// computeNext returns the next fire time at/after now for the job's schedule.
func (j *Job) computeNext(now time.Time) (time.Time, error) {
	switch j.Kind {
	case "cron":
		sched, err := cron.ParseStandard(j.Spec)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid cron %q: %w", j.Spec, err)
		}
		return sched.Next(now), nil
	case "every":
		d, err := time.ParseDuration(j.Spec)
		if err != nil || d <= 0 {
			return time.Time{}, fmt.Errorf("invalid interval %q", j.Spec)
		}
		base := j.LastRun
		if base.IsZero() {
			base = j.CreatedAt
		}
		n := base.Add(d)
		for !n.After(now) {
			n = n.Add(d)
		}
		return n, nil
	case "at":
		t, err := parseAt(j.Spec, now)
		if err != nil {
			return time.Time{}, err
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unknown schedule kind %q", j.Kind)
}

// parseAt accepts RFC3339, "2006-01-02 15:04", or "15:04" (today, or tomorrow if
// already past).
func parseAt(s string, now time.Time) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04", "2006-01-02T15:04"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	if t, err := time.ParseInLocation("15:04", s, time.Local); err == nil {
		today := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
		if !today.After(now) {
			today = today.Add(24 * time.Hour)
		}
		return today, nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q (want RFC3339, \"2006-01-02 15:04\", or \"15:04\")", s)
}

// fire runs a job without a mail provider. Mail-requiring jobs fail closed.
func (j *Job) fire(w io.Writer) error { return j.fireWithMail(w, nil) }

// FireJob executes a job with an explicit output-mail provider.
func FireJob(j *Job, w io.Writer, deliver MailDelivery) error {
	return j.fireWithMail(w, deliver)
}

func (j *Job) fireWithMail(w io.Writer, deliver MailDelivery) error {
	if len(j.Command) == 0 {
		return fmt.Errorf("job %s has no command", j.ID)
	}
	if err := validateJobPlatform(j); err != nil {
		return fmt.Errorf("job %s: %w", j.ID, err)
	}
	if j.MailCompletion && deliver == nil {
		return fmt.Errorf("job %s: %w", j.ID, ErrMailDeliveryUnsupported)
	}
	c := commandWithUmask(j.Command, j.Umask, j.UmaskSet)
	c.Dir = j.Dir
	if j.EnvSet {
		c.Env = append([]string(nil), j.Env...)
	} else {
		c.Env = os.Environ()
	}
	// A POSIX at-job must observe exactly the environment captured when it
	// was submitted. Agentic scheduler jobs retain their existing metadata.
	if j.Kind != "at" {
		c.Env = append(c.Env,
			"BASHY_SCHEDULE_JOB="+j.ID,
			"BASHY_SCHEDULE_PROMPT="+j.Prompt,
			"BASHY_SCHEDULE_CONTEXT="+j.Context,
		)
	}
	if j.StdinSet {
		c.Stdin = strings.NewReader(j.Stdin)
	}
	if !j.MailOutput {
		c.Stdout, c.Stderr = w, w
		return c.Run()
	}
	var output bytes.Buffer
	c.Stdout, c.Stderr = &output, &output
	runErr := c.Run()
	if output.Len() > 0 {
		if deliver == nil {
			return errors.Join(runErr, fmt.Errorf("job %s: %w", j.ID, ErrMailDeliveryUnsupported))
		}
		if err := deliver(j.MailTo, append([]byte(nil), output.Bytes()...)); err != nil {
			return fmt.Errorf("job %s: cannot deliver output mail: %w", j.ID, err)
		}
	} else if j.MailCompletion {
		status := "completed successfully"
		if runErr != nil {
			status = "completed with error: " + runErr.Error()
		}
		msg := []byte(fmt.Sprintf("at job %s %s\n", j.ID, status))
		if err := deliver(j.MailTo, msg); err != nil {
			return fmt.Errorf("job %s: cannot deliver completion mail: %w", j.ID, err)
		}
	}
	return runErr
}

// scheduleOutputJSON resolves whether to emit the JSON envelope, honoring
// $BASHY_AGENTIC with an explicit --json / --plain / --json=false override —
// the same precedence weave and dag use.
func scheduleOutputJSON(cmd *cobra.Command) bool {
	jsonF, _ := cmd.Flags().GetBool("json")
	plainF, _ := cmd.Flags().GetBool("plain")
	quietF, _ := cmd.Flags().GetBool("quiet")
	return weavecli.ResolveOutputModeEx(cmd.Flags().Changed("json"), jsonF, plainF, quietF) == weavecli.OutputJSON
}

// NewScheduleCmd builds the `bashy schedule` command tree.
func NewScheduleCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "schedule",
		Short: "Modern cron: run commands on a cron/interval/at schedule, with an agentic prompt",
	}
	root.PersistentFlags().Bool("json", false, "machine-readable JSON envelope")
	root.PersistentFlags().Bool("plain", false, "plain-text output (overrides $BASHY_AGENTIC)")
	root.PersistentFlags().Bool("quiet", false, "minimal output")
	root.AddCommand(addCmd(), listCmd(), rmCmd(), runCmd(), tickCmd(), daemonCmd(), startCmd(), statusCmd(), stopServiceCmd())
	return root
}

func addCmd() *cobra.Command {
	var cronExpr, every, at, name, prompt, ctx string
	c := &cobra.Command{
		Use:   "add [flags] -- command [args...]",
		Short: "Add a scheduled job",
		RunE: func(cmd *cobra.Command, args []string) error {
			kinds := 0
			kind, spec := "", ""
			if cronExpr != "" {
				kind, spec, kinds = "cron", cronExpr, kinds+1
			}
			if every != "" {
				kind, spec, kinds = "every", every, kinds+1
			}
			if at != "" {
				kind, spec, kinds = "at", at, kinds+1
			}
			if kinds != 1 {
				return fmt.Errorf("specify exactly one of --cron, --every, --at")
			}
			if len(args) == 0 {
				return fmt.Errorf("a command is required (after --)")
			}
			now := time.Now()
			cwd, _ := os.Getwd()
			j := &Job{
				ID: strconv.FormatInt(now.UnixNano(), 36), Name: name,
				Kind: kind, Spec: spec, Command: args, Dir: cwd,
				Prompt: prompt, Context: ctx, Enabled: true, CreatedAt: now,
			}
			next, err := j.computeNext(now)
			if err != nil {
				return err
			}
			j.NextRun = next
			s, err := load()
			if err != nil {
				return err
			}
			s.Jobs = append(s.Jobs, j)
			if err := s.save(); err != nil {
				return err
			}
			if scheduleOutputJSON(cmd) {
				b, _ := json.Marshal(map[string]any{"schema_version": "bashy-schedule-v1", "kind": "added", "id": j.ID, "next_run": j.NextRun})
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "scheduled %s (%s %s) next %s\n", j.ID, j.Kind, j.Spec, j.NextRun.Format(time.RFC3339))
			}
			return nil
		},
	}
	c.Flags().StringVar(&cronExpr, "cron", "", "5-field cron expression (e.g. \"*/15 * * * *\")")
	c.Flags().StringVar(&every, "every", "", "fixed interval (e.g. 30m, 2h)")
	c.Flags().StringVar(&at, "at", "", "one-shot time (RFC3339, \"2006-01-02 15:04\", or \"15:04\")")
	c.Flags().StringVar(&name, "name", "", "optional human name")
	c.Flags().StringVar(&prompt, "prompt", "", "agentic: instruction passed as BASHY_SCHEDULE_PROMPT when the job fires")
	c.Flags().StringVar(&ctx, "context", "", "agentic: context passed as BASHY_SCHEDULE_CONTEXT when the job fires")
	return c
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List scheduled jobs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := load()
			if err != nil {
				return err
			}
			if scheduleOutputJSON(cmd) {
				b, _ := json.Marshal(map[string]any{"schema_version": "bashy-schedule-v1", "kind": "list", "jobs": publicJobs(s.Jobs)})
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
				return nil
			}
			if len(s.Jobs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no scheduled jobs")
				return nil
			}
			for _, j := range s.Jobs {
				state := "on"
				if !j.Enabled {
					state = "off"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s [%s] %s %s -> %s  next=%s\n",
					j.ID, state, j.Kind, j.Spec, strings.Join(j.Command, " "), j.NextRun.Format(time.RFC3339))
			}
			return nil
		},
	}
}

// publicJobs returns listing copies with captured environment values removed.
// at spool state needs those values to meet POSIX, but a diagnostic/listing
// surface must not turn that private state into a credential disclosure.
func publicJobs(jobs []*Job) []*Job {
	out := make([]*Job, 0, len(jobs))
	for _, job := range jobs {
		clone := *job
		clone.Env = nil
		clone.Stdin = ""
		clone.StdinSet = false
		out = append(out, &clone)
	}
	return out
}

func rmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>",
		Short: "Remove a scheduled job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := load()
			if err != nil {
				return err
			}
			kept := s.Jobs[:0]
			found := false
			for _, j := range s.Jobs {
				if j.ID == args[0] || j.Name == args[0] {
					found = true
					continue
				}
				kept = append(kept, j)
			}
			if !found {
				return fmt.Errorf("no such job %q", args[0])
			}
			s.Jobs = kept
			if err := s.save(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", args[0])
			return nil
		},
	}
}

func runCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <id>",
		Short: "Run a scheduled job now (ignores its schedule)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := load()
			if err != nil {
				return err
			}
			j := s.find(args[0])
			if j == nil {
				return fmt.Errorf("no such job %q", args[0])
			}
			if err = j.fire(os.Stdout); err != nil {
				return err
			}
			j.LastRun = time.Now()
			return s.save()
		},
	}
}

// tick fires every enabled job that is due, then reschedules it (one-shot `at`
// jobs are disabled after firing). Idempotent — wire it to the daemon, a host
// cron line, or call by hand.
func tickCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tick",
		Short: "Fire all due jobs once, then reschedule (idempotent)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fired, err := tickOnce(time.Now(), os.Stdout)
			if err != nil {
				return err
			}
			if scheduleOutputJSON(cmd) {
				b, _ := json.Marshal(map[string]any{"schema_version": "bashy-schedule-v1", "kind": "tick", "fired": fired})
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "tick: fired %d job(s)\n", len(fired))
			}
			return nil
		},
	}
}

// tickOnce is the testable core: fire due jobs as of now, return their ids.
func tickOnce(now time.Time, w *os.File) ([]string, error) {
	return tickOnceWithMail(now, w, nil)
}

func tickOnceWithMail(now time.Time, w io.Writer, deliver MailDelivery) ([]string, error) {
	var fired []string
	var due []*Job
	err := UpdateJobs(func(jobs []*Job) ([]*Job, error) {
		for _, j := range jobs {
			if !j.Enabled || j.NextRun.IsZero() || j.NextRun.After(now) {
				continue
			}
			if err := validateJobPlatform(j); err != nil {
				return nil, fmt.Errorf("job %s: %w", j.ID, err)
			}
			if j.MailCompletion && deliver == nil {
				return nil, fmt.Errorf("job %s: %w", j.ID, ErrMailDeliveryUnsupported)
			}
			j.LastRun = now
			fired = append(fired, j.ID)
			copy := *j
			copy.Command = append([]string(nil), j.Command...)
			copy.Env = append([]string(nil), j.Env...)
			due = append(due, &copy)
			if j.Kind == "at" {
				j.Enabled = false // one-shot
				continue
			}
			if next, nerr := j.computeNext(now); nerr == nil {
				j.NextRun = next
			}
		}
		return jobs, nil
	})
	if err != nil {
		return fired, err
	}
	// The transaction claims each due job by disabling/rescheduling it before
	// execution. Run commands after releasing the store lock so a scheduled
	// command may submit another job without deadlocking the daemon.
	var fireErrs []error
	for _, j := range due {
		if err := j.fireWithMail(w, deliver); err != nil {
			fmt.Fprintln(w, err)
			fireErrs = append(fireErrs, err)
		}
	}
	return fired, errors.Join(fireErrs...)
}

// TickOnceWithMail fires due jobs using an explicit mail provider.
func TickOnceWithMail(now time.Time, w io.Writer, deliver MailDelivery) ([]string, error) {
	return tickOnceWithMail(now, w, deliver)
}

func daemonCmd() *cobra.Command {
	var interval time.Duration
	c := &cobra.Command{
		Use:   "daemon",
		Short: "Run a foreground scheduler loop, firing due jobs on an interval",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Own the service pidfile so `schedule status` is accurate even when the
			// daemon is launched directly (not via `schedule start`), and so a clean
			// exit removes it. StartService also writes it for race-free readiness.
			p := servicePidPath()
			_ = writePid(p, os.Getpid())
			defer func() {
				if pid, _ := readPid(p); pid == os.Getpid() {
					_ = os.Remove(p)
				}
			}()
			fmt.Fprintf(cmd.ErrOrStderr(), "schedule daemon: ticking every %s (state %s)\n", interval, statePath())
			t := time.NewTicker(interval)
			defer t.Stop()
			for {
				select {
				case <-cmd.Context().Done():
					return nil
				case <-t.C:
					if _, err := tickOnce(time.Now(), os.Stdout); err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "schedule daemon: %v\n", err)
					}
				}
			}
		},
	}
	c.Flags().DurationVar(&interval, "interval", time.Minute, "how often to check for due jobs")
	return c
}
