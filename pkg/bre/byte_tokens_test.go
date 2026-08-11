package bre

import (
	"bytes"
	"regexp/syntax"
	"testing"
	"unicode/utf8"
)

func alternatingWordBytes() [256]bool {
	var words [256]bool
	for i := range words {
		words[i] = i%2 == 0
	}
	return words
}

func TestByteTokenCodecExhaustiveRoundTripAndHomogeneity(t *testing.T) {
	words := alternatingWordBytes()
	codec := newByteTokenCodec(words)
	raw := make([]byte, 256)
	for i := range raw {
		raw[i] = byte(i)
	}
	encoded := codec.encodeSubject(raw)
	if !utf8.ValidString(encoded.text) {
		t.Fatal("encoded subject is not valid UTF-8")
	}
	got, err := codec.decodeSubject(encoded.text)
	if err != nil {
		t.Fatalf("decodeSubject: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("round trip differs:\n got %v\nwant %v", got, raw)
	}

	seen := make(map[string]byte, 256)
	for i, token := range codec.tokens {
		if prior, ok := seen[token]; ok {
			t.Fatalf("bytes %d and %d share token %q", prior, i, token)
		}
		seen[token] = byte(i)
		runes := []rune(token)
		if len(runes) != 2 {
			t.Fatalf("byte %d token has %d runes, want 2", i, len(runes))
		}
		wantWord := words[byte(i)]
		for j, r := range runes {
			if gotWord := syntax.IsWordChar(r); gotWord != wantWord {
				t.Errorf("byte %d rune %d (%U) word=%v, want %v", i, j, r, gotWord, wantWord)
			}
		}
	}
}

func TestByteTokenCodecAdjacencyHasNoInternalWordBoundary(t *testing.T) {
	var words [256]bool
	words[0x10] = true
	words[0x12] = true
	codec := newByteTokenCodec(words)

	for _, tc := range []struct {
		name  string
		left  byte
		right byte
	}{
		{"word-word", 0x10, 0x12},
		{"word-nonword", 0x10, 0x11},
		{"nonword-word", 0x11, 0x10},
		{"nonword-nonword", 0x11, 0x13},
	} {
		t.Run(tc.name, func(t *testing.T) {
			left := []rune(codec.tokens[tc.left])
			right := []rune(codec.tokens[tc.right])
			if syntax.IsWordChar(left[0]) != syntax.IsWordChar(left[1]) {
				t.Fatal("left token has an internal word boundary")
			}
			if syntax.IsWordChar(right[0]) != syntax.IsWordChar(right[1]) {
				t.Fatal("right token has an internal word boundary")
			}
			gotSeamBoundary := syntax.IsWordChar(left[1]) != syntax.IsWordChar(right[0])
			wantSeamBoundary := words[tc.left] != words[tc.right]
			if gotSeamBoundary != wantSeamBoundary {
				t.Fatalf("token seam boundary=%v, want %v", gotSeamBoundary, wantSeamBoundary)
			}
		})
	}
}

func TestEncodedByteSubjectBoundaryMapAndNewlines(t *testing.T) {
	codec := newByteTokenCodec(alternatingWordBytes())
	empty := codec.encodeSubject(nil)
	if empty.text != "" {
		t.Fatalf("empty encoded text=%q, want empty", empty.text)
	}
	if offset, err := empty.encodedOffset(0); err != nil || offset != 0 {
		t.Fatalf("empty encodedOffset(0)=(%d, %v), want (0, nil)", offset, err)
	}
	if offset, err := empty.rawOffset(0); err != nil || offset != 0 {
		t.Fatalf("empty rawOffset(0)=(%d, %v), want (0, nil)", offset, err)
	}

	raw := []byte{0x00, 0x01, '\n', '\n', 0xfe, 0xff}
	encoded := codec.encodeSubject(raw)
	if got := bytes.Count([]byte(encoded.text), []byte{'\n'}); got != 2 {
		t.Fatalf("encoded newline count=%d, want 2", got)
	}

	for rawOffset := 0; rawOffset <= len(raw); rawOffset++ {
		encodedOffset, err := encoded.encodedOffset(rawOffset)
		if err != nil {
			t.Fatalf("encodedOffset(%d): %v", rawOffset, err)
		}
		gotRawOffset, err := encoded.rawOffset(encodedOffset)
		if err != nil {
			t.Fatalf("rawOffset(%d): %v", encodedOffset, err)
		}
		if gotRawOffset != rawOffset {
			t.Errorf("offset round trip=%d, want %d", gotRawOffset, rawOffset)
		}
	}

	if _, err := encoded.encodedOffset(-1); err == nil {
		t.Error("encodedOffset(-1) succeeded")
	}
	if _, err := encoded.encodedOffset(len(raw) + 1); err == nil {
		t.Error("encodedOffset past end succeeded")
	}
	if _, err := encoded.rawOffset(-1); err == nil {
		t.Error("rawOffset(-1) succeeded")
	}
	if _, err := encoded.rawOffset(len(encoded.text) + 1); err == nil {
		t.Error("rawOffset past end succeeded")
	}

	// Every non-newline token has at least one encoded byte offset between its
	// source boundaries. None of those half-token offsets may be accepted.
	for i, value := range raw {
		if value == '\n' {
			continue
		}
		start := encoded.rawToEncoded[i]
		end := encoded.rawToEncoded[i+1]
		for offset := start + 1; offset < end; offset++ {
			if _, err := encoded.rawOffset(offset); err == nil {
				t.Errorf("rawOffset accepted half-token offset %d for raw byte %d", offset, i)
			}
		}
	}
}

func TestByteTokenCodecRejectsMalformedEncoding(t *testing.T) {
	codec := newByteTokenCodec(alternatingWordBytes())
	token := codec.tokens[0]
	_, firstSize := utf8.DecodeRuneInString(token)
	for name, encoded := range map[string]string{
		"half token":    token[:firstSize],
		"unknown pair":  "!!",
		"invalid UTF-8": string([]byte{0xff, 0xff}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := codec.decodeSubject(encoded); err == nil {
				t.Fatalf("decodeSubject(%q) succeeded", encoded)
			}
		})
	}
}
