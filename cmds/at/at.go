// Package atcmd implements at(1): execute a command at a specified time.
// Reads the command from stdin (or -f FILE) and schedules it as a
// one-shot job through the bashy schedule store.
//
//	at [-f FILE] TIMESPEC
//	at -l            — list pending jobs (same as atq)
//	at -r JOBID      — remove a job (same as atrm)
package atcmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "at",
	Synopsis: "Schedule a command to run at a specified time.",
	Usage:    "at [-f FILE] TIMESPEC\n   or: at -l\n   or: at -r JOBID",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	filename := fs.StringP("file", "f", "", "read the job from FILE rather than standard input")
	listFlag := fs.BoolP("list", "l", false, "list pending jobs (same as atq)")
	removeFlag := fs.BoolP("remove", "r", false, "remove job(s)")
	queue := fs.StringP("queue", "q", "", "use the named single-letter queue")
	touchTime := fs.StringP("time", "t", "", "schedule using [[CC]YY]MMDDhhmm[.SS]")

	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	if *queue != "" && !validQueue(*queue) {
		return tool.UsageError(rc, cmd, "invalid queue %q", *queue)
	}
	if *listFlag {
		if code := checkAtAccess(rc); code != 0 {
			return code
		}
		return listJobs(rc, operands, *queue)
	}
	if *removeFlag {
		if code := checkAtAccess(rc); code != 0 {
			return code
		}
		return removeJobs(rc, operands)
	}

	if *touchTime == "" && len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing timespec")
	}
	if *touchTime != "" && len(operands) != 0 {
		return tool.UsageError(rc, cmd, "-t and a timespec operand are mutually exclusive")
	}
	if code := checkAtAccess(rc); code != 0 {
		return code
	}

	loc, err := atLocation(rc.Getenv("TZ"))
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}
	now := time.Now().In(loc)
	timespec := strings.Join(operands, " ")
	var when time.Time
	if *touchTime != "" {
		when, err = schedule.ParseAtTouchTime(*touchTime, now, loc)
	} else {
		when, err = schedule.ParseAtTimespecInLocation(timespec, now, loc)
	}
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}

	if when.Before(now) {
		return tool.UsageError(rc, cmd, "time %q is in the past", timespec)
	}

	var cmdText string
	if *filename != "" {
		data, err := os.ReadFile(rc.Path(*filename))
		if err != nil {
			return tool.UsageError(rc, cmd, "cannot read file %q: %v", *filename, err)
		}
		cmdText = string(data)
	} else {
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, rc.In); err != nil {
			return tool.UsageError(rc, cmd, "cannot read stdin: %v", err)
		}
		cmdText = buf.String()
	}

	cmdText = strings.TrimSpace(cmdText)
	if cmdText == "" {
		return tool.UsageError(rc, cmd, "no command given")
	}

	id := strconv.FormatInt(now.UnixNano(), 36)
	cwd := rc.Dir
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	shell := rc.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}

	j := &schedule.Job{
		ID:   id,
		Kind: "at",
		Spec: when.Format(time.RFC3339),
		// POSIX defines the input as shell command language, not an argv to
		// split on whitespace. Preserve the complete program for a separate
		// non-interactive shell invocation so redirections, pipelines,
		// expansions, and multi-line constructs retain their meaning.
		Command:   []string{shell, "-c", cmdText},
		Dir:       cwd,
		Queue:     queueOrDefault(*queue),
		Env:       append([]string(nil), rc.Env...),
		EnvSet:    true,
		Enabled:   true,
		CreatedAt: now,
		NextRun:   when,
	}
	if rc.UmaskSet {
		j.Umask, j.UmaskSet = uint32(rc.Umask.Perm()), true
	} else if mask, ok := processUmask(); ok {
		j.Umask, j.UmaskSet = mask, true
	}

	if err := schedule.UpdateJobs(func(jobs []*schedule.Job) ([]*schedule.Job, error) {
		return append(jobs, j), nil
	}); err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot save schedule: %v\n", cmd.Name, err)
		return 1
	}
	fmt.Fprintf(rc.Err, "job %s at %s\n", id, formatJobTime(when))
	return 0
}

func listJobs(rc *tool.RunContext, ids []string, queue string) int {
	jobs, err := schedule.LoadJobs()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot load schedule: %v\n", cmd.Name, err)
		return 1
	}
	for _, j := range jobs {
		if j.Kind != "at" || !j.Enabled {
			continue
		}
		if queue != "" && queueOrDefault(j.Queue) != queue {
			continue
		}
		if len(ids) > 0 && !containsID(ids, j.ID, j.Name) {
			continue
		}
		fmt.Fprintf(rc.Out, "%s\t%s\n", j.ID, formatJobTime(j.NextRun))
	}
	return 0
}

func validQueue(queue string) bool {
	return len(queue) == 1 && queue[0] >= 'a' && queue[0] <= 'z'
}

func queueOrDefault(queue string) string {
	if queue == "" {
		return "a"
	}
	return queue
}

func formatJobTime(t time.Time) string {
	return t.Format("Mon Jan _2 15:04:05 2006")
}

func containsID(ids []string, id, name string) bool {
	for _, candidate := range ids {
		if candidate == id || candidate == name {
			return true
		}
	}
	return false
}

func removeJobs(rc *tool.RunContext, ids []string) int {
	if len(ids) == 0 {
		return tool.UsageError(rc, cmd, "missing job ID for -r")
	}
	if err := schedule.UpdateJobs(func(jobs []*schedule.Job) ([]*schedule.Job, error) {
		for _, id := range ids {
			found := false
			for i := len(jobs) - 1; i >= 0; i-- {
				if jobs[i].ID == id || jobs[i].Name == id {
					jobs = append(jobs[:i], jobs[i+1:]...)
					found = true
				}
			}
			if !found {
				fmt.Fprintf(rc.Err, "%s: no job %q\n", cmd.Name, id)
			}
		}
		return jobs, nil
	}); err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot save schedule: %v\n", cmd.Name, err)
		return 1
	}
	return 0
}
