// Package locale resolves POSIX locale-category names from an invocation
// environment (tool.RunContext.Env) without touching the process.
//
// The resolver is a set of pure functions: given an []string in
// os.Environ() shape (KEY=VALUE entries, last duplicate wins) and a
// category, it returns the effective locale name following POSIX §8.2
// precedence. It never calls [os.Environ], [os.Setenv], setlocale, or any
// process-global API, and it returns the raw locale name without
// normalization, canonicalization, or case folding — codeset and modifier
// (e.g. ".UTF-8@euro") are preserved exactly as they appeared.
//
// The consumers that need locale resolution are pure-Go reimplementations
// of comm, sort, sed, tr, and awk. They use it to replace process-global
// locale state with invocation-owned category selection while retaining
// deterministic C/POSIX behavior.
// The helper in cmds/grep/locale.go that duplicates this logic is
// expected to migrate here once grep is refactored onto the shared package.
package locale

import "strings"

// Category identifies a POSIX locale category.
type Category string

// The five POSIX locale categories that sort, sed, tr, and awk may consult.
// Each corresponds to an LC_* environment variable of the same name.
const (
	// Collate is LC_COLLATE (string collation order: sort, sed, tr).
	Collate Category = "LC_COLLATE"
	// CType is LC_CTYPE (character classification, case mapping: sed, tr, awk).
	CType Category = "LC_CTYPE"
	// Numeric is LC_NUMERIC (decimal-point character: awk, printf).
	Numeric Category = "LC_NUMERIC"
	// Messages is LC_MESSAGES (affirmative/negative response format: awk
	// on some systems; also used by tools that emit translated diagnostics).
	Messages Category = "LC_MESSAGES"
	// Time is LC_TIME (date and time formats: date, awk).
	Time Category = "LC_TIME"
)

// Default is the effective locale name when LC_ALL, the category variable,
// and LANG are all unset or empty. POSIX leaves that default
// implementation-defined; this coreutils implementation deliberately chooses
// the POSIX locale for deterministic behavior on every host.
const Default = "POSIX"

// Categories holds the effective locale name for every supported category,
// resolved from a single environment snapshot via [ResolveAll].
type Categories struct {
	// All is the raw LC_ALL value (or [Default] when unset or empty).
	// It is informational: the per-category fields already reflect
	// LC_ALL's override when it is nonempty.
	All string
	// Collate is the effective LC_COLLATE.
	Collate string
	// CType is the effective LC_CTYPE.
	CType string
	// Numeric is the effective LC_NUMERIC.
	Numeric string
	// Messages is the effective LC_MESSAGES.
	Messages string
	// Time is the effective LC_TIME.
	Time string
}

// Resolve returns the effective locale name for a single category from the
// given environment ([]string in os.Environ() shape; last duplicate key
// wins, matching [tool.RunContext.Getenv] semantics).
//
// Environment precedence — the first nonempty value wins, per POSIX §8.2:
//
//  1. LC_ALL
//  2. the specific category variable (cat, e.g. LC_COLLATE)
//  3. LANG
//  4. [Default] ("POSIX"), this implementation's deterministic fallback
//
// Empty values fall through: an assignment such as LC_COLLATE= (empty) does
// not shadow LANG, and LC_ALL= (empty) does not shadow the per-category
// variable. This matches glibc, where an empty LC_* value is treated the
// same as the variable being unset.
//
// The returned string is the locale name exactly as it appeared in the
// environment — including codeset and modifier (e.g.
// "de_DE.ISO-8859-1@euro") — with no normalization, canonicalization, or
// case folding. Callers that need to branch on specific names do their own
// matching.
//
// Resolve is a pure function: it reads only its arguments and has no side
// effects.
func Resolve(env []string, cat Category) string {
	for _, key := range []string{"LC_ALL", string(cat), "LANG"} {
		if key == "" {
			continue // zero-value Category — skip to next precedence level
		}
		if v, ok := getEnv(env, key); ok && v != "" {
			return v
		}
	}
	return Default
}

// ResolveAll resolves every supported category from the same environment
// snapshot, plus the raw LC_ALL value. It is a convenience wrapper around
// [Resolve] so callers that need multiple categories can avoid repeated
// full-precedence scans.
func ResolveAll(env []string) Categories {
	return Categories{
		All:      resolveAllVar(env),
		Collate:  Resolve(env, Collate),
		CType:    Resolve(env, CType),
		Numeric:  Resolve(env, Numeric),
		Messages: Resolve(env, Messages),
		Time:     Resolve(env, Time),
	}
}

// resolveAllVar returns the raw LC_ALL value, or Default when unset or empty.
func resolveAllVar(env []string) string {
	if v, ok := getEnv(env, "LC_ALL"); ok && v != "" {
		return v
	}
	return Default
}

// getEnv looks up key in env (os.Environ() shape). The last duplicate wins,
// matching tool.RunContext.Getenv semantics: when the environment is built
// by appending, the final assignment is the effective one. The bool reports
// whether key was present at all (even when its value is empty), so callers
// can distinguish "set but empty" from "unset".
func getEnv(env []string, key string) (string, bool) {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return env[i][len(prefix):], true
		}
	}
	return "", false
}

// MessageMatcher is an invocation-owned projection of the LC_MESSAGES
// affirmative expression. Its match is anchored at the first response byte,
// as POSIX yesexpr is; callers must not trim leading white space first.
type MessageMatcher struct{ affirmativeInitials string }

// MessagesMatcher resolves the affirmative matcher for an invocation. The
// deterministic C/POSIX expression is the fallback for locales whose catalog
// is not embedded. German locales additionally accept their customary j/J.
func MessagesMatcher(env []string) MessageMatcher {
	name := strings.ToLower(Resolve(env, Messages))
	if strings.HasPrefix(name, "de_de") {
		return MessageMatcher{affirmativeInitials: "jJyY"}
	}
	return MessageMatcher{affirmativeInitials: "yY"}
}

// MatchAffirmative reports whether response matches the invocation's anchored
// LC_MESSAGES affirmative expression.
func MatchAffirmative(env []string, response string) bool {
	return MessagesMatcher(env).MatchAffirmative(response)
}

// MatchAffirmative applies the matcher's anchored affirmative expression.
func (m MessageMatcher) MatchAffirmative(response string) bool {
	return response != "" && strings.ContainsRune(m.affirmativeInitials, rune(response[0]))
}
