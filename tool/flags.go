package tool

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// Version is what --version reports for every tool. The multicall
// binary may stamp it at build time.
var Version = "dev"

// NewFlags returns a FlagSet for one tool invocation with GNU-style
// behavior: combined short flags (-la), --long flags, flags permitted
// after operands (GNU permutation). --help and --version are defined
// here so every tool answers them.
//
// The contract for everything NOT defined on the returned set: Parse
// fails with exit code 2 and an error that names the flag and says the
// pure-Go implementation doesn't support it. Tools never silently
// ignore or approximate a flag.
func NewFlags(name string) *pflag.FlagSet {
	fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
	fs.SetInterspersed(true)
	fs.SortFlags = false
	fs.Bool("help", false, "display this help and exit")
	fs.Bool("version", false, "output version information and exit")
	// Suppress pflag's own error+usage printing; Parse below owns the
	// output so it lands on the invocation's stderr, not the process's.
	fs.SetOutput(discard{})
	return fs
}

// AliasHelpVersion rewrites uutils-style standalone -h and -V aliases
// to the universal long options. Only commands that do not use those
// short options for command-specific behavior should call this helper.
func AliasHelpVersion(args []string) []string {
	var out []string
	rest := false
	for _, arg := range args {
		if rest {
			out = append(out, arg)
			continue
		}
		if arg == "--" {
			rest = true
			out = append(out, arg)
			continue
		}
		switch arg {
		case "-h":
			out = append(out, "--help")
		case "-V":
			out = append(out, "--version")
		default:
			if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && len(arg) > 2 {
				kept := "-"
				for _, r := range arg[1:] {
					switch r {
					case 'h':
						out = append(out, "--help")
					case 'V':
						out = append(out, "--version")
					default:
						kept += string(r)
					}
				}
				if kept != "-" {
					out = append(out, kept)
				}
			} else {
				out = append(out, arg)
			}
		}
	}
	return out
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// Parse parses args against fs and handles the universal flags.
//
// Returns (operands, -1) when the tool should proceed. Otherwise the
// int is the exit code the tool must return immediately: 0 after
// --help/--version output, 2 after a usage error (unknown flag, bad
// value) with the contract message already printed to rc.Err.
func Parse(rc *RunContext, t *Tool, fs *pflag.FlagSet, args []string) ([]string, int) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			printHelp(rc, t, fs)
			return nil, 0
		}
		fmt.Fprintf(rc.Err, "%s: %v\n", t.Name, err)
		fmt.Fprintf(rc.Err, "%s: not every GNU flag is implemented in pure-Go coreutils — see '%s --help' for the supported subset\n", t.Name, t.Name)
		return nil, 2
	}
	if v, _ := fs.GetBool("help"); v {
		printHelp(rc, t, fs)
		return nil, 0
	}
	if v, _ := fs.GetBool("version"); v {
		fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", t.Name, Version)
		return nil, 0
	}
	return fs.Args(), -1
}

func printHelp(rc *RunContext, t *Tool, fs *pflag.FlagSet) {
	fmt.Fprintf(rc.Out, "Usage: %s\n", t.Usage)
	if t.Synopsis != "" {
		fmt.Fprintf(rc.Out, "%s\n", t.Synopsis)
	}
	u := fs.FlagUsages()
	for _, char := range []string{"1", "t", "S", "v", "g", "o", "C", "x", "p", "f", "U", "X", "q", "c", "u", "m", "Z"} {
		u = strings.ReplaceAll(u, ", --"+char+" ", "      ")
	}
	fmt.Fprintf(rc.Out, "\nOptions:\n%s", u)
	fmt.Fprintf(rc.Out, "\nImplements the documented GNU semantics for the flags above;\nanything else fails with a clear error (exit 2), never a silent guess.\n")
}

// UsageError prints a GNU-shaped usage diagnostic to rc.Err and
// returns 2 — for operand-level errors after flag parsing succeeded
// (wrong arity, invalid argument value, …).
func UsageError(rc *RunContext, t *Tool, format string, a ...any) int {
	fmt.Fprintf(rc.Err, "%s: %s\n", t.Name, fmt.Sprintf(format, a...))
	fmt.Fprintf(rc.Err, "Try '%s --help' for more information.\n", t.Name)
	return 2
}

// NotSupported prints the contract error for a flag/mode the pure-Go
// implementation deliberately does not cover, and returns 2.
func NotSupported(rc *RunContext, t *Tool, what string) int {
	fmt.Fprintf(rc.Err, "%s: %s is not supported by pure-Go coreutils — see '%s --help' for the supported subset\n", t.Name, what, t.Name)
	return 2
}
