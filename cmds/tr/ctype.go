// Stage A LC_CTYPE: injectable provider lifecycle and pre-computed class
// tables.  Provider-backed ordinary classes work for delete, squeeze,
// complement, and SET1-to-literal translation; paired lower/upper
// translation under non-C locales is explicitly unsupported until Stage B.

package trcmd

import (
	"fmt"

	"github.com/qiangli/coreutils/pkg/ctype"
	"github.com/qiangli/coreutils/pkg/locale"
)

// ctypeOpener opens a ctype provider for the named locale.
// Production code passes ctype.Open; tests inject fakes.
type ctypeOpener func(name string) (ctypeProvider, error)

// ctypeProvider is the narrow interface that tr needs from a locale
// provider.  It mirrors pkg/ctype.Provider's 12 classification methods,
// ToLower, ToUpper, and Close — nothing else.
type ctypeProvider interface {
	IsAlnum(byte) (bool, error)
	IsAlpha(byte) (bool, error)
	IsBlank(byte) (bool, error)
	IsCntrl(byte) (bool, error)
	IsDigit(byte) (bool, error)
	IsGraph(byte) (bool, error)
	IsLower(byte) (bool, error)
	IsPrint(byte) (bool, error)
	IsPunct(byte) (bool, error)
	IsSpace(byte) (bool, error)
	IsUpper(byte) (bool, error)
	IsXDigit(byte) (bool, error)
	ToLower([]byte) ([]byte, error)
	ToUpper([]byte) ([]byte, error)
	Close() error
}

// ctypeTables holds every character class in ascending byte order plus
// exact [256]byte case-mapping tables, precomputed from a provider or
// from the hardcoded C-locale definitions.
type ctypeTables struct {
	alnum  []byte
	alpha  []byte
	blank  []byte
	cntrl  []byte
	digit  []byte
	graph  []byte
	lower  []byte
	print  []byte
	punct  []byte
	space  []byte
	upper  []byte
	xdigit []byte

	toLower [256]byte
	toUpper [256]byte
}

// classFromTable returns the members of the named class from the tables
// together with a boolean indicating whether the class was recognised.
// A known class returns true even when its membership is empty; an
// unrecognised name returns (nil, false).
func (ct *ctypeTables) classFromTable(name string) ([]byte, bool) {
	switch name {
	case "alnum":
		return ct.alnum, true
	case "alpha":
		return ct.alpha, true
	case "blank":
		return ct.blank, true
	case "cntrl":
		return ct.cntrl, true
	case "digit":
		return ct.digit, true
	case "graph":
		return ct.graph, true
	case "lower":
		return ct.lower, true
	case "print":
		return ct.print, true
	case "punct":
		return ct.punct, true
	case "space":
		return ct.space, true
	case "upper":
		return ct.upper, true
	case "xdigit":
		return ct.xdigit, true
	default:
		return nil, false
	}
}

// isCPOSIX reports whether name is C, POSIX, or empty — any value that
// selects the C/POSIX locale behaviour.
func isCPOSIX(name string) bool {
	return name == "" || name == "C" || name == "POSIX"
}

// buildCLocale produces ctypeTables from the hardcoded ASCII predicates
// (the existing C-locale behaviour). It never touches a provider.
func buildCLocale() *ctypeTables {
	ct := &ctypeTables{}
	for c := 0; c < 256; c++ {
		b := byte(c)
		if classPred["alnum"](b) {
			ct.alnum = append(ct.alnum, b)
		}
		if classPred["alpha"](b) {
			ct.alpha = append(ct.alpha, b)
		}
		if classPred["blank"](b) {
			ct.blank = append(ct.blank, b)
		}
		if classPred["cntrl"](b) {
			ct.cntrl = append(ct.cntrl, b)
		}
		if classPred["digit"](b) {
			ct.digit = append(ct.digit, b)
		}
		if classPred["graph"](b) {
			ct.graph = append(ct.graph, b)
		}
		if classPred["lower"](b) {
			ct.lower = append(ct.lower, b)
		}
		if classPred["print"](b) {
			ct.print = append(ct.print, b)
		}
		if classPred["punct"](b) {
			ct.punct = append(ct.punct, b)
		}
		if classPred["space"](b) {
			ct.space = append(ct.space, b)
		}
		if classPred["upper"](b) {
			ct.upper = append(ct.upper, b)
		}
		if classPred["xdigit"](b) {
			ct.xdigit = append(ct.xdigit, b)
		}
	}
	// C-locale case maps: pure ASCII.
	for c := 0; c < 256; c++ {
		ct.toLower[c] = asciiLower(byte(c))
		ct.toUpper[c] = asciiUpper(byte(c))
	}
	return ct
}

