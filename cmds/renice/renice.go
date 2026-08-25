// Package renicecmd implements renice as specified by POSIX.1-2008
// (Issue 7, 2016 Edition) XCU renice: request that the nice values of
// running processes, process groups, or users be changed.
//
// Grammar and semantics notes that are easy to get wrong, all load-bearing:
//
//   - Issue 7's only synopsis is `renice [-g|-p|-u] -n increment ID...`.
//     The historical `renice nice_value ID...` first-operand form was
//     removed from the standard in Issue 6, and it took an ABSOLUTE nice
//     value where -n takes an increment — accepting it under either
//     reading silently changes results, so it is refused with a usage
//     error that names -n.
//   - renice is exempt from Utility Syntax Guideline 9: options may appear
//     between operands. -g, -p, and -u each re-interpret the FOLLOWING
//     operands, while -n supplies one invocation-wide increment regardless
//     of its position.
//   - STDOUT is "Not used": success is silent. Diagnostics go to stderr
//     only, one per failing ID, and processing continues in operand
//     order (final exit status >0 on any failure).
//   - Nice-value bounds are implementation-defined and applied by the
//     kernel: POSIX setpriority() clamps an out-of-range value to the
//     exceeded limit rather than failing, so the computed target is
//     passed through, saturated only far outside any real limit to keep
//     the arithmetic overflow-safe.
package renicecmd

import (
	"errors"
	"fmt"
	"os/user"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "renice",
	Synopsis: "Set nice values of running processes.",
	Usage:    "renice [-g|-p|-u] -n increment ID...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

// scheduler is the invocation-owned seam over the host's getpriority()/
// setpriority(). run wires the real kernel scheduler; tests substitute a
// recording fake, so grammar, which-dispatch, ordering, and failure
// handling are all exercised without touching live processes.
type scheduler interface {
	get(which, id int) (int, error)
	set(which, id, prio int) error
	members(which, id int) ([]int, error)
}

// target is one ID operand together with the selector in effect at its
// position on the command line.
type target struct {
	which int
	op    string
}

// setpriority() accepts a C int. Keep the exact requested value throughout
// userspace whenever it is representable by that public interface; the
// kernel, not renice, applies the implementation-defined nice limits.
const (
	priorityArgMin = -1 << 31
	priorityArgMax = 1<<31 - 1
)

func run(rc *tool.RunContext, args []string) int {
	sched, hostErr := newHostScheduler()
	return runWith(rc, args, sched, hostErr)
}

// runWith parses first so --help/--version and usage diagnostics behave
// identically on every platform, then fails closed once — before any ID
// is touched — on hosts without POSIX nice values.
func runWith(rc *tool.RunContext, args []string, sched scheduler, hostErr error) int {
	delta, targets, code := parseRenice(rc, args)
	if code >= 0 {
		return code
	}
	if hostErr != nil {
		fmt.Fprintf(rc.Err, "renice: %v\n", hostErr)
		return 1
	}
	status := 0
	for _, tg := range targets {
		if err := adjust(rc, sched, delta, tg); err != nil {
			fmt.Fprintf(rc.Err, "renice: %v\n", err)
			status = 1
		}
	}
	return status
}

// parseRenice implements the Issue 7 grammar with the Guideline 9
// exemption: selector options are positional and re-interpret following
// operands, while the single -n value applies to every target. It returns
// the increment, the ordered targets, and an exit code (negative when the
// tool should proceed).
func parseRenice(rc *tool.RunContext, args []string) (int64, []target, int) {
	// Preserve the repository-wide -h/-V extension while the positional
	// parser below owns renice's POSIX Guideline 9 exception.
	args = tool.AliasHelpVersion(args)
	var (
		delta   int64
		haveN   bool
		sel     = whichProcess
		targets []target
	)
	fail := func(format string, a ...any) (int64, []target, int) {
		return 0, nil, tool.UsageError(rc, cmd, format, a...)
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			for _, op := range args[i+1:] {
				targets = append(targets, target{which: sel, op: op})
			}
			i = len(args)

		case a == "-" || !strings.HasPrefix(a, "-"):
			targets = append(targets, target{which: sel, op: a})

		case strings.HasPrefix(a, "--"):
			name, value, hasValue := strings.Cut(a[2:], "=")
			if name == "" {
				return fail("unrecognized option '%s'", a)
			}
			long, ambiguous := matchLongOption(name)
			if ambiguous {
				return fail("option '--%s' is ambiguous", name)
			}
			if long == "" {
				return fail("unrecognized option '--%s'", name)
			}
			switch long {
			case "help", "version":
				if hasValue {
					return fail("option '--%s' doesn't allow an argument", long)
				}
				if long == "help" {
					printReniceHelp(rc)
				} else {
					fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", cmd.Name, tool.Version)
				}
				return 0, nil, 0
			case "pgrp":
				if hasValue {
					return fail("option '--%s' doesn't allow an argument", long)
				}
				sel = whichPGroup
			case "pid":
				if hasValue {
					return fail("option '--%s' doesn't allow an argument", long)
				}
				sel = whichProcess
			case "user":
				if hasValue {
					return fail("option '--%s' doesn't allow an argument", long)
				}
				sel = whichUser
			case "increment":
				if haveN {
					return fail("duplicate -n increment")
				}
				if !hasValue {
					i++
					if i >= len(args) {
						return fail("option '--increment' requires an argument")
					}
					value = args[i]
				}
				n, err := parseIncrement(value)
				if err != nil {
					return fail("%v", err)
				}
				delta, haveN = n, true
			}

		default: // short-option cluster
			for pos := 1; pos < len(a); pos++ {
				switch a[pos] {
				case 'g':
					sel = whichPGroup
				case 'p':
					sel = whichProcess
				case 'u':
					sel = whichUser
				case 'n':
					if haveN {
						return fail("duplicate -n increment")
					}
					value := a[pos+1:]
					if value == "" {
						i++
						if i >= len(args) {
							return fail("option requires an argument -- 'n'")
						}
						value = args[i]
					}
					n, err := parseIncrement(value)
					if err != nil {
						return fail("%v", err)
					}
					delta, haveN = n, true
					pos = len(a)
				default:
					return fail("invalid option -- '%s'", string(a[pos]))
				}
			}
		}
	}
	if !haveN {
		return fail("-n increment is required (the obsolescent 'renice increment ID...' form is not supported)")
	}
	if len(targets) == 0 {
		return fail("missing ID operand")
	}
	return delta, targets, -1
}

