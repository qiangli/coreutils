package cutcmd

// Invocation-local LC_CTYPE semantics (issue 736), mirroring the bounded
// character model cmds/fold established for issue 735 without importing it:
// the locale selects character boundaries only, and source bytes are never
// re-encoded on the way to standard output.

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/locale"
)

type encodingMode int

const (
	// encodingSingleByte covers C/POSIX and the carried de_DE.ISO-8859-1
	// aliases: every byte is exactly one character, so -c selects the
	// same spans as -b and -b -n has no multi-byte characters to keep
	// whole.
	encodingSingleByte encodingMode = iota
	encodingUTF8
)

// resolveEncoding maps the effective LC_CTYPE (LC_ALL > LC_CTYPE > LANG >
// POSIX default, per pkg/locale) onto the bounded encoding inventory this
// implementation carries. Any other locale name is a hard error the caller
// must surface before reading input or writing output.
func resolveEncoding(env []string) (encodingMode, error) {
	name := locale.Resolve(env, locale.CType)
	base, codeset := splitLocaleName(name)
	switch {
	case (base == "C" || base == "POSIX") && codeset == "":
		return encodingSingleByte, nil
	case (base == "C" || base == "POSIX") && normalizeCodeset(codeset) == "UTF8":
		return encodingUTF8, nil
	case strings.EqualFold(base, "de_DE") && normalizeCodeset(codeset) == "ISO88591":
		return encodingSingleByte, nil
	default:
		return 0, fmt.Errorf(
			"LC_CTYPE %q is unavailable; supported locales are C/POSIX, their UTF-8 aliases, and de_DE.ISO-8859-1",
			name,
		)
	}
}

func splitLocaleName(name string) (base, codeset string) {
	name, _, _ = strings.Cut(name, "@")
	base, codeset, _ = strings.Cut(name, ".")
	return base, codeset
}

func normalizeCodeset(name string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(name))
}

// isSingleCharacter reports whether s is exactly one character under enc,
// which is what POSIX requires of the -d delim operand-argument.
func isSingleCharacter(s string, enc encodingMode) bool {
	if enc != encodingUTF8 {
		return len(s) == 1
	}
	r, size := utf8.DecodeRuneInString(s)
	return size == len(s) && !(r == utf8.RuneError && size == 1)
}
