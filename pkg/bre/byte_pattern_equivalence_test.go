// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package bre

import (
	"reflect"
	"testing"
)

// TestBytePatternSnapshotCopiesEveryField pins the invariant that broke when
// the equivalence table was added: a new bytePatternTables field must reach
// every grammar's compiler. snapshot is the single copy site, so a field it
// forgets is caught here rather than in one grammar's runtime behavior.
func TestBytePatternSnapshotCopiesEveryField(t *testing.T) {
	input := syntheticBytePatternTables()
	input.dotAll = true
	input.multi = true
	got := input.snapshot()
	if !reflect.DeepEqual(got, input) {
		t.Fatal("snapshot dropped a bytePatternTables field")
	}
	// The class map must be a copy, not the caller's map.
	got.classes["word"] = [256]bool{}
	if reflect.DeepEqual(got.classes["word"], input.classes["word"]) {
		t.Fatal("snapshot aliased the caller's class map")
	}
}

// TestLocaleByteEquivalenceIsGrammarIndependent pins POSIX XBD 9.3.5: an
// equivalence class and a collating element mean the same thing in a BRE and
// in an ERE. The ERE compiler previously dropped the equivalence table, so
// `[[=a=]]` compiled clean and matched nothing at all — not even 'a'.
func TestLocaleByteEquivalenceIsGrammarIndependent(t *testing.T) {
	tables, err := SnapshotLocaleByteTables(fakeByteEquivalence{newFakeByteCtype()})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		pattern string
		input   byte
		want    bool
	}{
		{`[[=a=]]`, 'a', true},   // reflexive: the class always contains its own member
		{`[[=a=]]`, 0xe4, true},  // provider-supplied equivalent
		{`[[=a=]]`, 'b', false},  // unrelated byte
		{`[[=b=]]`, 'b', true},   // byte with no provider-supplied equivalents
		{`[[=b=]]`, 0xe4, false}, // and nothing else joins it
		{`[[.a.]]`, 'a', true},   // collating element is the literal byte
		{`[[.a.]]`, 0xe4, false}, // a collating element is NOT an equivalence class
	} {
		for _, syntax := range []struct {
			name string
			kind ByteRegexpSyntax
		}{{"BRE", ByteRegexpBRE}, {"ERE", ByteRegexpERE}} {
			re, err := CompileLocaleByteRegexpTables([]byte(tc.pattern), tables, ByteRegexpOptions{Syntax: syntax.kind})
			if err != nil {
				t.Fatalf("%s compile %q: %v", syntax.name, tc.pattern, err)
			}
			got, err := re.MatchString(string([]byte{tc.input}))
			if err != nil {
				t.Fatalf("%s match %q: %v", syntax.name, tc.pattern, err)
			}
			if got != tc.want {
				t.Errorf("%s %q against %#02x = %v, want %v", syntax.name, tc.pattern, tc.input, got, tc.want)
			}
		}
	}
}
