package tool

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// nativeNullFlags are the documented upstream spellings --null may map to,
// in preference order. --null itself is never invented for a tool.
var nativeNullFlags = []string{"zero", "zero-terminated", "null-data"}

// commonOpts tracks the long-only agent options registered on a FlagSet
// outside the cert profile. A tool's own flag of the same name always wins:
// registration skips any name the FlagSet already defines.
type commonOpts struct {
	added     map[string]bool
	rewritten string // replacement argv word for --null
}

// register defines the agent options on fs (non-cert only). --null is a
// scan-time alias for the command's native NUL spelling and never reaches
// pflag as itself.
func (co *commonOpts) register(fs *pflag.FlagSet, t *Tool) {
	co.added = map[string]bool{}
	add := func(name string, def func()) {
		if fs.Lookup(name) == nil {
			def()
			co.added[name] = true
		}
	}
	if t.PreviewLevel == "local-preview" {
		add("dry-run", func() {
			fs.Bool("dry-run", false, "record a conditional local effect plan without changing files; not a success guarantee")
		})
	}
	add("explain", func() {
		fs.Bool("explain", false, "print what would run, parsed and validated, and exit without executing")
	})
	add("no-input", func() {
		fs.Bool("no-input", false, "fail with a distinct error instead of prompting; piped data input is preserved")
	})
	add("json", func() { fs.Bool("json", false, "structured output (exit 2 if the command has no structured output)") })
	add("fields", func() { fs.String("fields", "", "select structured output fields (exit 2 if unsupported)") })
	add("null", func() {
		fs.Bool("null", false, "use the command's native NUL-delimited spelling (-z/--zero/--null-data)")
	})
}

func (co *commonOpts) isCommon(name string) bool { return co.added[name] }

// takeNull rewrites a bare --null to the command's native zero-terminated
// spelling. n is the words to drop (0 = replace), -1 on error.
func (co *commonOpts) takeNull(rc *RunContext, t *Tool, fs *pflag.FlagSet) int {
	for _, native := range nativeNullFlags {
		if f := fs.Lookup(native); f != nil && !flagTakesValue(f) && !co.added[native] {
			co.rewritten = "--" + native
			return 0
		}
	}
	NotSupported(rc, t, "--null (no native zero-terminated spelling)")
	return -1
}

// apply runs after a successful parse. --explain wins over every other
// common option and never executes; otherwise unsupported structured output
// fails with exit 2. --no-input only arms rc.NoInput: prompting commands
// fail at prompt time via PromptBlocked, and rc.In is never touched here.
func (co *commonOpts) apply(rc *RunContext, t *Tool, fs *pflag.FlagSet) int {
	on := func(name string) bool { return co.added[name] && fs.Lookup(name).Changed }
	json, fields, noInput := on("json"), on("fields"), on("no-input")
	if on("explain") {
		fmt.Fprintf(rc.Out, "explain: %s\n", t.Name)
		fmt.Fprintf(rc.Out, "usage: %s\n", t.Usage)
		var set, common []string
		fs.Visit(func(f *pflag.Flag) {
			if f.Name == helpAliasFlag || f.Name == versionAliasFlag {
				return
			}
			s := "--" + f.Name
			if f.Value.Type() != "bool" {
				s += "=" + f.Value.String()
			}
			if co.added[f.Name] {
				common = append(common, s)
			} else {
				set = append(set, s)
			}
		})
		if co.rewritten != "" {
			set = append(set, co.rewritten)
		}
		fmt.Fprintf(rc.Out, "flags: %s\n", strings.Join(set, " "))
		fmt.Fprintf(rc.Out, "common: %s\n", strings.Join(common, " "))
		fmt.Fprintf(rc.Out, "operands: %s\n", strings.Join(fs.Args(), " "))
		fmt.Fprintf(rc.Out, "effects: not evaluated; the command was not executed and its side effects are not known to the argument parser\n")
		return 0
	}
	if json || fields {
		what := "--json"
		if !json {
			what = "--fields"
		}
		return NotSupported(rc, t, what+" (no native or declared structured output)")
	}
	if co.added["dry-run"] {
		rc.dryRun = flagBool(fs, "dry-run")
	}
	if noInput {
		rc.NoInput = true
	}
	return -1
}

// PromptBlocked is called by a command immediately before it would write a
// prompt and read a reply. When --no-input is active it reports a distinct
// diagnostic (once), records the failure so the invocation exits non-zero,
// and returns true: the caller must then skip the prompt and treat the
// action as not confirmed. It never reads rc.In.
func PromptBlocked(rc *RunContext, name string) bool {
	if rc == nil || !rc.NoInput {
		return false
	}
	if !rc.noInputHit {
		rc.noInputHit = true
		fmt.Fprintf(rc.Err, "%s: input required (confirmation prompt), but --no-input forbids prompting; not confirmed\n", name)
	}
	return true
}
