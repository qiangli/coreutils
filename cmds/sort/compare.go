package sortcmd

// Ordering comparators, all C-locale / byte-oriented:
//
//   - numCompare implements GNU strnumcmp semantics for -n: arbitrary-
//     precision decimal string comparison (no float conversion), with
//     leading blanks skipped, an optional '-' sign (no '+'), and any
//     text without digits comparing equal to zero.
//   - humanCompare implements -h: sign first, then SI suffix order
//     (K=k < M < G < T < P < E < Z < Y < R < Q), then numeric value.
//   - foldCompare implements -f: bytewise after ASCII upcasing.

import "strings"

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// parseNumParts splits the longest numeric prefix of s (after leading
// blanks) into sign, integer digits (leading zeros trimmed) and
// fraction digits (trailing zeros trimmed). A zero or absent number
// reports sign 0.
func parseNumParts(s string) (sign int, ip, fp string) {
	i := 0
	for i < len(s) && isBlank(s[i]) {
		i++
	}
	neg := false
	if i < len(s) && s[i] == '-' {
		neg = true
		i++
	}
	j := i
	for j < len(s) && isDigit(s[j]) {
		j++
	}
	ip = strings.TrimLeft(s[i:j], "0")
	if j < len(s) && s[j] == '.' {
		k := j + 1
		for k < len(s) && isDigit(s[k]) {
			k++
		}
		fp = strings.TrimRight(s[j+1:k], "0")
	}
	if ip == "" && fp == "" {
		return 0, "", ""
	}
	if neg {
		return -1, ip, fp
	}
	return 1, ip, fp
}

// magCompare compares two non-negative decimal magnitudes given as
// trimmed integer and fraction digit strings.
func magCompare(ia, fa, ib, fb string) int {
	if d := cmpInt(len(ia), len(ib)); d != 0 {
		return d
	}
	if d := strings.Compare(ia, ib); d != 0 {
		return d
	}
	return strings.Compare(fa, fb)
}

func numCompare(a, b string) int {
	sa, ia, fa := parseNumParts(a)
	sb, ib, fb := parseNumParts(b)
	if sa != sb {
		return cmpInt(sa, sb)
	}
	m := magCompare(ia, fa, ib, fb)
	if sa < 0 {
		return -m
	}
	return m
}

// siOrder mirrors GNU sort's unit_order table ('k' is the only
// lowercase entry).
var siOrder = map[byte]int{
	'K': 1, 'k': 1, 'M': 2, 'G': 3, 'T': 4, 'P': 5,
	'E': 6, 'Z': 7, 'Y': 8, 'R': 9, 'Q': 10,
}

// unitOrder mirrors GNU find_unit_order: the unit is the first byte
// after the (optionally signed, optionally fractional) number, negated
// for negative numbers.
func unitOrder(s string) int {
	i := 0
	neg := false
	if i < len(s) && s[i] == '-' {
		neg = true
		i++
	}
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isDigit(s[i]) {
			i++
		}
	}
	o := 0
	if i < len(s) {
		o = siOrder[s[i]]
	}
	if neg {
		o = -o
	}
	return o
}

func humanCompare(a, b string) int {
	ta := strings.TrimLeft(a, " \t")
	tb := strings.TrimLeft(b, " \t")
	if d := cmpInt(unitOrder(ta), unitOrder(tb)); d != 0 {
		return d
	}
	return numCompare(ta, tb)
}

func upperByte(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

func foldCompare(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		ca, cb := upperByte(a[i]), upperByte(b[i])
		if ca != cb {
			if ca < cb {
				return -1
			}
			return 1
		}
	}
	return cmpInt(len(a), len(b))
}
