// Package chgrpcmd implements chgrp(1) per the GNU coreutils manual:
// change the group of each FILE to GROUP (name or numeric id), with -R.
//
// Unix only: Windows has no gid ownership model, so the Windows build
// fails loudly instead (see chgrp_windows.go).
//
// Portions adapted from https://github.com/guonaihong/coreutils chgrp/chgrp.go (Apache-2.0).
// Changes: rewired to tool framework; group lookup is name-then-numeric
// via os/user; verbose/reference flags dropped; recursion uses
// filepath.WalkDir with lchown for traversed symlinks.
package chgrpcmd

import (
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "chgrp",
	Synopsis: "Change the group of each FILE to GROUP.",
	Usage:    "chgrp [OPTION]... GROUP FILE...",
}

// Run is wired in init: a literal would create an initialization cycle.
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	recursive := fs.BoolP("recursive", "R", false, "operate on files and directories recursively")
	verbose := fs.BoolP("verbose", "v", false, "output a diagnostic for every file processed")
	changes := fs.BoolP("changes", "c", false, "like verbose but report only when a change is made")
	silent := fs.BoolP("silent", "f", false, "suppress most error messages")
	fs.Bool("quiet", false, "suppress most error messages")
	preserveRoot := fs.Bool("preserve-root", false, "fail to operate recursively on '/'")
	fs.Bool("no-preserve-root", false, "do not treat '/' specially (the default)")
	reference := fs.String("reference", "", "use RFILE's group rather than specifying a GROUP value")
	fromRef := fs.String("from", "", "change only if current owner:group matches FROM")
	fs.Bool("dereference", false, "affect the referent of each symbolic link (the default)")
	noDereference := fs.Bool("no-dereference", false, "affect symbolic links instead of their referents")
	noTraverse := fs.BoolP("P", "P", false, "never follow symbolic links (with -R)")
	cmdLineH := fs.BoolP("H", "H", false, "follow command-line symbolic links (with -R)")
	followL := fs.BoolP("L", "L", false, "follow every symbolic link encountered (with -R)")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if *reference != "" && *silent {
		*silent = false // -f doesn't suppress --reference errors
	}
	if *reference != "" {
		if len(operands) == 0 {
			return tool.UsageError(rc, cmd, "missing operand")
		}
		rfi, err := statFile(rc, *reference)
		if err != nil {
			return statusError(rc, "cannot stat reference file '%s': %v", *reference, err)
		}
		groupStr := rfi.gidStr()
		if groupStr == "" {
			return statusError(rc, "cannot determine group of reference file '%s'", *reference)
		}
		fromUid, fromGid, ferr := parseFromSpec(*fromRef)
		if ferr != nil {
			return statusError(rc, "%v", ferr)
		}
		return apply(rc, groupStr, operands[0:], *recursive, *verbose, *changes, *silent || isTrue(fs, "quiet"), *preserveRoot, *noDereference, *noTraverse, *cmdLineH, *followL || isTrue(fs, "dereference"), fromUid, fromGid)
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}
	if len(operands) == 1 {
		return tool.UsageError(rc, cmd, "missing operand after '%s'", operands[0])
	}
	fromUid, fromGid, ferr := parseFromSpec(*fromRef)
	if ferr != nil {
		return statusError(rc, "%v", ferr)
	}
	return apply(rc, operands[0], operands[1:], *recursive, *verbose, *changes, *silent || isTrue(fs, "quiet"), *preserveRoot, *noDereference, *noTraverse, *cmdLineH, *followL || isTrue(fs, "dereference"), fromUid, fromGid)
}

func isTrue(fs interface{ GetBool(string) (bool, error) }, name string) bool {
	v, err := fs.GetBool(name)
	return err == nil && v
}
