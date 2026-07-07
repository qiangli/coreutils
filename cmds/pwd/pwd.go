// Package pwdcmd implements pwd(1) per the GNU coreutils manual:
// print the name of the current working directory.
//
// Framework note: the "current working directory" is the invocation's
// rc.Dir — never the process cwd (the embedding shell owns its own
// cwd). The default and -L therefore print rc.Dir itself: rc.Dir is
// the authoritative logical directory name, playing the role the
// validated $PWD plays in standalone GNU pwd. -P resolves every
// symlink in rc.Dir. When both -L and -P are given, the last one
// takes precedence (GNU rule).
package pwdcmd

import (
	"fmt"
	"path/filepath"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "pwd",
	Synopsis: "Print the full filename of the current working directory.",
	Usage:    "pwd [OPTION]...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	fs.BoolP("logical", "L", false, "print the logical working directory, even with symlinks")
	fs.BoolP("physical", "P", false, "print the physical directory, with all symlinks resolved")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) > 0 {
		// GNU pwd warns and proceeds; the operands have no meaning.
		fmt.Fprintln(rc.Err, "pwd: ignoring non-option arguments")
	}
	if rc.Dir == "" {
		fmt.Fprintln(rc.Err, "pwd: cannot determine current directory")
		return 1
	}
	if lastLP(args) == 'P' {
		resolved, err := filepath.EvalSymlinks(rc.Dir)
		if err != nil {
			fmt.Fprintf(rc.Err, "pwd: %v\n", err)
			return 1
		}
		fmt.Fprintln(rc.Out, resolved)
		return 0
	}
	fmt.Fprintln(rc.Out, rc.Dir)
	return 0
}

// lastLP scans args for the last occurrence of -L/-P (or their long
// forms) — pflag reports values but not order, and GNU specifies that
// the last of the two wins. Default is 'L'.
func lastLP(args []string) byte {
	mode := byte('L')
	for _, a := range args {
		switch {
		case a == "--":
			return mode
		case a == "--logical":
			mode = 'L'
		case a == "--physical":
			mode = 'P'
		case len(a) > 1 && a[0] == '-' && a[1] != '-':
			for _, c := range a[1:] {
				switch c {
				case 'L':
					mode = 'L'
				case 'P':
					mode = 'P'
				}
			}
		}
	}
	return mode
}
