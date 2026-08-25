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

	posixlocale "github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "at",
	Synopsis: "Schedule a command to run at a specified time.",
	Usage: "at [-m] [-f FILE] [-q QUEUE] -t TIME\n" +
		"   or: at [-m] [-f FILE] [-q QUEUE] TIMESPEC...\n" +
		"   or: at -r JOBID...\n" +
		"   or: at -l -q QUEUE\n" +
		"   or: at -l [JOBID...]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	filename := fs.StringP("file", "f", "", "read the job from FILE rather than standard input")
	listFlag := fs.BoolP("list", "l", false, "list pending jobs (same as atq)")
	mailCompletion := fs.BoolP("mail", "m", false, "mail completion even when the job produces no output")
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
	modes := 0
	if *listFlag {
		modes++
	}
	if *removeFlag {
		modes++
	}
	if modes > 1 {
		return tool.UsageError(rc, cmd, "-l and -r are mutually exclusive")
	}
	if *listFlag {
		if *filename != "" || *touchTime != "" || *mailCompletion {
			return tool.UsageError(rc, cmd, "-l cannot be combined with -f, -m, or -t")
		}
		if *queue != "" && len(operands) != 0 {
			return tool.UsageError(rc, cmd, "-l -q does not accept job IDs")
		}
		if code := checkAtAccess(rc); code != 0 {
			return code
		}
		return listJobs(rc, operands, *queue)
	}
	if *removeFlag {
		if *filename != "" || *queue != "" || *touchTime != "" || *mailCompletion {
			return tool.UsageError(rc, cmd, "-r cannot be combined with -f, -m, -q, or -t")
		}
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
	identity, err := schedule.AuthenticatedIdentity()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", cmd.Name, err)
		return 1
	}

	loc, err := atLocation(rc.Getenv("TZ"))
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}
	now := time.Now().In(loc)
	formatter, err := posixlocale.ResolveTime(rc.Env)
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}
	timespec := strings.Join(operands, " ")
	var when time.Time
	if *touchTime != "" {
		when, err = schedule.ParseAtTouchTime(*touchTime, now, loc)
	} else {
		when, err = schedule.ParseAtTimespecInLocationWithLocale(timespec, now, loc, formatter)
	}
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}

	if when.Before(now) {
		return tool.UsageError(rc, cmd, "time %q is in the past", timespec)
	}

	cwd := rc.Dir
	if cwd == "" {
		return tool.UsageError(rc, cmd, "invocation working directory is required")
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

	id := strconv.FormatInt(now.UnixNano(), 36)
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
		Command:        []string{shell, "-c", cmdText},
		Dir:            cwd,
		Queue:          queueOrDefault(*queue),
		Env:            append([]string(nil), rc.Env...),
		EnvSet:         true,
		Enabled:        true,
		MailOutput:     true,
		MailCompletion: *mailCompletion,
		MailTo:         identity.Name,
		OwnerUID:       identity.UID,
		OwnerName:      identity.Name,
		BatchLoad:      queueOrDefault(*queue) == "b",
		CreatedAt:      now,
		NextRun:        when,
	}
	if rc.UmaskSet {
		j.Umask, j.UmaskSet = uint32(rc.Umask.Perm()), true
	} else if mask, ok := processUmask(); ok {
		j.Umask, j.UmaskSet = mask, true
	}
	if err := schedule.ValidateJobExecution(j); err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", cmd.Name, err)
		return 1
	}

	formatted, err := formatJobTime(rc, when)
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}
	if err := schedule.StoreFor(rc.Dir, rc.Env).SubmitJobWithConfirmation(j, func() error {
		_, err := fmt.Fprintf(rc.Err, "job %s at %s\n", id, formatted)
		return err
	}); err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot submit job: %v\n", cmd.Name, err)
		return 1
	}
	return 0
}

func listJobs(rc *tool.RunContext, ids []string, queue string) int {
	identity, err := schedule.AuthenticatedIdentity()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", cmd.Name, err)
		return 1
	}
	jobs, err := schedule.StoreFor(rc.Dir, rc.Env).LoadJobs()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot load schedule: %v\n", cmd.Name, err)
		return 1
	}
	for _, j := range jobs {
		if j.Kind != "at" || !j.Enabled || j.OwnerUID != identity.UID {
			continue
		}
		if queue != "" && queueOrDefault(j.Queue) != queue {
			continue
		}
		if len(ids) > 0 && !containsID(ids, j.ID, j.Name) {
			continue
		}
		formatted, err := formatJobTime(rc, j.NextRun)
		if err != nil {
			fmt.Fprintf(rc.Err, "%s: %v\n", cmd.Name, err)
			return 1
		}
		if _, err := fmt.Fprintf(rc.Out, "%s\t%s\n", j.ID, formatted); err != nil {
			return 1
		}
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

func formatJobTime(rc *tool.RunContext, t time.Time) (string, error) {
	loc, err := atLocation(rc.Getenv("TZ"))
	if err != nil {
		return "", err
	}
	formatter, err := posixlocale.ResolveTime(rc.Env)
	if err != nil {
		return "", err
	}
	return formatter.FormatAtJobTime(t.In(loc)), nil
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
	missing := false
	identity, identityErr := schedule.AuthenticatedIdentity()
	if identityErr != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", cmd.Name, identityErr)
		return 1
	}
	if err := schedule.StoreFor(rc.Dir, rc.Env).UpdateJobs(func(jobs []*schedule.Job) ([]*schedule.Job, error) {
		// Validate all operands before mutating the store. An unsuccessful
		// removal is an error, and must not leave a partially removed set whose
		// result depends on operand order.
		for _, id := range ids {
			found := false
			for _, job := range jobs {
				if job.Kind == "at" && job.OwnerUID == identity.UID && (job.ID == id || job.Name == id) {
					found = true
					break
				}
			}
			if !found {
				fmt.Fprintf(rc.Err, "%s: no job %q\n", cmd.Name, id)
				missing = true
			}
		}
		if missing {
			return jobs, nil
		}
		for _, id := range ids {
			for i := len(jobs) - 1; i >= 0; i-- {
				if jobs[i].Kind == "at" && jobs[i].OwnerUID == identity.UID && (jobs[i].ID == id || jobs[i].Name == id) {
					jobs = append(jobs[:i], jobs[i+1:]...)
				}
			}
		}
		return jobs, nil
	}); err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot save schedule: %v\n", cmd.Name, err)
		return 1
	}
	if missing {
		return 1
	}
	return 0
}
