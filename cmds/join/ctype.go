package joincmd

import "github.com/qiangli/coreutils/pkg/ctype"

// LC_CTYPE case folding for -i. POSIX join defines -i case-insensitivity in
// terms of the current locale: two field characters match when they map to the
// same case as defined by LC_CTYPE, so under a non-C LC_CTYPE the high-byte
// letter pairs (e.g. Ä/ä) must fold equal, not just the ASCII pairs. GNU join's
// keycmp uses memcasecmp — a per-LC_CTYPE fold — for the same reason.
//
// The invocation-owned pkg/ctype provider (glibc via dlopen on the supported
// linux targets, a fail-closed stub elsewhere) is reached through ctypeOpener
// so tests inject a deterministic fake. Its uppercase map is snapshotted once
// into a 256-entry table and the provider is closed immediately; the immutable
// table then drives the comparison path with no per-byte provider calls.

// ctypeProvider is the subset of pkg/ctype.Provider that join's -i folding
// needs: an uppercase map over every byte value.
type ctypeProvider interface {
	ToUpper([]byte) ([]byte, error)
	Close() error
}

type ctypeOpener func(string) (ctypeProvider, error)

// openCType is the production opener wired in run.
func openCType(name string) (ctypeProvider, error) { return ctype.Open(name) }

// snapshotFoldTable materializes the provider's uppercase mapping of every
// byte. Folding to uppercase (rather than GNU's lowercase) keeps join's -i fold
// direction identical to its C-locale ASCII path (upperByte); the load-bearing
// requirement is that case variants of the same letter fold equal, which both
// directions satisfy.
func snapshotFoldTable(p ctypeProvider) (*[256]byte, error) {
	var t [256]byte
	for c := 0; c < 256; c++ {
		up, err := p.ToUpper([]byte{byte(c)})
		if err != nil {
			return nil, err
		}
		if len(up) == 1 {
			t[c] = up[0]
		} else {
			t[c] = byte(c)
		}
	}
	return &t, nil
}
