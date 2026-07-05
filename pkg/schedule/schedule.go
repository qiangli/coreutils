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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	Kind      string    `json:"kind"` // cron | every | at
	Spec      string    `json:"spec"`
	Command   []string  `json:"command"`
	Dir       string    `json:"dir,omitempty"`
	Prompt    string    `json:"prompt,omitempty"`
	Context   string    `json:"context,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	LastRun   time.Time `json:"last_run,omitempty"`
	NextRun   time.Time `json:"next_run,omitempty"`
}

type store struct {
	Jobs []*Job `json:"jobs"`
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

func load() (*store, error) {
	s := &store{}
	b, err := os.ReadFile(statePath())
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

func (s *store) save() error {
	p := statePath()
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
	tmp.Close()
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

// fire runs a job's command with the agentic env injected, writing output to w.
func (j *Job) fire(w *os.File) error {
	if len(j.Command) == 0 {
		return fmt.Errorf("job %s has no command", j.ID)
	}
	c := exec.Command(j.Command[0], j.Command[1:]...)
	c.Dir = j.Dir
	c.Env = append(os.Environ(),
		"BASHY_SCHEDULE_JOB="+j.ID,
		"BASHY_SCHEDULE_PROMPT="+j.Prompt,
		"BASHY_SCHEDULE_CONTEXT="+j.Context,
	)
	c.Stdout, c.Stderr = w, w
	return c.Run()
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
				b, _ := json.Marshal(map[string]any{"schema_version": "bashy-schedule-v1", "kind": "list", "jobs": s.Jobs})
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
			err = j.fire(os.Stdout)
			j.LastRun = time.Now()
			_ = s.save()
			return err
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
	s, err := load()
	if err != nil {
		return nil, err
	}
	var fired []string
	changed := false
	for _, j := range s.Jobs {
		if !j.Enabled || j.NextRun.IsZero() || j.NextRun.After(now) {
			continue
		}
		_ = j.fire(w) // a failing job is logged via w; schedule continues
		j.LastRun = now
		fired = append(fired, j.ID)
		changed = true
		if j.Kind == "at" {
			j.Enabled = false // one-shot
			continue
		}
		if next, nerr := j.computeNext(now); nerr == nil {
			j.NextRun = next
		}
	}
	if changed {
		if err := s.save(); err != nil {
			return fired, err
		}
	}
	return fired, nil
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
