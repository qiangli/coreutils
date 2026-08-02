// Package tzenv resolves the POSIX TZ environment variable to a
// *time.Location, per POSIX XBD 8.3 (Other Environment Variables, TZ):
//
//   - TZ unset: the system's local time zone (time.Local).
//   - TZ set but empty: UTC (matching glibc's tzset).
//   - ":name" or a value that is not a valid POSIX TZ expansion
//     (e.g. "America/New_York", "UTC"): looked up as an IANA zone via
//     time.LoadLocation; on failure UTC is used, mirroring glibc's
//     "default rules" fallback.
//   - "std offset [dst [offset] [,start[/time],end[/time]]]" (e.g.
//     "EST5", "EST5EDT,M3.2.0,M11.1.0", "<+0530>-5:30"): the full
//     POSIX expansion. Rule evaluation — including the tzcode default
//     US rules when the rule part is omitted — is delegated to the
//     standard library by synthesizing a minimal TZif v2 blob whose
//     extend footer carries the spec verbatim; package time's own
//     tzset interprets it for every practical instant. If the spec's
//     tail is malformed the location degrades to the fixed std zone
//     rather than erroring, matching tzcode.
//
// Resolution never fails: date/touch-family tools always have a
// location to format in. Tools receive the environment from their
// RunContext (never os.Environ) and pass it here.
package tzenv

import (
	"encoding/binary"
	"strings"
	"time"
)

// Location resolves TZ from env (os.Environ shape, "KEY=VALUE"; the
// last assignment wins, matching tool.RunContext.Getenv).
func Location(env []string) *time.Location {
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], "TZ="); ok {
			return FromValue(v)
		}
	}
	return time.Local
}

// FromValue resolves a set TZ value (possibly empty) to a location.
func FromValue(tz string) *time.Location {
	if tz == "" {
		return time.UTC
	}
	if name, ok := strings.CutPrefix(tz, ":"); ok {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
		return time.UTC
	}
	if std, stdOff, ok := posixStdZone(tz); ok {
		if loc, err := time.LoadLocationFromTZData(tz, tzif(tz, std, stdOff)); err == nil {
			return loc
		}
		return time.FixedZone(std, stdOff)
	}
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	return time.UTC
}

// posixStdZone reports whether tz begins with a valid POSIX
// "std offset" prefix — the discriminator between the POSIX expansion
// and an implementation-defined (IANA) name — and returns the std
// designation and its offset in seconds east of UTC (POSIX offsets
// are west-positive, hence the negation).
func posixStdZone(tz string) (name string, offset int, ok bool) {
	name, rest, ok := tzName(tz)
	if !ok {
		return "", 0, false
	}
	west, _, ok := tzOffset(rest)
	if !ok {
		return "", 0, false
	}
	return name, -west, true
}

// tzName parses the leading std/dst designation: either an unquoted
// run of at least three characters that are not digits, ',', '-' or
// '+', or an angle-bracket-quoted form (POSIX.1-2008).
func tzName(s string) (name, rest string, ok bool) {
	if s == "" {
		return "", "", false
	}
	if s[0] == '<' {
		if i := strings.IndexByte(s, '>'); i > 1 {
			return s[1:i], s[i+1:], true
		}
		return "", "", false
	}
	i := 0
	for i < len(s) && !strings.ContainsRune("0123456789,-+", rune(s[i])) {
		i++
	}
	if i < 3 {
		return "", "", false
	}
	return s[:i], s[i:], true
}

// tzOffset parses [+-]hh[:mm[:ss]], returning seconds (west positive,
// as written). Hour range follows tzcode (0..167) rather than POSIX's
// 0..24, matching package time's tzset.
func tzOffset(s string) (seconds int, rest string, ok bool) {
	neg := false
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	h, s, ok := tzNum(s, 167)
	if !ok {
		return 0, "", false
	}
	seconds = h * 3600
	for _, scale := range []int{60, 1} {
		if len(s) == 0 || s[0] != ':' {
			break
		}
		var n int
		n, s, ok = tzNum(s[1:], 59)
		if !ok {
			return 0, "", false
		}
		seconds += n * scale
	}
	if neg {
		seconds = -seconds
	}
	return seconds, s, true
}

func tzNum(s string, max int) (n int, rest string, ok bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' && i < 3 {
		n = n*10 + int(s[i]-'0')
		i++
	}
	if i == 0 || n > max {
		return 0, "", false
	}
	return n, s[i:], true
}

// tzif synthesizes a minimal TZif v2 blob: one zone record (the std
// fallback), one transition in the deep past, and spec as the extend
// footer — so package time's lookup routes every practical instant
// through its own POSIX-TZ interpreter.
func tzif(spec, std string, stdOff int) []byte {
	abbr := append([]byte(std), 0)
	var b []byte
	hdr := func(timecnt uint32) {
		b = append(b, "TZif2"...)
		b = append(b, make([]byte, 15)...)
		for _, n := range []uint32{0, 0, 0, timecnt, 1, uint32(len(abbr))} {
			b = binary.BigEndian.AppendUint32(b, n)
		}
	}
	zone := func() {
		b = binary.BigEndian.AppendUint32(b, uint32(int32(stdOff)))
		b = append(b, 0, 0) // isdst, abbrev index
		b = append(b, abbr...)
	}
	hdr(0) // v1 block: zone record only (skipped by v2-aware readers)
	zone()
	hdr(1) // v2 block: 64-bit transition far before any practical time
	when := int64(-1) << 59
	b = binary.BigEndian.AppendUint64(b, uint64(when))
	b = append(b, 0)
	zone()
	b = append(b, '\n')
	b = append(b, spec...)
	b = append(b, '\n')
	return b
}
