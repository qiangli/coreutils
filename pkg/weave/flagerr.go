package weave

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/qiangli/coreutils/pkg/weavecli"
)

// weaveFlagErrorFunc is installed on the `weave` root (cobra's
// FlagErrorFunc() climbs to the parent, so EVERY subverb inherits it)
// and turns a flag-parse failure into a LOUD, self-reported usage error.
//
// Why this exists: every weave command sets SilenceErrors/SilenceUsage
// (subverbs print their own envelope, so cobra must not double-print).
// That silence also swallowed cobra's own structural errors — a bad flag
// never reaches RunE, so nothing printed it, and `weave baton write
// --note ...` exited non-zero with ZERO output. A conductor read that as
// "checkpoint written" and the successor got a stale baton.
//
// Rather than delegating the message to the host (bashy's `case "weave"`
// dispatch), weave reports flag errors itself and returns an
// *exitCodeError — so IsStructuredExit is true, the host stays silent,
// and there is exactly one message no matter who drives the tree.
func weaveFlagErrorFunc(cmd *cobra.Command, err error) error {
	mode := flagErrOutputMode(cmd)
	msg := err.Error()
	if name, ok := unknownFlagName(msg); ok {
		if best, ok := nearestFlag(cmd, name); ok {
			msg = fmt.Sprintf("%s; did you mean --%s?", msg, best)
		}
	}
	msg = fmt.Sprintf("%s (run `%s --help` for the supported flags)", msg, cmd.CommandPath())
	return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, cmd.CommandPath(),
		weavecli.ExitInvalidArg, fmt.Errorf("%s", msg)))
}

// flagErrOutputMode resolves --json/--plain/--quiet from whatever pflag
// managed to parse BEFORE the offending flag (pflag sets flags as it
// goes and stops at the error), so `weave list --json --bogus` still
// answers in the envelope shape the caller asked for.
func flagErrOutputMode(cmd *cobra.Command) weavecli.OutputMode {
	boolFlag := func(name string) (set, val bool) {
		f := cmd.Flags().Lookup(name)
		if f == nil {
			return false, false
		}
		return f.Changed, f.Value.String() == "true"
	}
	jsonSet, jsonVal := boolFlag("json")
	_, plain := boolFlag("plain")
	_, quiet := boolFlag("quiet")
	return weavecli.ResolveOutputModeEx(jsonSet, jsonVal, plain, quiet)
}

// unknownFlagName extracts the offending flag name from pflag's error
// text ("unknown flag: --note", "unknown shorthand flag: 'z' in -z",
// "flag needs an argument: --tool"). Returns ok=false when the message
// carries no recoverable name — the raw error is still reported.
func unknownFlagName(msg string) (string, bool) {
	i := strings.Index(msg, "--")
	if i < 0 {
		return "", false
	}
	name := msg[i+2:]
	// "flag needs an argument: --tool" style messages end at the name;
	// defensive trim for any trailing clause.
	if j := strings.IndexAny(name, " \t'\""); j >= 0 {
		name = name[:j]
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	// --flag=value forms: compare on the name half only.
	if j := strings.Index(name, "="); j >= 0 {
		name = name[:j]
	}
	return name, name != ""
}

// nearestFlag picks the closest valid long flag (local + inherited) to
// the misspelled one: an edit distance within a third of the name's
// length, ties broken alphabetically for determinism.
func nearestFlag(cmd *cobra.Command, name string) (string, bool) {
	var candidates []string
	seen := map[string]bool{}
	collect := func(fs *pflag.FlagSet) {
		if fs == nil {
			return
		}
		fs.VisitAll(func(f *pflag.Flag) {
			if f.Hidden || seen[f.Name] {
				return
			}
			seen[f.Name] = true
			candidates = append(candidates, f.Name)
		})
	}
	collect(cmd.Flags())
	collect(cmd.InheritedFlags())
	collect(cmd.PersistentFlags())
	sort.Strings(candidates)

	limit := len(name)/3 + 1
	best, bestD := "", limit+1
	for _, c := range candidates {
		// A misspelling that is a prefix/extension of a real flag
		// (--note vs --notes) is the common conductor typo: treat it as
		// distance 1 regardless of length.
		d := editDistance(name, c)
		if strings.HasPrefix(c, name) || strings.HasPrefix(name, c) {
			d = 1
		}
		if d <= limit && d < bestD {
			best, bestD = c, d
		}
	}
	return best, best != ""
}

// editDistance is the plain Levenshtein distance (no deps; the flag
// name sets are tiny).
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
