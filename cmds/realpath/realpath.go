// Package realpathcmd implements realpath(1) per the GNU coreutils
// manual: print the resolved absolute file name of each FILE. By
// default all components are resolved physically (symlinks expanded
// as encountered) and all but the last component must exist.
//
// Implemented flags: -E/--canonicalize, -e/--canonicalize-existing,
// -m/--canonicalize-missing, -P/--physical, -s/--strip/--no-symlinks,
// -L/--logical, -q/--quiet, -z/--zero, --relative-to=DIR, and
// --relative-base=DIR.
package realpathcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "realpath",
	Synopsis: "Print the resolved absolute file name; all but the last component must exist.",
	Usage:    "realpath [OPTION]... FILE...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	fs.BoolP("canonicalize", "E", false, "all but the last component must exist")
	fs.BoolP("canonicalize-existing", "e", false, "all components of the path must exist")
	fs.BoolP("canonicalize-missing", "m", false, "no path components need exist or be a directory")
	fs.BoolP("logical", "L", false, "resolve '..' components before symlinks")
	fs.BoolP("physical", "P", false, "resolve symlinks as encountered")
	quiet := fs.BoolP("quiet", "q", false, "suppress most error messages")
	fs.BoolP("strip", "s", false, "don't expand symlinks")
	fs.Bool("no-symlinks", false, "don't expand symlinks (same as --strip)")
	zero := fs.BoolP("zero", "z", false, "end each output line with NUL, not newline")
	relTo := fs.String("relative-to", "", "print the resolved path relative to DIR")
	relBaseOpt := fs.String("relative-base", "", "print absolute paths unless paths are below DIR")
	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}

	mode := canonAllButLast
	switch lastEEM(args) { // GNU: the last of -E/-e/-m wins
	case 'E':
		mode = canonAllButLast
	case 'e':
		mode = canonExisting
	case 'm':
		mode = canonMissing
	}
	resolveMode := lastLPS(args)
	lexical := resolveMode == 's'
	logical := resolveMode == 'L'
	delim := "\n"
	if *zero {
		delim = "\x00"
	}

	resolve := func(operand string) (string, error) {
		abs, err := absOperand(rc, operand)
		if err != nil {
			return "", err
		}
		if logical {
			abs = filepath.Clean(abs)
		}
		if lexical {
			res := filepath.Clean(abs)
			return res, stripCheck(res, mode)
		}
		return canonicalize(abs, mode)
	}

	relToBase := ""
	if fs.Changed("relative-to") {
		// GNU canonicalizes DIR with the same mode; failure is fatal.
		base, err := resolve(*relTo)
		if err != nil {
			if !*quiet {
				fmt.Fprintf(rc.Err, "realpath: %s: %s\n", *relTo, pathErrText(err))
			}
			return 1
		}
		relToBase = base
	}
	relBase := ""
	if fs.Changed("relative-base") {
		base, err := resolve(*relBaseOpt)
		if err != nil {
			if !*quiet {
				fmt.Fprintf(rc.Err, "realpath: %s: %s\n", *relBaseOpt, pathErrText(err))
			}
			return 1
		}
		relBase = base
	}
	if relBase != "" && relToBase != "" && !isWithin(relToBase, relBase) {
		relBase = ""
		relToBase = ""
	}

	status := 0
	for _, op := range operands {
		res, err := resolve(op)
		if err != nil {
			if !*quiet {
				fmt.Fprintf(rc.Err, "realpath: %s: %s\n", op, pathErrText(err))
			}
			status = 1
			continue
		}
		if relBase != "" {
			if isWithin(res, relBase) {
				base := relBase
				if relToBase != "" {
					base = relToBase
				}
				if rel, rerr := filepath.Rel(base, res); rerr == nil {
					res = rel
				}
			}
		} else if relToBase != "" {
			if rel, rerr := filepath.Rel(relToBase, res); rerr == nil {
				res = rel
			}
		}
		// unrelatable (e.g. different Windows volume): GNU falls
		// back to the absolute name.
		fmt.Fprint(rc.Out, res, delim)
	}
	return status
}

// stripCheck applies the per-mode existence requirement that gnulib
// keeps even when symlink expansion is off (-s): -e requires the
// whole path, the default requires the parent directory, -m nothing.
func stripCheck(res string, mode int) error {
	switch mode {
	case canonMissing:
		return nil
	case canonExisting:
		_, err := os.Lstat(res)
		return err
	default:
		if _, err := os.Lstat(res); err == nil {
			return nil
		}
		fi, err := os.Stat(filepath.Dir(res))
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			return errNotDir
		}
		return nil
	}
}

// lastEEM scans args for the last occurrence of -E/-e/-m (or long forms):
// pflag reports values but not order, and in GNU realpath the later
// mode wins.
func lastEEM(args []string) byte {
	var mode byte
	for _, a := range args {
		switch {
		case a == "--":
			return mode
		case a == "--canonicalize":
			mode = 'E'
		case a == "--canonicalize-existing":
			mode = 'e'
		case a == "--canonicalize-missing":
			mode = 'm'
		case len(a) > 1 && a[0] == '-' && a[1] != '-':
			for _, c := range a[1:] {
				switch c {
				case 'E':
					mode = 'E'
				case 'e':
					mode = 'e'
				case 'm':
					mode = 'm'
				}
			}
		}
	}
	return mode
}

// lastLPS scans args for the last symlink-resolution selector. GNU
// realpath lets -L, -P, and -s/--strip/--no-symlinks override each
// other by argument order.
func lastLPS(args []string) byte {
	var mode byte
	for _, a := range args {
		switch {
		case a == "--":
			return mode
		case a == "--logical":
			mode = 'L'
		case a == "--physical":
			mode = 'P'
		case a == "--strip" || a == "--no-symlinks":
			mode = 's'
		case len(a) > 1 && a[0] == '-' && a[1] != '-':
			for _, c := range a[1:] {
				switch c {
				case 'L', 'P':
					mode = byte(c)
				case 's':
					mode = 's'
				}
			}
		}
	}
	return mode
}

func isWithin(path, base string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
