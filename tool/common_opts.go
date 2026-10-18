package tool

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// nativeNullFlags are the documented upstream spellings --null may map to,
// in preference order. --null itself is never invented for a tool.
var nativeNullFlags = []string{"zero", "zero-terminated", "null-data"}

// commonOpts holds the long-only agent options recognized at the parse
// chokepoint outside the cert profile. A tool's own flag of the same name
// always wins: the scan only consumes a name the FlagSet does not define.
type commonOpts struct {
	explain, noInput, json bool
	fields                 string
	hasFields              bool
	rewritten              string // replacement argv word for --null (take returned n == 0)
}

// take consumes one common option at out[i]. It returns the number of argv
// words to drop (0 means replace out[i] with co.rewritten), -1 on a usage
// error already printed, and ok=false when name is not a common option.
func (co *commonOpts) take(rc *RunContext, t *Tool, fs *pflag.FlagSet, name string, hasValue bool, value string, args []string, i int) (n int, ok bool) {
	switch name {
	case "explain", "no-input", "json":
		if hasValue {
			return 0, false
		}
		switch name {
		case "explain":
			co.explain = true
		case "no-input":
			co.noInput = true
		default:
			co.json = true
		}
		return 1, true
	case "fields":
		co.hasFields = true
		if hasValue {
			co.fields = value
			return 1, true
		}
		if i+1 >= len(args) {
			UsageError(rc, t, "option '--fields' requires an argument")
			return -1, true
		}
		co.fields = args[i+1]
		return 2, true
	case "null":
		if hasValue {
			return 0, false
		}
		for _, native := range nativeNullFlags {
			if f := fs.Lookup(native); f != nil && !flagTakesValue(f) {
				co.rewritten = "--" + native
				return 0, true
			}
		}
		NotSupported(rc, t, "--null (no native zero-terminated spelling)")
		return -1, true
	}
	return 0, false
}

// apply runs after a successful parse: unsupported structured output and
// prompt requests fail with exit 2, --explain prints and returns 0. Nothing
// here consumes rc.In, so piped input is preserved under --no-input.
func (co *commonOpts) apply(rc *RunContext, t *Tool, fs *pflag.FlagSet) int {
	if co.json || co.hasFields {
		what := "--json"
		if !co.json {
			what = "--fields"
		}
		return NotSupported(rc, t, what+" (no native or declared structured output)")
	}
	if co.noInput {
		for _, n := range []string{"interactive", "interactive-once"} {
			if f := fs.Lookup(n); f != nil && f.Changed && f.Value.String() != "false" && f.Value.String() != "never" && f.Value.String() != "no" {
				fmt.Fprintf(rc.Err, "%s: --%s would prompt, but --no-input forbids prompting; not executed\n", t.Name, n)
				return 2
			}
		}
	}
	if co.explain {
		fmt.Fprintf(rc.Out, "explain: %s\n", t.Name)
		fmt.Fprintf(rc.Out, "usage: %s\n", t.Usage)
		var set []string
		fs.Visit(func(f *pflag.Flag) {
			if f.Name == helpAliasFlag || f.Name == versionAliasFlag {
				return
			}
			if f.Value.Type() == "bool" {
				set = append(set, "--"+f.Name)
			} else {
				set = append(set, "--"+f.Name+"="+f.Value.String())
			}
		})
		fmt.Fprintf(rc.Out, "flags: %s\n", strings.Join(set, " "))
		fmt.Fprintf(rc.Out, "operands: %s\n", strings.Join(fs.Args(), " "))
		fmt.Fprintf(rc.Out, "effects: none; arguments were parsed and validated only, the command was not executed\n")
		return 0
	}
	return -1
}
