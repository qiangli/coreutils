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

// ByteEquivalence is the optional locale collation surface used for POSIX
// [=x=] bracket expressions. Providers that do not implement it retain the
// C-locale behavior where every single byte is equivalent only to itself.
type ByteEquivalence interface {
	Equivalents(byte) ([]byte, error)
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
// DotAll lets dot consume raw newline, and MultiLine makes ^ and $ operate at
// the preserved raw-newline boundaries.
type ByteRegexpOptions struct {
	Syntax    ByteRegexpSyntax
	FoldCase  bool
	DotAll    bool
	MultiLine bool
}

// LocaleByteRegexp is a POSIX leftmost-longest regexp over locale-classified
// single-byte input. All observable indices refer to the original raw bytes.
type LocaleByteRegexp struct {
	pattern *localeBytePattern
}

// LocaleByteTables is an immutable, opaque snapshot of all locale byte
// classifications and lowercase mappings needed by LocaleByteRegexp.
// A snapshot may be reused concurrently after its provider has been closed.
type LocaleByteTables struct {
	tables bytePatternTables
}

// SnapshotLocaleByteTables reads each of the twelve CType classifications for
// all 256 bytes and one exact 256-byte lowercase mapping. It retains no
// reference to provider.
func SnapshotLocaleByteTables(provider ByteCtype) (*LocaleByteTables, error) {
	if provider == nil {
		return nil, fmt.Errorf("locale byte regexp: nil CType provider")
	}
	tables, err := buildBytePatternTables(provider)
	if err != nil {
		return nil, err
	}
	return &LocaleByteTables{tables: tables}, nil
}

// CompileLocaleByteRegexp snapshots all twelve CType classes and the complete
// 256-byte lowercase table before compiling. It does not retain provider.
func CompileLocaleByteRegexp(pattern []byte, provider ByteCtype, options ByteRegexpOptions) (*LocaleByteRegexp, error) {
	tables, err := SnapshotLocaleByteTables(provider)
	if err != nil {
		return nil, err
	}
	return CompileLocaleByteRegexpTables(pattern, tables, options)
}

// CompileLocaleByteRegexpTables compiles against an immutable reusable
// snapshot. It never calls or retains the CType provider used to create it.
func CompileLocaleByteRegexpTables(pattern []byte, snapshot *LocaleByteTables, options ByteRegexpOptions) (*LocaleByteRegexp, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("locale byte regexp: nil locale byte tables")
	}
	tables := snapshot.tables
	tables.dotAll = options.DotAll
	tables.multi = options.MultiLine
	var err error
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
	for value := 0; value < 256; value++ {
		tables.equivalent[value][value] = true
	}
	if provider, ok := provider.(ByteEquivalence); ok {
		for value := 0; value < 256; value++ {
			members, err := provider.Equivalents(byte(value))
			if err != nil {
				return bytePatternTables{}, fmt.Errorf("locale byte regexp: equivalence class for byte %#02x: %w", value, err)
			}
			for _, member := range members {
				tables.equivalent[value][member] = true
			}
		}
	}
	return tables, nil
}

// FindSubmatchIndex returns the leftmost-longest match and submatches as raw
// byte offsets. Invariant failures are returned, never converted to no-match.
func (r *LocaleByteRegexp) FindSubmatchIndex(src []byte) ([]int, error) {
	return r.pattern.findSubmatchIndex(src)
}

// FindAllSubmatchIndex returns successive non-overlapping raw-byte matches.
// n < 0 means all and n == 0 means none. RE2 can observe empty matches between
// the two runes of one encoded byte; only those zero-length encoding artifacts
// are filtered. Every non-empty match and every retained capture must map to a
// raw boundary or the method returns an invariant error.
func (r *LocaleByteRegexp) FindAllSubmatchIndex(src []byte, n int) ([][]int, error) {
	if n == 0 {
		return nil, nil
	}
	encoded := r.pattern.codec.encodeSubject(src)
	matches := r.pattern.re.FindAllStringSubmatchIndex(encoded.text, -1)
	var decoded [][]int
	for _, match := range matches {
		if len(match) < 2 {
			return nil, fmt.Errorf("locale byte regexp: malformed RE2 match vector")
		}
		_, startErr := encoded.rawOffset(match[0])
		_, endErr := encoded.rawOffset(match[1])
		if match[0] == match[1] && startErr != nil {
			continue
		}
		if startErr != nil || endErr != nil {
			return nil, fmt.Errorf("locale byte regexp: non-boundary whole match invariant")
		}
		rawMatch := make([]int, len(match))
		for i, offset := range match {
			if offset < 0 {
				rawMatch[i] = -1
				continue
			}
			rawOffset, err := encoded.rawOffset(offset)
			if err != nil {
				return nil, fmt.Errorf("locale byte regexp: non-boundary capture invariant: %w", err)
			}
			rawMatch[i] = rawOffset
		}
		decoded = append(decoded, rawMatch)
		if n > 0 && len(decoded) == n {
			break
		}
	}
	return decoded, nil
}

// FindAllStringSubmatchIndex is the string form of FindAllSubmatchIndex.
func (r *LocaleByteRegexp) FindAllStringSubmatchIndex(src string, n int) ([][]int, error) {
	return r.FindAllSubmatchIndex([]byte(src), n)
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
