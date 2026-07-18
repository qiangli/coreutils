// Package chowncmd implements chown(1) per the GNU coreutils manual:
// change file owner and group ([OWNER][:[GROUP]] by name or numeric
// ID), with -R.
//
// Unix only: Windows has no uid/gid ownership model, so the Windows
// build fails loudly instead (see chown_windows.go).
//
// Portions adapted from https://github.com/guonaihong/coreutils chown/chown.go (Apache-2.0).
// Changes: rewired to tool framework; OWNER[:GROUP] parsing rewritten
// around strings.Cut with name-then-numeric lookup via os/user;
// '.' separator and verbose/from/reference flags dropped; recursion
// uses filepath.WalkDir with lchown for traversed symlinks.
package chowncmd

import (
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "chown",
	Synopsis: "Change the owner and/or group of each FILE to OWNER and/or GROUP.",
	Usage:    "chown [OPTION]... [OWNER][:[GROUP]] FILE...",
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
	reference := fs.String("reference", "", "use RFILE's owner and group rather than specifying OWNER[:GROUP]")
	fromRef := fs.String("from", "", "change only if current owner:group matches FROM")
	fs.Bool("dereference", false, "affect the referent of each symbolic link (the default)")
	noDereference := fs.BoolP("no-dereference", "h", false, "affect symbolic links instead of their referents")
	noTraverse := fs.BoolP("P", "P", false, "never follow symbolic links (with -R)")
	cmdLineH := fs.BoolP("H", "H", false, "follow command-line symbolic links (with -R)")
	followL := fs.BoolP("L", "L", false, "follow every symbolic link encountered (with -R)")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	isSilent := *silent || isBool(fs, "quiet")
	isFollowOrDeref := *followL || isBool(fs, "dereference")

	if *reference != "" {
		if len(operands) == 0 {
			return tool.UsageError(rc, cmd, "missing operand")
		}
		rfi, err := statFile(rc, *reference)
		if err != nil {
			return statusError(rc, "cannot stat reference file '%s': %v", *reference, err)
		}
		ownerSpec := rfi.ownerStr()
		if ownerSpec == "" {
			return statusError(rc, "cannot determine owner of reference file '%s'", *reference)
		}
		if *fromRef != "" {
			fromUid, fromGid, ferr := parseSpec(*fromRef)
			if ferr != nil {
				return statusError(rc, "%v", ferr)
			}
			return apply(rc, ownerSpec, operands[0:], *recursive, *verbose, *changes, isSilent, *preserveRoot, *noDereference, *noTraverse, *cmdLineH, isFollowOrDeref, fromUid, fromGid)
		}
		return apply(rc, ownerSpec, operands[0:], *recursive, *verbose, *changes, isSilent, *preserveRoot, *noDereference, *noTraverse, *cmdLineH, isFollowOrDeref, -1, -1)
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}
	if len(operands) == 1 {
		return tool.UsageError(rc, cmd, "missing operand after '%s'", operands[0])
	}
	if *fromRef != "" {
		fromUid, fromGid, ferr := parseSpec(*fromRef)
		if ferr != nil {
			return statusError(rc, "%v", ferr)
		}
		return apply(rc, operands[0], operands[1:], *recursive, *verbose, *changes, isSilent, *preserveRoot, *noDereference, *noTraverse, *cmdLineH, isFollowOrDeref, fromUid, fromGid)
	}
	return apply(rc, operands[0], operands[1:], *recursive, *verbose, *changes, isSilent, *preserveRoot, *noDereference, *noTraverse, *cmdLineH, isFollowOrDeref, -1, -1)
}

func isBool(fs interface{ GetBool(string) (bool, error) }, name string) bool {
	v, err := fs.GetBool(name)
	return err == nil && v
}
