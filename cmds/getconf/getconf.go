// Package getconfcmd implements getconf(1), one of the POSIX-required commands
// the Go multicall previously did not supply.
//
// Why Go rather than a managed external: getconf is owned by glibc (libc-bin on
// Debian/Ubuntu). There is no standalone getconf to download — provisioning it
// through pkg/binmgr would mean shipping a C library to obtain one small
// utility. The same is true of tput/tabs (ncurses) and logger/renice/mesg/write
// (util-linux/bsdutils). For this family a Go implementation is not a fallback,
// it is the only practical path, and it has the side benefit of working on
// every platform bashy targets without a toolchain or a configure step.
//
// Scope: the POSIX Utility Syntax for getconf is
//
//	getconf [-v specification] system_var
//	getconf [-v specification] path_var pathname
//	getconf -a                          (report all, a common extension)
//
// A variable that is defined but has no limit prints "undefined"; an unknown
// variable is a usage error. Both behaviours are load-bearing for conformance:
// a caller distinguishes "no limit" from "no such variable".
package getconfcmd

import (
	"fmt"
	"sort"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "getconf",
	Synopsis: "Print the value of a POSIX configuration variable.",
	Usage: `getconf [-v specification] system_var
  getconf [-v specification] path_var pathname
  getconf -a`,
}

func init() { cmd.Run = run; tool.Register(cmd) }

// undefined is what POSIX requires for a variable that exists but has no limit,
// as distinct from one that does not exist at all.
const undefined = "undefined"

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	all := fs.BoolP("all", "a", false, "print all known configuration variables")
	spec := fs.StringP("specification", "v", "", "select a programming environment")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	// -v selects a programming environment. Only the environment this build
	// actually targets is honoured; silently accepting another specification
	// would report this environment's values under that name.
	if fs.Changed("specification") && !knownSpecification(*spec) {
		fmt.Fprintf(rc.Err, "getconf: unsupported specification: %s\n", *spec)
		return 1
	}

	if *all {
		if len(operands) > 0 {
			return tool.UsageError(rc, cmd, "-a takes no operands")
		}
		names := make([]string, 0, len(sysVars)+len(confstrVars))
		for n := range sysVars {
			names = append(names, n)
		}
		names = append(names, confstrVars...)
		sort.Strings(names)
		for _, n := range names {
			v, ok := systemValue(n)
			if !ok {
				continue
			}
			if _, err := fmt.Fprintf(rc.Out, "%-32s %s\n", n, v); err != nil {
				fmt.Fprintf(rc.Err, "getconf: write error: %v\n", err)
				return 1
			}
		}
		return 0
	}

	switch len(operands) {
	case 1:
		name := operands[0]
		v, ok := systemValue(name)
		if !ok {
			return tool.UsageError(rc, cmd, "unknown variable %q", name)
		}
		if _, err := fmt.Fprintln(rc.Out, v); err != nil {
			fmt.Fprintf(rc.Err, "getconf: write error: %v\n", err)
			return 1
		}
		return 0
	case 2:
		name, path := operands[0], operands[1]
		v, ok, err := pathValue(rc, name, path)
		if !ok {
			return tool.UsageError(rc, cmd, "unknown variable %q", name)
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "getconf: %s %s: %v\n", name, path, err)
			return 1
		}
		if _, err := fmt.Fprintln(rc.Out, v); err != nil {
			fmt.Fprintf(rc.Err, "getconf: write error: %v\n", err)
			return 1
		}
		return 0
	case 0:
		return tool.UsageError(rc, cmd, "missing operand")
	default:
		return tool.UsageError(rc, cmd, "extra operand %q", operands[2])
	}
}
