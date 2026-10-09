package bre

import "testing"

// POSIX requires a negated bracket expression to honor the locale's LC_COLLATE
// equivalence classes: when a ≡ 0xE4 in a locale, [^[=a=]] must NOT match 0xE4,
// whereas under C/POSIX collation (where the a equivalence class is just {a})
// the same expression MUST match 0xE4. A pager/search built on these tables
// therefore produces observably different results across locales — the behavior
// relied on by LC_COLLATE-sensitive regular-expression searches. The positive
// form [[=a=]] is covered by TestCompileLocaleByteRegexpSnapshotsEquivalenceClasses;
// this pins the negated form, which inverts the class membership.
func TestNegatedEquivalenceClassIsLocaleSensitive(t *testing.T) {
	german, err := SnapshotLocaleByteTables(fakeByteEquivalence{newFakeByteCtype()})
	if err != nil {
		t.Fatalf("german snapshot: %v", err)
	}
	c, err := SnapshotLocaleByteTables(newFakeByteCtype())
	if err != nil {
		t.Fatalf("c snapshot: %v", err)
	}

	gre, err := CompileLocaleByteRegexpTables([]byte(`[^[=a=]]`), german, ByteRegexpOptions{})
	if err != nil {
		t.Fatalf("german compile: %v", err)
	}
	cre, err := CompileLocaleByteRegexpTables([]byte(`[^[=a=]]`), c, ByteRegexpOptions{})
	if err != nil {
		t.Fatalf("c compile: %v", err)
	}

	gm, err := gre.MatchString(string([]byte{0xe4}))
	if err != nil {
		t.Fatalf("german match: %v", err)
	}
	cm, err := cre.MatchString(string([]byte{0xe4}))
	if err != nil {
		t.Fatalf("c match: %v", err)
	}

	if gm {
		t.Errorf("[^[=a=]] matched 0xE4 under a locale where a≡0xE4; 0xE4 is inside the class")
	}
	if !cm {
		t.Errorf("[^[=a=]] did not match 0xE4 under C collation; 0xE4 is outside the {a} class")
	}
	if gm == cm {
		t.Fatalf("negated equivalence class was not locale-sensitive: german=%v c=%v", gm, cm)
	}
}

// Companion check for the LC_CTYPE half: a negated character class [^[:alpha:]]
// must not match a byte the locale classifies as alphabetic. The synthetic
// provider classifies the high byte 0xE9 (é) as a single-byte locale letter
// (see syntheticBytePatternTables), so [^[:alpha:]] must reject it while still
// matching a non-alpha byte such as '0'.
func TestNegatedCtypeClassHonorsLocaleAlpha(t *testing.T) {
	re, err := CompileLocaleByteRegexp([]byte(`[^[:alpha:]]`), newFakeByteCtype(), ByteRegexpOptions{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if m, err := re.MatchString(string([]byte{0xe9})); err != nil || m {
		t.Fatalf("[^[:alpha:]] matched locale-alpha byte 0xE9: matched=%v err=%v", m, err)
	}
	if m, err := re.MatchString("0"); err != nil || !m {
		t.Fatalf("[^[:alpha:]] did not match non-alpha '0': matched=%v err=%v", m, err)
	}
}
