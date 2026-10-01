package localedef

import (
	"fmt"
	"strings"
)

// DefaultCharmap returns a fresh ASCII mapping for the POSIX portable
// character names. Explicit -f maps replace this implementation default.
func DefaultCharmap() *Charmap {
	m := &Charmap{CodeSet: "ASCII", MinBytes: 1, MaxBytes: 1, Symbols: make(map[string][]byte)}
	add := func(name string, b byte) { m.Symbols[name] = []byte{b} }
	for b := 0; b < 128; b++ {
		add(fmt.Sprintf("U%04X", b), byte(b))
	}
	for b := byte('A'); b <= 'Z'; b++ {
		add(string(b), b)
	}
	for b := byte('a'); b <= 'z'; b++ {
		add(string(b), b)
	}
	for i, name := range strings.Fields("zero one two three four five six seven eight nine") {
		add(name, byte('0'+i))
	}
	for i, name := range strings.Fields("NUL SOH STX ETX EOT ENQ ACK alert backspace tab newline vertical-tab form-feed carriage-return SO SI DLE DC1 DC2 DC3 DC4 NAK SYN ETB CAN EM SUB ESC IS4 IS3 IS2 IS1") {
		add(name, byte(i))
	}
	punctuation := map[string]byte{
		"space": ' ', "exclamation-mark": '!', "quotation-mark": '"', "number-sign": '#', "dollar-sign": '$', "percent-sign": '%', "ampersand": '&', "apostrophe": '\'',
		"left-parenthesis": '(', "right-parenthesis": ')', "asterisk": '*', "plus-sign": '+', "comma": ',', "hyphen-minus": '-', "period": '.', "slash": '/',
		"colon": ':', "semicolon": ';', "less-than-sign": '<', "equals-sign": '=', "greater-than-sign": '>', "question-mark": '?', "commercial-at": '@',
		"left-square-bracket": '[', "backslash": '\\', "right-square-bracket": ']', "circumflex": '^', "underscore": '_', "grave-accent": '`',
		"left-curly-bracket": '{', "vertical-line": '|', "right-curly-bracket": '}', "tilde": '~', "DEL": 127,
	}
	for name, b := range punctuation {
		add(name, b)
	}
	return m
}