func matchLongOption(name string) (match string, ambiguous bool) {
	for _, cand := range []string{"increment", "pgrp", "pid", "user", "help", "version"} {
		if cand == name {
			return cand, false
		}
		if strings.HasPrefix(cand, name) {
			if match != "" {
				return "", true
			}
			match = cand
		}
	}
	return match, false
}

// parseIncrement accepts a signed decimal integer representable by the
// implementation. Larger magnitudes cannot be added exactly and are usage
// errors rather than silently approximated values.
func parseIncrement(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err == nil {
		return n, nil
	}
	return 0, fmt.Errorf("invalid increment %q", s)
}

func adjust(rc *tool.RunContext, sched scheduler, delta int64, tg target) error {
	id, err := resolveID(tg)
	if err != nil {
		return err
	}
	if id == 0 && (tg.which == whichProcess || tg.which == whichPGroup) && !rc.DedicatedProcess {
		return fmt.Errorf("%s: ID 0 is not supported by an in-process embedding (it requires a dedicated command process)", tg.op)
	}

	// A relative adjustment must be computed from every process's current
	// value. Collective getpriority/setpriority calls collapse heterogeneous
	// groups to one value, so group and user selectors are expanded first.
	pids := []int{id}
	if tg.which != whichProcess {
		pids, err = sched.members(tg.which, id)
		if err != nil {
			return fmt.Errorf("%s: %v", tg.op, err)
		}
	}
	var failures []string
	for _, pid := range pids {
		old, getErr := sched.get(whichProcess, pid)
		if getErr != nil {
			failures = append(failures, fmt.Sprintf("process %d: %v", pid, getErr))
			continue
		}
		if setErr := sched.set(whichProcess, pid, requestedNice(old, delta)); setErr != nil {
			failures = append(failures, fmt.Sprintf("process %d: %v", pid, setErr))
		}
	}
	if len(failures) != 0 {
		return fmt.Errorf("%s: %s", tg.op, strings.Join(failures, "; "))
	}
	return nil
}

// requestedNice forms old+delta without inventing a nice-value bound. It
// saturates only at the C-int boundary of setpriority(); the kernel applies
// the host's actual, implementation-defined nice limit.
func requestedNice(old int, delta int64) int {
	if delta > priorityArgMax-int64(old) {
		return priorityArgMax
	}
	if delta < priorityArgMin-int64(old) {
		return priorityArgMin
	}
	return old + int(delta)
}

// resolveID interprets one operand under its selector. For -u, POSIX
// orders the lookup: an existing user NAME wins, and only otherwise is an
// unsigned decimal operand used as a numeric user ID. For -g/-p the
// operand must be an unsigned decimal integer. Process IDs use signed
// pid_t range; user IDs may use all 32 bits where Go's host int can carry
// them, and fail clearly on 32-bit hosts where the syscall wrapper cannot.
func resolveID(tg target) (int, error) {
	if tg.which == whichUser {
		u, lookupErr := user.Lookup(tg.op)
		if lookupErr == nil {
			return parseUID(tg.op, u.Uid)
		}
		var unknown user.UnknownUserError
		if !errors.As(lookupErr, &unknown) {
			return 0, fmt.Errorf("%s: user lookup failed: %v", tg.op, lookupErr)
		}
		n, err := strconv.ParseUint(tg.op, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("%s: no such user", tg.op)
		}
		return uidToHostInt(tg.op, n)
	}
	n, err := strconv.ParseUint(tg.op, 10, 31)
	if err != nil {
		return 0, fmt.Errorf("invalid ID %q", tg.op)
	}
	return int(n), nil
}

func intMax() int { return int(^uint(0) >> 1) }

func parseUID(operand, uid string) (int, error) {
	n, err := strconv.ParseUint(uid, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid user ID %q", operand, uid)
	}
	return uidToHostInt(operand, n)
}

func uidToHostInt(operand string, uid uint64) (int, error) {
	if uid > uint64(intMax()) {
		return 0, fmt.Errorf("%s: user ID %d is not supported on this %d-bit host", operand, uid, strconv.IntSize)
	}
	return int(uid), nil
}

func printReniceHelp(rc *tool.RunContext) {
	fmt.Fprintf(rc.Out, "Usage: %s\n%s\n\n", cmd.Usage, cmd.Synopsis)
	fmt.Fprint(rc.Out, `Options:
  -n, --increment increment
                 add increment to the nice value of each ID (negative
                 increments usually require appropriate privileges)
  -g, --pgrp     interpret the following operands as process group IDs
  -p, --pid      interpret the following operands as process IDs (default)
  -u, --user     interpret the following operands as user names or user IDs
  -h, --help     display this help and exit
  -V, --version  output version information and exit

-g/-p/-u may appear between operands and re-interpret the operands that
follow. -n is one invocation-wide increment and may also be interspersed;
it applies to every ID regardless of its position.
Success is silent (POSIX: standard output not used). Exit status: 0 on
success, 1 if any ID could not be adjusted, 2 on a usage error.
`)
}
