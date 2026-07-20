package sortcmd

// Ordering comparators, all C-locale / byte-oriented:
//
//   - numCompare implements GNU strnumcmp semantics for -n.
//   - generalNumCompare implements GNU general-numeric -g (float-compatible).
//   - humanCompare implements -h: sign first, then SI suffix order.
//   - monthCompare implements -M: three-letter month abbreviation order.
//   - versionCompare implements -V: natural comparison of version strings.
//   - textKeyCompare composes -d, -f, and -i for bytewise text keys.

import (
	"math"
	"strconv"
	"strings"
)

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
	return compareFraction(fa, fb)
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

func textKeyCompare(a, b string, o keyOpts) int {
	if !o.dict && !o.ignoreNP && !o.fold {
		return strings.Compare(a, b)
	}
	return strings.Compare(normalizeTextKey(a, o), normalizeTextKey(b, o))
}

func normalizeTextKey(s string, o keyOpts) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if o.dict && !(c == ' ' || c == '\t' || (c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			continue
		}
		if o.ignoreNP && (c < 32 || c > 126) {
			continue
		}
		if o.fold {
			c = upperByte(c)
		}
		b = append(b, c)
	}
	return string(b)
}

var months = map[string]int{
	"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4,
	"MAY": 5, "JUN": 6, "JUL": 7, "AUG": 8,
	"SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
}

func monthOrder(a, b string) int {
	ua := strings.ToUpper(strings.TrimLeft(a, " \t"))
	ub := strings.ToUpper(strings.TrimLeft(b, " \t"))
	ma, mb := 0, 0
	if len(ua) >= 3 {
		ma = months[ua[:3]]
	}
	if len(ub) >= 3 {
		mb = months[ub[:3]]
	}
	return cmpInt(ma, mb)
}

func monthCompare(a, b string) int {
	if d := monthOrder(a, b); d != 0 {
		return d
	}
	return strings.Compare(strings.TrimLeft(a, " \t"), strings.TrimLeft(b, " \t"))
}

func compareFraction(a, b string) int {
	na, nb := len(a), len(b)
	n := na
	if nb < n {
		n = nb
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	// Missing trailing fractional digits are zeroes: .2 and .20
	// represent the same value, while .2 is greater than .19.
	for na < nb {
		if b[na] != '0' {
			return -1
		}
		na++
	}
	for nb < na {
		if a[nb] != '0' {
			return 1
		}
		nb++
	}
	return 0
}

func filevercmp(a, b string) int {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		aIsNum := a[ai] >= '0' && a[ai] <= '9'
		bIsNum := b[bi] >= '0' && b[bi] <= '9'
		if aIsNum && bIsNum {
			for ai < len(a) && a[ai] == '0' {
				ai++
			}
			for bi < len(b) && b[bi] == '0' {
				bi++
			}
			aStart, bStart := ai, bi
			for ai < len(a) && a[ai] >= '0' && a[ai] <= '9' {
				ai++
			}
			for bi < len(b) && b[bi] >= '0' && b[bi] <= '9' {
				bi++
			}
			alen, blen := ai-aStart, bi-bStart
			if alen < blen {
				return -1
			}
			if alen > blen {
				return 1
			}
			if d := strings.Compare(a[aStart:ai], b[bStart:bi]); d != 0 {
				return d
			}
			continue
		}
		if a[ai] != b[bi] {
			if a[ai] < b[bi] {
				return -1
			}
			return 1
		}
		ai++
		bi++
	}
	if ai < len(a) {
		return 1
	}
	if bi < len(b) {
		return -1
	}
	return 0
}

func versionCompare(a, b string) int {
	return filevercmp(strings.TrimLeft(a, " \t"), strings.TrimLeft(b, " \t"))
}

func generalNumParse(s string) (float64, bool) {
	t := strings.TrimLeft(s, " \t")
	if t == "" {
		return 0, false
	}
	if len(t) >= 3 && (t[:3] == "nan" || t[:3] == "NaN") {
		return math.NaN(), true
	}
	if len(t) >= 3 && (t[:3] == "inf" || t[:3] == "Inf" || t[:3] == "INF") {
		return math.Inf(1), true
	}
	if len(t) >= 4 && (t[:4] == "+nan" || t[:4] == "+NaN" || t[:4] == "-nan" || t[:4] == "-NaN") {
		return math.NaN(), true
	}
	if len(t) >= 4 && (t[:4] == "+inf" || t[:4] == "+Inf" || t[:4] == "+INF") {
		return math.Inf(1), true
	}
	if len(t) >= 4 && (t[:4] == "-inf" || t[:4] == "-Inf" || t[:4] == "-INF") {
		return math.Inf(-1), true
	}
	end, ok := generalNumPrefix(t)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(t[:end], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func generalNumPrefix(s string) (int, bool) {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits := 0
	for i < len(s) && isDigit(s[i]) {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isDigit(s[i]) {
			i++
			digits++
		}
	}
	if digits == 0 {
		return 0, false
	}
	end := i
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		expStart := j
		for j < len(s) && isDigit(s[j]) {
			j++
		}
		if j > expStart {
			end = j
		}
	}
	return end, true
}

func generalNumCompare(a, b string) int {
	va, oka := generalNumParse(a)
	vb, okb := generalNumParse(b)
	if !oka && !okb {
		return 0
	}
	if !oka {
		return -1
	}
	if !okb {
		return 1
	}
	if math.IsNaN(va) && math.IsNaN(vb) {
		return 0
	}
	if math.IsNaN(va) {
		return 1
	}
	if math.IsNaN(vb) {
		return -1
	}
	if va < vb {
		return -1
	}
	if va > vb {
		return 1
	}
	return 0
}
