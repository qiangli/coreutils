package sedcmd

// Invocation-local locale-category resolution for sed.
//
// POSIX.1 Issue 7 XBD 7.3 gives each category a disjoint job, and sed uses
// exactly two of them (XCU sed, ENVIRONMENT VARIABLES):
//
//   - LC_CTYPE decides the CHARACTER MODEL of the BRE/ERE and of the
//     pattern space — how bytes group into characters, what the
//     [:class:] names denote, and how the I/i modifier folds case.
//   - LC_COLLATE decides the COLLATION — what [=equivalence=] and
//     [.collating-symbol.] denote inside a bracket expression, and how a
//     range's endpoints order.
//
// Each category is resolved on its own here and nothing is derived from
// the pair. That is the whole point of this file: a UTF-8 LC_COLLATE is
// not permission to change the character model, and a UTF-8 LC_CTYPE is
// not permission to change the collation.
//
// The inventory is the bounded one the rest of the userland carries (see
// cmds/cut, cmds/od, pkg/locale): C/POSIX, their UTF-8 aliases, and the
// carried single-byte non-C locales served by pkg/ctype and pkg/collate.
// A name outside it is not approximated — it reaches the provider, which
// rejects it by name, and sed exits 2 with that diagnostic.

import "strings"

// sedCType is the character model LC_CTYPE selects.
type sedCType int

const (
	// sedCTypeC is C/POSIX: one byte is one character and the POSIX
	// classes are the C locale's ASCII sets.
	sedCTypeC sedCType = iota
	// sedCTypeUTF8 is a C/POSIX UTF-8 alias ("C.UTF-8", "POSIX.utf8", …).
	// A character is one UTF-8 sequence, so '.' spans a whole multi-byte
	// character. No byte-table provider can describe that codeset, so
	// this model deliberately stays on the character-oriented matcher
	// rather than being approximated by the C byte tables.
	sedCTypeUTF8
	// sedCTypeProvider is any other name: the character model has to come
	// from pkg/ctype, which serves the carried single-byte locales and
	// rejects everything else by name.
	sedCTypeProvider
)

// sedCollate is the collation model LC_COLLATE selects.
type sedCollate int

const (
	// sedCollateC is C/POSIX and its UTF-8 aliases. POSIX fixes the C
	// locale's collation: every collating element is a single character,
	// ordered by character value, and no character has an equivalent.
	// The UTF-8 aliases share that ordering — UTF-8 encodes code points
	// in ascending byte order — so they need no provider either.
	sedCollateC sedCollate = iota
	// sedCollateProvider is any other name: weights, equivalence classes
	// and collating elements come from pkg/collate.
	sedCollateProvider
)

func resolveSedCType(name string) sedCType {
	base, codeset := splitLocaleName(name)
	if base != "C" && base != "POSIX" {
		return sedCTypeProvider
	}
	switch normalizeCodeset(codeset) {
	case "":
		return sedCTypeC
	case "UTF8":
		return sedCTypeUTF8
	default:
		return sedCTypeProvider
	}
}

func resolveSedCollate(name string) sedCollate {
	base, codeset := splitLocaleName(name)
	if base != "C" && base != "POSIX" {
		return sedCollateProvider
	}
	switch normalizeCodeset(codeset) {
	case "", "UTF8":
		return sedCollateC
	default:
		return sedCollateProvider
	}
}

// splitLocaleName separates "C.UTF-8@modifier" into its base name and
// codeset, discarding the modifier, exactly as cmds/cut and cmds/od do.
func splitLocaleName(name string) (base, codeset string) {
	name, _, _ = strings.Cut(name, "@")
	base, codeset, _ = strings.Cut(name, ".")
	return base, codeset
}

func normalizeCodeset(name string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(name))
}
