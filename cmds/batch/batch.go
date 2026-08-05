// Package batchcmd implements batch(1): schedule a command to run
// when system load permits — in our implementation, this is an alias
// for "at now" (immediate one-shot).
package batchcmd

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
	Name:     "batch",
	Synopsis: "Schedule a command to run as soon as possible.",
	Usage:    "batch [-f FILE]\n   or: batch",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	filename := fs.StringP("file", "f", "", "read the job from FILE rather than standard input")

	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	_ = operands

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

	now := time.Now()
	when := now.Add(1 * time.Second)
	id := strconv.FormatInt(now.UnixNano(), 36)
	shell := rc.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}
	cwd := rc.Dir
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	j := &schedule.Job{
		ID:        id,
		Kind:      "at",
		Spec:      when.Format(time.RFC3339),
		Command:   []string{shell, "-c", cmdText},
		Dir:       cwd,
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
	// POSIX at/batch write the job confirmation to stderr (never stdout)
	// in the traditional "Mon Jan _2 15:04:05 2006" format — the same shape
	// cmds/at uses.
	fmt.Fprintf(rc.Err, "job %s at %s\n", id, formatJobTime(when))
	return 0
}

// formatJobTime renders the time in the traditional at/batch format,
// matching cmds/at.formatJobTime.
func formatJobTime(t time.Time) string {
	return t.Format("Mon Jan _2 15:04:05 2006")
}
