package bre

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// byteTokenCodec is the byte-to-rune substrate for a future locale-aware
// regexp path. It is deliberately disconnected from Regexp and the public API.
//
// Each non-newline byte becomes exactly two runes whose RE2 word
// classification is homogeneous: word bytes use two ASCII word runes and
// non-word bytes use two private-use runes. This prevents a false word
// boundary inside a token. Newline remains a literal newline so a later
// pattern transform can preserve dot and line-anchor behavior.
type byteTokenCodec struct {
	tokens  [256]string
	reverse map[string]byte
}

const byteTokenWordAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_"

func newByteTokenCodec(wordBytes [256]bool) byteTokenCodec {
	c := byteTokenCodec{reverse: make(map[string]byte, 256)}
	wordRunes := []rune(byteTokenWordAlphabet)
	for i := 0; i < 256; i++ {
		var token string
		if wordBytes[byte(i)] {
			token = string([]rune{
				wordRunes[i/len(wordRunes)],
				wordRunes[i%len(wordRunes)],
			})
		} else {
			// Private-use runes are non-word under RE2's ASCII definition.
			// The disjoint ranges make every pair unique without introducing
			// punctuation or regexp metacharacter concerns into the subject.
			token = string([]rune{rune(0xE000 + i/16), rune(0xE100 + i%16)})
		}
		c.tokens[i] = token
		c.reverse[token] = byte(i)
	}
	return c
}

// encodedByteSubject records both directions of the boundary mapping. All
// offsets are byte offsets: raw offsets index the original byte slice and
// encoded offsets use the convention returned by regexp match methods.
type encodedByteSubject struct {
	text         string
	rawToEncoded []int
	encodedToRaw map[int]int
}

func (c byteTokenCodec) encodeSubject(raw []byte) encodedByteSubject {
	var b strings.Builder
	// Six bytes is the maximum encoded width (two private-use UTF-8 runes).
	b.Grow(len(raw) * 6)
	rawToEncoded := make([]int, len(raw)+1)
	encodedToRaw := make(map[int]int, len(raw)+1)
	for i, value := range raw {
		rawToEncoded[i] = b.Len()
		encodedToRaw[b.Len()] = i
		if value == '\n' {
			b.WriteByte('\n')
		} else {
			b.WriteString(c.tokens[value])
		}
	}
	rawToEncoded[len(raw)] = b.Len()
	encodedToRaw[b.Len()] = len(raw)
	return encodedByteSubject{
		text:         b.String(),
		rawToEncoded: rawToEncoded,
		encodedToRaw: encodedToRaw,
	}
}

func (s encodedByteSubject) encodedOffset(rawOffset int) (int, error) {
	if rawOffset < 0 || rawOffset >= len(s.rawToEncoded) {
		return 0, fmt.Errorf("raw byte offset %d is outside [0,%d]", rawOffset, len(s.rawToEncoded)-1)
	}
	return s.rawToEncoded[rawOffset], nil
}

func (s encodedByteSubject) rawOffset(encodedOffset int) (int, error) {
	if rawOffset, ok := s.encodedToRaw[encodedOffset]; ok {
		return rawOffset, nil
	}
	return 0, fmt.Errorf("encoded byte offset %d is not a source boundary", encodedOffset)
}

func (c byteTokenCodec) decodeSubject(encoded string) ([]byte, error) {
	decoded := make([]byte, 0, len(encoded))
	for len(encoded) > 0 {
		if encoded[0] == '\n' {
			decoded = append(decoded, '\n')
			encoded = encoded[1:]
			continue
		}
		first, firstSize := utf8.DecodeRuneInString(encoded)
		if first == utf8.RuneError && firstSize == 1 {
			return nil, fmt.Errorf("invalid UTF-8 in byte token")
		}
		if firstSize >= len(encoded) {
			return nil, fmt.Errorf("incomplete byte token")
		}
		second, secondSize := utf8.DecodeRuneInString(encoded[firstSize:])
		if second == utf8.RuneError && secondSize == 1 {
			return nil, fmt.Errorf("invalid UTF-8 in byte token")
		}
		tokenEnd := firstSize + secondSize
		token := encoded[:tokenEnd]
		value, ok := c.reverse[token]
		if !ok {
			return nil, fmt.Errorf("unknown byte token %q", token)
		}
		decoded = append(decoded, value)
		encoded = encoded[tokenEnd:]
	}
	return decoded, nil
}
