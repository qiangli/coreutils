package trcmd

import (
	"fmt"
	"sort"

	"github.com/qiangli/coreutils/pkg/collate"
	"github.com/qiangli/coreutils/pkg/locale"
)

// collateProvider is the complete, snapshot-only LC_COLLATE surface tr needs.
// Production uses pkg/collate's bounded ISO-8859-1 provider; tests inject
// deterministic tables and lifecycle failures.
type collateProvider interface {
	Equivalents(byte) ([]byte, error)
	EquivalenceClasses() ([]bool, error)
	CollationWeights() ([]byte, error)
	CollatingElements() ([]bool, error)
	Close() error
}

type collateOpener func(string) (collateProvider, error)

func prodCollateOpener(name string) (collateProvider, error) { return collate.Open(name) }

type collationTables struct {
	equivalent [256][]rune
	valid      [256]bool
	weight     [256]byte
	element    [256]bool
}

func cCollationTables() *collationTables {
	t := &collationTables{}
	for i := 0; i < 256; i++ {
		t.equivalent[i] = []rune{rune(i)}
		t.valid[i] = true
		t.weight[i] = byte(i)
		t.element[i] = true
	}
	return t
}

// snapshotCollation copies every provider-owned result before Close. Snapshot
// errors take precedence over Close errors, matching the other locale users.
func snapshotCollation(p collateProvider) (*collationTables, error) {
	t := &collationTables{}
	var snapshotErr error

	valid, err := p.EquivalenceClasses()
	if err != nil || len(valid) != 256 {
		snapshotErr = fmt.Errorf("invalid equivalence-class validity: len=%d err=%v", len(valid), err)
	} else {
		copy(t.valid[:], valid)
	}
	if snapshotErr == nil {
		weights, err := p.CollationWeights()
		if err != nil || len(weights) != 256 {
			snapshotErr = fmt.Errorf("invalid collation weights: len=%d err=%v", len(weights), err)
		} else {
			copy(t.weight[:], weights)
		}
	}
	if snapshotErr == nil {
		elements, err := p.CollatingElements()
		if err != nil || len(elements) != 256 {
			snapshotErr = fmt.Errorf("invalid collating elements: len=%d err=%v", len(elements), err)
		} else {
			copy(t.element[:], elements)
		}
	}
	if snapshotErr == nil {
		for source := 0; source < 256; source++ {
			members, err := p.Equivalents(byte(source))
			if err != nil {
				snapshotErr = fmt.Errorf("equivalence class for byte %#02x: %w", source, err)
				break
			}
			if !t.valid[source] {
				if len(members) != 0 {
					snapshotErr = fmt.Errorf("invalid collating byte %#02x has a non-empty equivalence class", source)
				}
				continue
			}
			seenSelf := false
			for _, member := range members {
				if !t.element[member] {
					snapshotErr = fmt.Errorf("equivalence class for byte %#02x contains invalid collating byte %#02x", source, member)
					break
				}
				if member == byte(source) {
					seenSelf = true
				}
				t.equivalent[source] = append(t.equivalent[source], rune(member))
			}
			if snapshotErr != nil {
				break
			}
			if !t.element[source] || !seenSelf {
				snapshotErr = fmt.Errorf("equivalence class for byte %#02x is not reflexive", source)
				break
			}
		}
	}

	closeErr := p.Close()
	if snapshotErr != nil {
		return nil, snapshotErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return t, nil
}

func openCollationTables(env []string, charModel *charTables, opener collateOpener) (*collationTables, string, error) {
	name := locale.Resolve(env, locale.Collate)
	if isCPOSIX(name) {
		return cCollationTables(), name, nil
	}
	if charModel.multibyte {
		return nil, name, fmt.Errorf("single-byte LC_COLLATE cannot be combined with a multi-byte LC_CTYPE")
	}
	p, err := opener(name)
	if err != nil {
		return nil, name, err
	}
	t, err := snapshotCollation(p)
	return t, name, err
}

func (t *collationTables) equivalents(c byte) []rune {
	if !t.valid[c] {
		return []rune{rune(c)}
	}
	return append([]rune(nil), t.equivalent[c]...)
}

// rangeChars expands a single-byte range in ascending collation order.
// Equal weights are made deterministic by byte value; equivalence expansion
// remains the separate [=c=] construct.
func (t *collationTables) rangeChars(lo, hi byte) ([]rune, bool) {
	if !t.element[lo] || !t.element[hi] || t.weight[hi] < t.weight[lo] {
		return nil, false
	}
	out := make([]byte, 0, 256)
	for c := 0; c < 256; c++ {
		if !t.element[c] {
			continue
		}
		if w := t.weight[c]; w >= t.weight[lo] && w <= t.weight[hi] {
			out = append(out, byte(c))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		wi, wj := t.weight[out[i]], t.weight[out[j]]
		if wi != wj {
			return wi < wj
		}
		return out[i] < out[j]
	})
	runes := make([]rune, len(out))
	for i, c := range out {
		runes[i] = rune(c)
	}
	return runes, true
}

// orderCharacters orders an LC_CTYPE-derived character set using LC_COLLATE.
// CollatingElements is intentionally not consulted here: it describes which
// bytes may participate in collation constructs such as ranges, not which
// bytes are characters in the selected single-byte LC_CTYPE.
func (t *collationTables) orderCharacters(chars []byte) []rune {
	out := append([]byte(nil), chars...)
	sort.Slice(out, func(i, j int) bool {
		wi, wj := t.weight[out[i]], t.weight[out[j]]
		if wi != wj {
			return wi < wj
		}
		return out[i] < out[j]
	})
	runes := make([]rune, len(out))
	for i, c := range out {
		runes[i] = rune(c)
	}
	return runes
}
