// Package atrmcmd implements atrm(1): remove pending `at` jobs from
// the bashy schedule store.
package atrmcmd

import (
	"fmt"

	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "atrm",
	Synopsis: "Remove pending at jobs.",
	Usage:    "atrm JOBID...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing job ID")
	}

	jobs, err := schedule.LoadJobs()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot load schedule: %v\n", cmd.Name, err)
		return 1
	}

	removed := 0
	for _, id := range operands {
		found := false
		for i := len(jobs) - 1; i >= 0; i-- {
			if jobs[i].ID == id || jobs[i].Name == id {
				jobs = append(jobs[:i], jobs[i+1:]...)
				found = true
				removed++
			}
		}
		if !found {
			fmt.Fprintf(rc.Err, "%s: no job %q\n", cmd.Name, id)
		}
	}

	if removed > 0 {
		if err := schedule.SaveJobs(jobs); err != nil {
			fmt.Fprintf(rc.Err, "%s: cannot save schedule: %v\n", cmd.Name, err)
			return 1
		}
	}
	return 0
}
