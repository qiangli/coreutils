// Package renicecmd implements renice(1): change the nice value of running
// processes, process groups, or users.
//
// Two details are easy to get wrong and both are load-bearing.
//
// First, POSIX renice sets an ABSOLUTE nice value, while the historical BSD
// form and the -n option are usually read as a relative increment. POSIX.1-2008
// specifies `renice -n increment` as an increment applied to the current value,
// and `renice increment ID` (obsolescent) likewise. This implementation treats
// -n as an increment and says so, because silently choosing the other reading
// changes every result by the process's existing niceness.
//
// Second, a nice value is clamped to [-20, 19] rather than rejected: setting 40
// yields 19, which is what the kernel does, and reporting an error instead would
// make a portable script fail where the reference succeeds.
package renicecmd

import (
	"fmt"
	"os/user"
	"strconv"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "renice",
	Synopsis: "Set nice values of running processes.",
	Usage: `renice -n increment [-g|-p|-u] ID...
  renice increment [-g|-p|-u] ID...   (obsolescent)`,
}

func init() { cmd.Run = run; tool.Register(cmd) }

const (
	niceMin = -20
	niceMax = 19
)

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	incr := fs.StringP("increment", "n", "", "nice increment")
	byGroup := fs.BoolP("pgrp", "g", false, "operands are process group IDs")
	byPid := fs.BoolP("pid", "p", false, "operands are process IDs (default)")
	byUser := fs.BoolP("user", "u", false, "operands are user names or IDs")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	sel := 0
	for _, b := range []bool{*byGroup, *byPid, *byUser} {
		if b {
			sel++
		}
	}
	if sel > 1 {
		return tool.UsageError(rc, cmd, "-g, -p and -u are mutually exclusive")
	}

	inc := *incr
	if inc == "" {
		// Obsolescent form: the increment is the first operand.
		if len(operands) == 0 {
			return tool.UsageError(rc, cmd, "missing increment")
		}
		inc, operands = operands[0], operands[1:]
	}
	delta, err := strconv.Atoi(inc)
	if err != nil {
		return tool.UsageError(rc, cmd, "invalid increment %q", inc)
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing ID operand")
	}

	which := whichProcess
	switch {
	case *byGroup:
		which = whichPGroup
	case *byUser:
		which = whichUser
	}

	status := 0
	for _, op := range operands {
		id, err := resolveID(op, *byUser)
		if err != nil {
			fmt.Fprintf(rc.Err, "renice: %v\n", err)
			status = 1
			continue
		}
		old, err := getPriority(which, id)
		if err != nil {
			fmt.Fprintf(rc.Err, "renice: %s: %v\n", op, err)
			status = 1
			continue
		}
		want := clamp(old + delta)
		if err := setPriority(which, id, want); err != nil {
			fmt.Fprintf(rc.Err, "renice: %s: %v\n", op, err)
			status = 1
			continue
		}
		fmt.Fprintf(rc.Out, "%d: old priority %d, new priority %d\n", id, old, want)
	}
	return status
}

// clamp mirrors the kernel: an out-of-range request saturates rather than
// failing, so a script asking for 40 gets 19 exactly as the reference does.
func clamp(v int) int {
	if v < niceMin {
		return niceMin
	}
	if v > niceMax {
		return niceMax
	}
	return v
}

func resolveID(op string, asUser bool) (int, error) {
	if asUser {
		if u, err := user.Lookup(op); err == nil {
			return strconv.Atoi(u.Uid)
		}
	}
	n, err := strconv.ParseUint(op, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid ID %q", op)
	}
	return int(n), nil
}
