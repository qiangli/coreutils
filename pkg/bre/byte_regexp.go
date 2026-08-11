package bre

import "fmt"

// ByteCtype is the locale byte-classification surface required by
// CompileLocaleByteRegexp. It is intentionally identical to pkg/ctype's
// classification and lowercase methods without importing that package.
type ByteCtype interface {
	IsAlpha(byte) (bool, error)
	IsAlnum(byte) (bool, error)
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
}

// ByteRegexpSyntax selects the POSIX expression grammar.
type ByteRegexpSyntax uint8

const (
	// ByteRegexpBRE selects POSIX basic regular-expression syntax.
	ByteRegexpBRE ByteRegexpSyntax = iota
	// ByteRegexpERE selects POSIX extended regular-expression syntax.
	ByteRegexpERE
)

// ByteRegexpOptions controls compilation. Locale folding is expanded into
// explicit byte-token alternatives; it never relies on RE2's Unicode (?i).
// Dot-newline and multiline modes are deliberately deferred until their
// complete POSIX utility contracts can be wired without approximation.
type ByteRegexpOptions struct {
	Syntax   ByteRegexpSyntax
	FoldCase bool
}

// LocaleByteRegexp is a POSIX leftmost-longest regexp over locale-classified
// single-byte input. All observable indices refer to the original raw bytes.
type LocaleByteRegexp struct {
	pattern *localeBytePattern
}

// CompileLocaleByteRegexp snapshots all twelve CType classes and the complete
// 256-byte lowercase table before compiling. It does not retain provider.
func CompileLocaleByteRegexp(pattern []byte, provider ByteCtype, options ByteRegexpOptions) (*LocaleByteRegexp, error) {
	if provider == nil {
		return nil, fmt.Errorf("locale byte regexp: nil CType provider")
	}
	tables, err := buildBytePatternTables(provider)
	if err != nil {
		return nil, err
	}
	var compiled *localeBytePattern
	switch options.Syntax {
	case ByteRegexpBRE:
		compiled, err = compileLocaleBytePattern(pattern, tables, options.FoldCase)
	case ByteRegexpERE:
		compiled, err = compileLocaleByteERE(pattern, tables, options.FoldCase)
	default:
		return nil, fmt.Errorf("locale byte regexp: unsupported syntax %d", options.Syntax)
	}
	if err != nil {
		return nil, err
	}
	return &LocaleByteRegexp{pattern: compiled}, nil
}

type byteClassifier func(byte) (bool, error)

func buildBytePatternTables(provider ByteCtype) (bytePatternTables, error) {
	classifiers := []struct {
		name string
		fn   byteClassifier
	}{
		{"alpha", provider.IsAlpha}, {"alnum", provider.IsAlnum},
		{"blank", provider.IsBlank}, {"cntrl", provider.IsCntrl},
		{"digit", provider.IsDigit}, {"graph", provider.IsGraph},
		{"lower", provider.IsLower}, {"print", provider.IsPrint},
		{"punct", provider.IsPunct}, {"space", provider.IsSpace},
		{"upper", provider.IsUpper}, {"xdigit", provider.IsXDigit},
	}
	tables := bytePatternTables{classes: make(map[string][256]bool, len(classifiers)+1)}
	for _, classifier := range classifiers {
		var class [256]bool
		for value := 0; value < 256; value++ {
			member, err := classifier.fn(byte(value))
			if err != nil {
				return bytePatternTables{}, fmt.Errorf("locale byte regexp: %s classification for byte %#02x: %w", classifier.name, value, err)
			}
			class[value] = member
		}
		tables.classes[classifier.name] = class
	}
	word := tables.classes["alnum"]
	word['_'] = true
	tables.classes["word"] = word
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	lower, err := provider.ToLower(all)
	if err != nil {
		return bytePatternTables{}, fmt.Errorf("locale byte regexp: lowercase table: %w", err)
	}
	if len(lower) != 256 {
		return bytePatternTables{}, fmt.Errorf("locale byte regexp: lowercase table has %d bytes, want 256", len(lower))
	}
	copy(tables.fold[:], lower)
	return tables, nil
}

// FindSubmatchIndex returns the leftmost-longest match and submatches as raw
// byte offsets. Invariant failures are returned, never converted to no-match.
func (r *LocaleByteRegexp) FindSubmatchIndex(src []byte) ([]int, error) {
	return r.pattern.findSubmatchIndex(src)
}

// MatchString reports whether src matches and preserves invariant errors.
func (r *LocaleByteRegexp) MatchString(src string) (bool, error) {
	match, err := r.FindSubmatchIndex([]byte(src))
	return match != nil, err
}

// Expand appends template expansion using raw source offsets.
func (r *LocaleByteRegexp) Expand(dst, template, src []byte, match []int) ([]byte, error) {
	if err := validateRawMatch(len(src), r.pattern.re.NumSubexp(), match); err != nil {
		return nil, err
	}
	return expandTemplate(dst, string(template), string(src), match), nil
}

// ExpandString appends template expansion using raw source offsets.
func (r *LocaleByteRegexp) ExpandString(dst []byte, template, src string, match []int) ([]byte, error) {
	if err := validateRawMatch(len(src), r.pattern.re.NumSubexp(), match); err != nil {
		return nil, err
	}
	return expandTemplate(dst, template, src, match), nil
}

func validateRawMatch(srcLen, numSubexp int, match []int) error {
	wantLen := 2 * (numSubexp + 1)
	if len(match) != wantLen {
		return fmt.Errorf("locale byte regexp: match index vector has %d entries, want %d", len(match), wantLen)
	}
	wholeStart, wholeEnd := match[0], match[1]
	if wholeStart < 0 || wholeEnd < wholeStart || wholeEnd > srcLen {
		return fmt.Errorf("locale byte regexp: invalid whole-match offsets [%d,%d]", wholeStart, wholeEnd)
	}
	for i := 2; i < len(match); i += 2 {
		start, end := match[i], match[i+1]
		if start == -1 && end == -1 {
			continue
		}
		if start < wholeStart || end < start || end > wholeEnd {
			return fmt.Errorf("locale byte regexp: invalid capture offsets [%d,%d] outside whole match [%d,%d]", start, end, wholeStart, wholeEnd)
		}
	}
	return nil
}
