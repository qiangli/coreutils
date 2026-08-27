package lscmd

import (
	"strconv"
	"strings"
	"time"
)

// resolveTZLocation determines the *time.Location that -l timestamps
// should be rendered in, honoring the invocation's TZ environment
// variable the same way POSIX/GNU ls does.
//
// Go's time.LoadLocation only recognizes IANA zoneinfo names (including
// database entries that happen to look like POSIX strings, e.g.
// "EST5EDT"). It does NOT parse the general POSIX TZ rule syntax
// (IEEE Std 1003.1 "TZ" in Base Definitions 8.3), so a bare offset form
// such as "EST5" or "UTC0" fails to load and Go silently falls back to
// UTC — silently discarding the requested offset. parsePosixTZ below
// fills that gap for the bounded offset-only "std offset" form. More
// complex forms are handled by the host zoneinfo database when available;
// they are not silently approximated.
func resolveTZLocation(tz string) *time.Location {
	if tz == "" {
		return time.Local
	}
	name := strings.TrimPrefix(tz, ":")
	if loc, err := time.LoadLocation(name); err == nil {
		return loc
	}
	if loc, ok := parsePosixTZ(tz); ok {
		return loc
	}
	// Matches glibc's behavior for a TZ value it cannot parse at all:
	// fall back to UTC rather than silently keeping a stale location.
	return time.UTC
}

// parsePosixTZ parses the offset-only "std offset" form of a POSIX TZ string
// (IEEE Std 1003.1 Base Definitions 8.3) into a fixed-offset
// time.Location. A DST suffix is deliberately rejected: accepting it while
// returning a fixed standard-time offset would give incorrect timestamps.
func parsePosixTZ(tz string) (*time.Location, bool) {
	name, i, ok := parseTZName(tz, 0)
	if !ok {
		return nil, false
	}
	offset, i, ok := parseTZOffset(tz, i)
	if !ok {
		return nil, false
	}
	if i != len(tz) {
		return nil, false
	}
	// POSIX offset is the value ADDED to local time to get UTC (i.e.
	// positive west of Greenwich) — the opposite sign convention from
	// time.FixedZone's "seconds east of UTC".
	return time.FixedZone(name, -offset), true
}

// parseTZName parses the std/dst name field: either a bracketed
// <...> form (letters, digits, '+', '-') or a bare run of three or
// more letters.
func parseTZName(s string, i int) (string, int, bool) {
	if i >= len(s) {
		return "", i, false
	}
	if s[i] == '<' {
		j := strings.IndexByte(s[i:], '>')
		if j < 0 {
			return "", i, false
		}
		end := i + j
		name := s[i+1 : end]
		if name == "" {
			return "", i, false
		}
		return name, end + 1, true
	}
	start := i
	for i < len(s) && isTZNameByte(s[i]) {
		i++
	}
	if i-start < 3 {
		return "", start, false
	}
	return s[start:i], i, true
}

func isTZNameByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

// parseTZOffset parses "[+-]hh[:mm[:ss]]" and returns the offset in
// seconds.
func parseTZOffset(s string, i int) (int, int, bool) {
	sign := 1
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		if s[i] == '-' {
			sign = -1
		}
		i++
	}
	hh, i, ok := parseTZNum(s, i)
	if !ok {
		return 0, i, false
	}
	total := hh * 3600
	if i < len(s) && s[i] == ':' {
		mm, j, ok := parseTZNum(s, i+1)
		if !ok {
			return 0, i, false
		}
		total += mm * 60
		i = j
		if i < len(s) && s[i] == ':' {
			ss, k, ok := parseTZNum(s, i+1)
			if !ok {
				return 0, i, false
			}
			total += ss
			i = k
		}
	}
	return sign * total, i, true
}

func parseTZNum(s string, i int) (int, int, bool) {
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == start {
		return 0, start, false
	}
	n, err := strconv.Atoi(s[start:i])
	if err != nil {
		return 0, start, false
	}
	return n, i, true
}
