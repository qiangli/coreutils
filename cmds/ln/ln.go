// Package lncmd implements ln(1) per the GNU coreutils manual: make
// hard or symbolic links between files.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/ln (BSD-3-Clause).
// Changes: rewired to tool framework; reduced to the -s/-f/-v subset;
// operand-form detection kept, RunContext-relative path resolution
// added (symlink TARGET text is stored verbatim, as GNU does).
package lncmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "ln",
	Synopsis: "Make links between files (hard by default, symbolic with --symbolic).",
	Usage: "ln [OPTION]... TARGET LINK_NAME\n" +
		"   or: ln [OPTION]... TARGET\n" +
		"   or: ln [OPTION]... TARGET... DIRECTORY",
}

// Run is wired in init: a literal would create an initialization cycle.
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	symbolic := fs.BoolP("symbolic", "s", false, "make symbolic links instead of hard links")
	force := fs.BoolP("force", "f", false, "remove existing destination files")
	verbose := fs.BoolP("verbose", "v", false, "print name of each linked file")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing file operand")
	}

	// Decide which GNU form applies: TARGET, TARGET LINK_NAME, or
	// TARGET... DIRECTORY (last operand is an existing directory).
	targets := operands
	linkName := ""
	dir := ""
	if len(operands) > 1 {
		last := operands[len(operands)-1]
		if fi, err := os.Stat(rc.Path(last)); err == nil && fi.IsDir() {
			dir = last
			targets = operands[:len(operands)-1]
		} else if len(operands) == 2 {
			targets = operands[:1]
			linkName = last
		} else {
			fmt.Fprintf(rc.Err, "ln: target '%s' is not a directory\n", last)
			return 1
		}
	}

	exit := 0
	for _, target := range targets {
		dest := linkName
		if dest == "" {
			dest = filepath.Base(target)
		}
		if dir != "" {
			dest = filepath.Join(dir, filepath.Base(target))
		}
		destPath := rc.Path(dest)
		if *force {
			if _, err := os.Lstat(destPath); err == nil {
				if err := os.Remove(destPath); err != nil {
					fmt.Fprintf(rc.Err, "ln: cannot remove '%s': %v\n", dest, reason(err))
					exit = 1
					continue
				}
			}
		}
		var err error
		if *symbolic {
			// The symlink stores TARGET exactly as written — GNU never
			// resolves it against the working directory.
			err = os.Symlink(target, destPath)
		} else {
			err = os.Link(rc.Path(target), destPath)
		}
		if err != nil {
			kind := "hard"
			if *symbolic {
				kind = "symbolic"
			}
			fmt.Fprintf(rc.Err, "ln: failed to create %s link '%s': %v\n", kind, dest, reason(err))
			exit = 1
			continue
		}
		if *verbose {
			arrow := "=>" // GNU prints => for hard links, -> for symlinks
			if *symbolic {
				arrow = "->"
			}
			fmt.Fprintf(rc.Out, "'%s' %s '%s'\n", dest, arrow, target)
		}
	}
	return exit
}

// reason unwraps os wrapper errors so diagnostics read like GNU's.
func reason(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.Err
	}
	return err
}
