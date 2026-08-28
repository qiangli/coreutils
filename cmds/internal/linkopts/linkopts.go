// Package linkopts resolves the symbolic-link option group shared by
// chown and chgrp: the traversal selectors -H, -L and -P, and the
// -h/--dereference pair that decides whether a change lands on a link
// or on its referent.
//
// POSIX.1 requires that -H, -L and -P "override each other and the
// command's actions are determined by the last one specified", and GNU
// applies the same rule to --dereference/--no-dereference. A flag set
// records only whether an option was seen, never in what order, so the
// group has to be read off the argument vector before the framework
// parser runs — the same treatment the repository gives every GNU flag
// with no long form.
//
// Scan strips only the traversal options, which GNU spells with no long
// form. -h and --dereference stay in the returned arguments so the
// command still declares them and --help still lists them; Scan reports
// only which of the two came last.
package linkopts

import (
	"strings"

	"github.com/qiangli/coreutils/cmds/internal/hierwalk"
)

// Deref is the tri-state of the -h/--dereference pair: unset, or
// explicitly selected by the last occurrence.
type Deref int

const (
	// DerefUnset means neither option was given.
	DerefUnset Deref = iota
	// DerefLink means -h/--no-dereference came last: act on the link.
	DerefLink
	// DerefReferent means --dereference came last: act on the referent.
	DerefReferent
)

// Resolved is the outcome of the option group.
type Resolved struct {
	// Mode is the traversal mode selected by the last of -H/-L/-P.
	// It is hierwalk.Physical when none was given. POSIX leaves that
	// default unspecified; physical traversal is this implementation's
	// deliberate fail-closed choice.
	Mode hierwalk.Mode
	// ModeSet reports that one of -H/-L/-P was actually given.
	ModeSet bool
	// Deref is the last of -h/--no-dereference/--dereference.
	Deref Deref
}

// Scan returns args with the traversal options removed, plus the
// resolved option group. traversal names the traversal options the
// command recognizes, spelled as the command documents them ("-H",
// "-L", "-P"); one the command does not name is left in place for the
// framework parser to reject. valueFlags names the command's long
// options that consume a separate argument, so an option value that
// looks like a short-option cluster is never rewritten.
//
// Scan deliberately does not diagnose anything: an argument it cannot
// make sense of is passed through untouched for the framework parser,
// which owns every usage diagnostic.
func Scan(args []string, traversal, valueFlags []string) ([]string, Resolved) {
	modes := traversalModes(traversal)
	var res Resolved
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		if strings.HasPrefix(arg, "--") && len(arg) > 2 {
			name, _, hasValue := strings.Cut(arg[2:], "=")
			switch {
			case matches(name, "dereference"):
				res.Deref = DerefReferent
			case matches(name, "no-dereference"):
				res.Deref = DerefLink
			}
			rest = append(rest, arg)
			// A value given as a separate argument is never an option.
			if !hasValue && takesValue(name, valueFlags) && i+1 < len(args) {
				i++
				rest = append(rest, args[i])
			}
			continue
		}
		if len(arg) > 1 && arg[0] == '-' && arg[1] != '-' {
			// Every short option chown and chgrp define is a boolean, so
			// a cluster can be scanned to its end without the ambiguity
			// an option value would introduce.
			kept := make([]byte, 0, len(arg))
			for j := 1; j < len(arg); j++ {
				if mode, ok := modes[arg[j]]; ok {
					res.Mode, res.ModeSet = mode, true
					continue
				}
				if arg[j] == 'h' {
					res.Deref = DerefLink
				}
				kept = append(kept, arg[j])
			}
			if len(kept) > 0 {
				rest = append(rest, "-"+string(kept))
			}
			continue
		}
		// POSIX Utility Syntax Guideline 9: the owner/group operand ends
		// option recognition. Preserve it and every following argument
		// literally, including names such as -L and --dereference.
		rest = append(rest, args[i:]...)
		break
	}
	return rest, res
}

// traversalModes maps each traversal option the command recognizes to
// the mode it selects. An option the command does not name is not in
// the map and so is left for the framework parser.
func traversalModes(traversal []string) map[byte]hierwalk.Mode {
	known := map[string]hierwalk.Mode{
		"-H": hierwalk.CommandLine,
		"-L": hierwalk.Logical,
		"-P": hierwalk.Physical,
	}
	modes := make(map[byte]hierwalk.Mode, len(traversal))
	for _, option := range traversal {
		if mode, ok := known[option]; ok {
			modes[option[1]] = mode
		}
	}
	return modes
}

// matches reports whether name is long option full or an unambiguous
// abbreviation of it. Ambiguity between the command's own options stays
// the framework parser's error to report; recognizing a prefix here only
// records ordering, and ordering is discarded when that parse fails.
func matches(name, full string) bool {
	return name != "" && strings.HasPrefix(full, name)
}

func takesValue(name string, valueFlags []string) bool {
	for _, flag := range valueFlags {
		if matches(name, flag) {
			return true
		}
	}
	return false
}
