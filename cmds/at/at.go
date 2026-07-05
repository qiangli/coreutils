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

	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	if *listFlag {
		return listJobs(rc)
	}
	if *removeFlag {
		return removeJobs(rc, operands)
	}

	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing timespec")
	}

	timespec := strings.Join(operands, " ")
	now := time.Now()
	when, err := schedule.ParseAtTimespec(timespec, now)
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}

	if !when.After(now) {
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
	cwd, _ := os.Getwd()
	parts := tokenize(cmdText)

	j := &schedule.Job{
		ID:        id,
		Kind:      "at",
		Spec:      when.Format(time.RFC3339),
		Command:   parts,
		Dir:       cwd,
		Enabled:   true,
		CreatedAt: now,
		NextRun:   when,
	}

	jobs, err := schedule.LoadJobs()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot load schedule: %v\n", cmd.Name, err)
		return 1
	}
	jobs = append(jobs, j)
	if err := schedule.SaveJobs(jobs); err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot save schedule: %v\n", cmd.Name, err)
		return 1
	}
	fmt.Fprintf(rc.Out, "job %s at %s\n", id, when.Format(time.RFC3339))
	return 0
}

func listJobs(rc *tool.RunContext) int {
	jobs, err := schedule.LoadJobs()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot load schedule: %v\n", cmd.Name, err)
		return 1
	}
	found := false
	for _, j := range jobs {
		if j.Kind != "at" || !j.Enabled {
			continue
		}
		found = true
		fmt.Fprintf(rc.Out, "%s\t%s\t%s\n", j.ID, j.NextRun.Format(time.RFC3339), strings.Join(j.Command, " "))
	}
	if !found {
		fmt.Fprintln(rc.Out, "no pending at jobs")
	}
	return 0
}

func removeJobs(rc *tool.RunContext, ids []string) int {
	if len(ids) == 0 {
		return tool.UsageError(rc, cmd, "missing job ID for -r")
	}
	jobs, err := schedule.LoadJobs()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot load schedule: %v\n", cmd.Name, err)
		return 1
	}
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
	if err := schedule.SaveJobs(jobs); err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot save schedule: %v\n", cmd.Name, err)
		return 1
	}
	return 0
}

func tokenize(s string) []string {
	return strings.Fields(s)
}
