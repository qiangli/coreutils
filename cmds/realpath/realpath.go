// Package realpathcmd implements realpath(1) per the GNU coreutils
// manual: print the resolved absolute file name of each FILE. By
// default all components are resolved physically (symlinks expanded
// as encountered) and all but the last component must exist.
//
// Implemented flags: -e/--canonicalize-existing,
// -m/--canonicalize-missing, -s/--strip/--no-symlinks,
// --relative-to=DIR. Anything else GNU defines (-L, -P, -q, -z,
// --relative-base) is not implemented and fails with the contract
// error naming the flag.
package realpathcmd

import (
	"fmt"
	"os"
	"path/filepath"

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
	fs.BoolP("canonicalize-existing", "e", false, "all components of the path must exist")
	fs.BoolP("canonicalize-missing", "m", false, "no path components need exist or be a directory")
	strip := fs.BoolP("strip", "s", false, "don't expand symlinks")
	noSym := fs.Bool("no-symlinks", false, "don't expand symlinks (same as --strip)")
	relTo := fs.String("relative-to", "", "print the resolved path relative to DIR")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}

	mode := canonAllButLast
	switch lastEM(args) { // GNU: the last of -e/-m wins
	case 'e':
		mode = canonExisting
	case 'm':
		mode = canonMissing
	}
	lexical := *strip || *noSym

	resolve := func(operand string) (string, error) {
		abs, err := absOperand(rc, operand)
		if err != nil {
			return "", err
		}
		if lexical {
			res := filepath.Clean(abs)
			return res, stripCheck(res, mode)
		}
		return canonicalize(abs, mode)
	}

	relBase := ""
	if fs.Changed("relative-to") {
		// GNU canonicalizes DIR with the same mode; failure is fatal.
		base, err := resolve(*relTo)
		if err != nil {
			fmt.Fprintf(rc.Err, "realpath: %s: %s\n", *relTo, pathErrText(err))
			return 1
		}
		relBase = base
	}

	status := 0
	for _, op := range operands {
		res, err := resolve(op)
		if err != nil {
			fmt.Fprintf(rc.Err, "realpath: %s: %s\n", op, pathErrText(err))
			status = 1
			continue
		}
		if relBase != "" {
			if rel, rerr := filepath.Rel(relBase, res); rerr == nil {
				res = rel
			}
			// unrelatable (e.g. different Windows volume): GNU
			// falls back to the absolute name.
		}
		fmt.Fprintln(rc.Out, res)
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

// lastEM scans args for the last occurrence of -e/-m (or long forms):
// pflag reports values but not order, and in GNU realpath the
// later of the two modes wins.
func lastEM(args []string) byte {
	var mode byte
	for _, a := range args {
		switch {
		case a == "--":
			return mode
		case a == "--canonicalize-existing":
			mode = 'e'
		case a == "--canonicalize-missing":
			mode = 'm'
		case len(a) > 1 && a[0] == '-' && a[1] != '-':
			for _, c := range a[1:] {
				switch c {
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
