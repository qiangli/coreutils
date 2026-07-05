// Package atqcmd implements atq(1): list pending one-shot `at` jobs
// from the bashy schedule store.
package atqcmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "atq",
	Synopsis: "List pending at jobs.",
	Usage:    "atq",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	_, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

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
