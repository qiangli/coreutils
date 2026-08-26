package sortcmd

// LC_CTYPE character classification and case folding for the textual key
// modifiers -f (fold), -d (dictionary order), and -i (ignore nonprinting).
//
// POSIX sort defines these modifiers in terms of LC_CTYPE: -f folds a
// character to its uppercase equivalent, -d keeps only <blank>s and
// alphanumeric characters, and -i keeps only printable characters, all as
// classified by the current locale. In the C/POSIX locale those classes are
// exactly ASCII, so the byte tables in normalizeTextKey suffice; in a non-C
// LC_CTYPE the high-byte letters must fold and classify too.
//
// The provider is the invocation-owned pkg/ctype seam (glibc via dlopen on the
// supported linux targets, a fail-closed stub elsewhere), reached through
// ctypeOpener so tests inject a deterministic fake. Its 256-entry classes and
// uppercase map are snapshotted once at init and the provider is closed
// immediately; the immutable tables then drive the hot comparison path with no
// per-byte provider calls, matching the cmds/sed snapshot shape.

// ctypeProvider is the subset of pkg/ctype.Provider that sort's text-key
// normalization needs: uppercase folding for -f and the three membership
// classes for -d/-i.
type ctypeProvider interface {
	IsAlnum(byte) (bool, error)
	IsBlank(byte) (bool, error)
	IsPrint(byte) (bool, error)
	ToUpper([]byte) ([]byte, error)
	Close() error
}

type ctypeOpener func(string) (ctypeProvider, error)

// ctypeTables is an immutable snapshot of the LC_CTYPE classes and uppercase
// map for all 256 byte values. It is safe to read concurrently after the
// provider has been closed.
type ctypeTables struct {
	fold  [256]byte
	alnum [256]bool
	blank [256]bool
	print [256]bool
}

// snapshotCtypeTables materializes the provider's classification of every byte.
// A single provider error aborts the snapshot so an incomplete table never
// reaches the comparison path.
func snapshotCtypeTables(p ctypeProvider) (*ctypeTables, error) {
	t := &ctypeTables{}
	for c := 0; c < 256; c++ {
		b := byte(c)
		up, err := p.ToUpper([]byte{b})
		if err != nil {
			return nil, err
		}
		if len(up) == 1 {
			t.fold[c] = up[0]
		} else {
			t.fold[c] = b
		}
		if t.alnum[c], err = p.IsAlnum(b); err != nil {
			return nil, err
		}
		if t.blank[c], err = p.IsBlank(b); err != nil {
			return nil, err
		}
		if t.print[c], err = p.IsPrint(b); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// normalize applies -d/-i filtering and -f folding using the locale classes,
// mirroring the ASCII normalizeTextKey but with the snapshotted tables.
func (t *ctypeTables) normalize(s string, o keyOpts) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if o.dict && !(t.blank[c] || t.alnum[c]) {
			continue
		}
		if o.ignoreNP && !t.print[c] {
			continue
		}
		if o.fold {
			c = t.fold[c]
		}
		b = append(b, c)
	}
	return string(b)
}
