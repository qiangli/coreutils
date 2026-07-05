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
	cwd, _ := os.Getwd()

	j := &schedule.Job{
		ID:        id,
		Kind:      "at",
		Spec:      when.Format(time.RFC3339),
		Command:   strings.Fields(cmdText),
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
