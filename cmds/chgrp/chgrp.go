// Package chgrpcmd implements chgrp(1) per POSIX.1-2016 and the GNU
// coreutils manual: change the group of each FILE to GROUP (name or
// numeric id), with -R and the -H/-L/-P/-h symbolic-link rules.
//
// Unix only: Windows has no gid ownership model, so the Windows build
// fails loudly instead (see chgrp_other.go).
//
// Portions adapted from https://github.com/guonaihong/coreutils chgrp/chgrp.go (Apache-2.0).
// Changes: rewired to tool framework; group lookup is name-then-numeric
// via os/user; recursion is the shared POSIX hierarchy walker in
// cmds/internal/hierwalk.
package chgrpcmd

import (
	"github.com/qiangli/coreutils/cmds/internal/hierwalk"
	"github.com/qiangli/coreutils/cmds/internal/linkopts"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "chgrp",
	Synopsis: "Change the group of each FILE to GROUP.",
	Usage: "chgrp [OPTION]... GROUP FILE...\n\n" +
		"  -H  with -R, follow a symbolic link named on the command line\n" +
		"  -L  with -R, follow every symbolic link to a directory\n" +
		"  -P  with -R, follow no symbolic link (the default)",
}

// traversalOptions are the POSIX symbolic-link traversal selectors
// chgrp recognizes; the last one given decides the walk.
// valueFlags are the long options taking a separate argument, which the
// pre-scan must not read as options.
var (
	traversalOptions = []string{"-H", "-L", "-P"}
	valueFlags       = []string{"reference", "from"}
)

// options is the parsed command line handed to the platform apply.
type options struct {
	files        []string
	recursive    bool
	verbose      bool
	changes      bool
	silent       bool
	preserveRoot bool
	// mode is the -H/-L/-P traversal mode, already reduced to
	// hierwalk.Physical when -R was not given (POSIX: the three are
	// ignored without -R).
	mode hierwalk.Mode
	// affectReferent reports that a change reaches a symbolic link's
	// referent rather than the link itself. POSIX -h clears it, and a
	// physical -R walk clears it for every link it reaches.
	affectReferent bool
	fromUid        int
	fromGid        int
	// hasRef reports that --reference supplied the ids. They are
	// carried as numbers rather than re-spelled as an operand: a
	// numeric id must never be looked up as a name, or a host holding
	// an account literally named "20" would silently redirect the
	// change.
	hasRef bool
	refUid int
	refGid int
}

// Run is wired in init: a literal would create an initialization cycle.
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	// -H, -L and -P have no long form and, per POSIX, override each
	// other by position; a flag set cannot express that, so they are
	// read off the argument vector before the framework parser runs.
	args, links := linkopts.Scan(args, traversalOptions, valueFlags)

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
	fs.BoolP("no-dereference", "h", false, "affect symbolic links instead of their referents")
	operands, code := tool.ParseRequireOrder(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	opts := options{
		recursive:      *recursive,
		verbose:        *verbose,
		changes:        *changes,
		silent:         *silent || isTrue(fs, "quiet"),
		preserveRoot:   *preserveRoot,
		mode:           links.Mode,
		affectReferent: links.Deref != linkopts.DerefLink,
		fromUid:        -1,
		fromGid:        -1,
	}
	if !opts.recursive {
		// POSIX: -H, -L and -P are ignored unless -R is specified.
		opts.mode = hierwalk.Physical
	} else if opts.mode == hierwalk.Physical {
		// A physical walk never reaches a link's referent, so asking for
		// one is a contradiction rather than something to approximate.
		if links.Deref == linkopts.DerefReferent {
			return statusError(rc, "-R --dereference requires either -H or -L")
		}
		opts.affectReferent = false
	}

	fromUid, fromGid, ferr := parseFromSpec(*fromRef)
	if ferr != nil {
		return statusError(rc, "%v", ferr)
	}
	opts.fromUid, opts.fromGid = fromUid, fromGid

	if fs.Changed("reference") {
		if len(operands) == 0 {
			return tool.UsageError(rc, cmd, "missing operand")
		}
		rfi, err := statFile(rc, *reference)
		if err != nil {
			return statusError(rc, "cannot stat reference file '%s': %v", *reference, err)
		}
		opts.hasRef = true
		opts.refUid, opts.refGid = rfi.ids()
		opts.files = operands
		return apply(rc, "", opts)
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}
	if len(operands) == 1 {
		return tool.UsageError(rc, cmd, "missing operand after '%s'", operands[0])
	}
	opts.files = operands[1:]
	return apply(rc, operands[0], opts)
}

func isTrue(fs interface{ GetBool(string) (bool, error) }, name string) bool {
	v, err := fs.GetBool(name)
	return err == nil && v
}