// buildProviderTables probes a ctypeProvider over bytes 0..255 to
// produce ctypeTables.  On success the provider is closed exactly once
// before returning; on failure it is still closed.  Build errors take
// precedence over Close errors.
func buildProviderTables(p ctypeProvider) (*ctypeTables, error) {
	ct := &ctypeTables{}
	var buildErr error

	// Helper to probe one classifier over all 256 bytes.
	probe := func(is func(byte) (bool, error)) ([]byte, error) {
		var out []byte
		for c := 0; c < 256; c++ {
			ok, err := is(byte(c))
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, byte(c))
			}
		}
		return out, nil
	}

	type classSlot struct {
		name string
		is   func(byte) (bool, error)
		dst  *[]byte
	}
	slots := []classSlot{
		{"alnum", p.IsAlnum, &ct.alnum},
		{"alpha", p.IsAlpha, &ct.alpha},
		{"blank", p.IsBlank, &ct.blank},
		{"cntrl", p.IsCntrl, &ct.cntrl},
		{"digit", p.IsDigit, &ct.digit},
		{"graph", p.IsGraph, &ct.graph},
		{"lower", p.IsLower, &ct.lower},
		{"print", p.IsPrint, &ct.print},
		{"punct", p.IsPunct, &ct.punct},
		{"space", p.IsSpace, &ct.space},
		{"upper", p.IsUpper, &ct.upper},
		{"xdigit", p.IsXDigit, &ct.xdigit},
	}
	for _, s := range slots {
		members, err := probe(s.is)
		if err != nil {
			buildErr = fmt.Errorf("classifying %s: %w", s.name, err)
			break
		}
		*s.dst = members
	}

	// ToLower / ToUpper: pass all 256 bytes, require exactly 256 back.
	if buildErr == nil {
		all := make([]byte, 256)
		for i := range all {
			all[i] = byte(i)
		}
		lo, err := p.ToLower(all)
		if err != nil {
			buildErr = fmt.Errorf("ToLower: %w", err)
		} else if len(lo) != 256 {
			buildErr = fmt.Errorf("ToLower returned %d bytes, want 256", len(lo))
		} else {
			copy(ct.toLower[:], lo)
		}
	}
	if buildErr == nil {
		all := make([]byte, 256)
		for i := range all {
			all[i] = byte(i)
		}
		up, err := p.ToUpper(all)
		if err != nil {
			buildErr = fmt.Errorf("ToUpper: %w", err)
		} else if len(up) != 256 {
			buildErr = fmt.Errorf("ToUpper returned %d bytes, want 256", len(up))
		} else {
			copy(ct.toUpper[:], up)
		}
	}

	closeErr := p.Close()
	if buildErr != nil {
		return nil, buildErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return ct, nil
}

// prodOpener adapts ctype.Open to the ctypeOpener signature.
func prodOpener(name string) (ctypeProvider, error) {
	return ctype.Open(name)
}

// openCType resolves the effective LC_CTYPE from rc.Env and builds
// ctypeTables.  For C/POSIX (including the default) it returns pure-Go
// tables and never calls opener.  For other locales it calls opener,
// builds tables, and closes the provider exactly once.
//
// The returned lcCType is the effective locale name for diagnostics.
func openCType(env []string, opener ctypeOpener) (tables *ctypeTables, lcCType string, err error) {
	lcCType = locale.Resolve(env, locale.CType)
	if isCPOSIX(lcCType) {
		return buildCLocale(), lcCType, nil
	}
	p, err := opener(lcCType)
	if err != nil {
		return nil, lcCType, err
	}
	tables, err = buildProviderTables(p)
	return tables, lcCType, err
}
