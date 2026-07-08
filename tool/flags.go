package tool

import (
	"errors"
	"fmt"
	"sort"
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

const (
	helpAliasFlag    = "help-short"
	versionAliasFlag = "version-short"
)

// AddHelpVersionAliases registers uutils-style -h and -V aliases for
// the universal --help/--version flags, but only when those shorthands
// are still unused by command-specific options. Call it after a command
// registers its own flags.
func AddHelpVersionAliases(fs *pflag.FlagSet) {
	addUniversalAlias(fs, "help", "h", helpAliasFlag)
	addUniversalAlias(fs, "version", "V", versionAliasFlag)
}

func addUniversalAlias(fs *pflag.FlagSet, long, short, aliasName string) {
	canonical := fs.Lookup(long)
	if canonical == nil || fs.ShorthandLookup(short) != nil {
		return
	}
	_ = fs.BoolP(aliasName, short, false, canonical.Usage)
	fs.Lookup(aliasName).Hidden = true

	// pflag can only map a shorthand to one flag. The hidden alias owns
	// parsing; the canonical flag carries the shorthand for help output.
	canonical.Shorthand = short
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
	AddHelpVersionAliases(fs)
	var code int
	args, code = expandLongOptionPrefixes(rc, t, fs, args)
	if code != -1 {
		return nil, code
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			printHelp(rc, t, fs)
			return nil, 0
		}
		fmt.Fprintf(rc.Err, "%s: %v\n", t.Name, err)
		fmt.Fprintf(rc.Err, "%s: not every GNU flag is implemented in pure-Go coreutils — see '%s --help' for the supported subset\n", t.Name, t.Name)
		return nil, 2
	}
	if flagBool(fs, "help") || flagBool(fs, helpAliasFlag) {
		printHelp(rc, t, fs)
		return nil, 0
	}
	if flagBool(fs, "version") || flagBool(fs, versionAliasFlag) {
		fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", t.Name, Version)
		return nil, 0
	}
	return fs.Args(), -1
}

func expandLongOptionPrefixes(rc *RunContext, t *Tool, fs *pflag.FlagSet, args []string) ([]string, int) {
	out := append([]string(nil), args...)
	for i := 0; i < len(out); i++ {
		arg := out[i]
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "--") && len(arg) > 2 {
			name, suffix, hasValue := strings.Cut(arg[2:], "=")
			flag := fs.Lookup(name)
			if flag == nil {
				var matches []string
				fs.VisitAll(func(f *pflag.Flag) {
					// The framework's hidden -h/-V alias flags are parser plumbing, not
					// GNU-style long options. Let exact uses parse as before, but do not
					// let these internal names create prefix matches or ambiguities.
					if f.Name == helpAliasFlag || f.Name == versionAliasFlag {
						return
					}
					if strings.HasPrefix(f.Name, name) {
						matches = append(matches, f.Name)
					}
				})
				sort.Strings(matches)
				switch len(matches) {
				case 0:
					continue
				case 1:
					flag = fs.Lookup(matches[0])
					if hasValue {
						out[i] = "--" + matches[0] + "=" + suffix
					} else {
						out[i] = "--" + matches[0]
					}
				default:
					fmt.Fprintf(rc.Err, "%s: option '--%s' is ambiguous; possibilities:", t.Name, name)
					for _, match := range matches {
						fmt.Fprintf(rc.Err, " '--%s'", match)
					}
					fmt.Fprintln(rc.Err)
					return nil, 2
				}
			}
			if !hasValue && flagTakesValue(flag) {
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			for pos := 1; pos < len(arg); pos++ {
				flag := fs.ShorthandLookup(arg[pos : pos+1])
				if flag == nil {
					continue
				}
				if flagTakesValue(flag) {
					if pos == len(arg)-1 {
						i++
					}
					break
				}
			}
		}
	}
	return out, -1
}

func flagTakesValue(flag *pflag.Flag) bool {
	return flag != nil && flag.NoOptDefVal == "" && flag.Value.Type() != "bool"
}

func flagBool(fs *pflag.FlagSet, name string) bool {
	v, err := fs.GetBool(name)
	return err == nil && v
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
