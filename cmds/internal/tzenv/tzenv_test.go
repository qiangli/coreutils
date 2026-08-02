package tzenv

import (
	"testing"
	"time"
)

// Fixed probe instants: one in northern-hemisphere summer, one in
// winter, so DST rules are exercised on both sides.
var (
	summer = time.Unix(1755000000, 0) // 2025-08-12T12:00:00Z
	winter = time.Unix(1735689600, 0) // 2025-01-01T00:00:00Z
)

func zoneAt(loc *time.Location, at time.Time) (string, int) {
	return at.In(loc).Zone()
}

func TestFromValuePosixSpecs(t *testing.T) {
	cases := []struct {
		tz                     string
		summerName, winterName string
		summerOff, winterOff   int
	}{
		{"UTC0", "UTC", "UTC", 0, 0},
		{"GMT0", "GMT", "GMT", 0, 0},
		{"EST5", "EST", "EST", -18000, -18000},
		// No rule part: tzcode default US rules apply.
		{"EST5EDT", "EDT", "EST", -14400, -18000},
		{"EST5EDT,M3.2.0,M11.1.0", "EDT", "EST", -14400, -18000},
		{"PST8PDT,M3.2.0,M11.1.0", "PDT", "PST", -25200, -28800},
		// East-of-Greenwich (negative POSIX offset) and sub-hour.
		{"CET-1CEST,M3.5.0,M10.5.0/3", "CEST", "CET", 7200, 3600},
		{"<+0530>-5:30", "+0530", "+0530", 19800, 19800},
		// Julian-day rules (southern hemisphere shape).
		{"NZST-12NZDT,M9.5.0,M4.1.0/3", "NZST", "NZDT", 43200, 46800},
	}
	for _, c := range cases {
		loc := FromValue(c.tz)
		sn, so := zoneAt(loc, summer)
		wn, wo := zoneAt(loc, winter)
		if sn != c.summerName || so != c.summerOff || wn != c.winterName || wo != c.winterOff {
			t.Errorf("TZ=%q: summer=%s/%d winter=%s/%d, want %s/%d %s/%d",
				c.tz, sn, so, wn, wo, c.summerName, c.summerOff, c.winterName, c.winterOff)
		}
	}
}

func TestFromValueSpecialAndFallback(t *testing.T) {
	// Empty value: UTC (glibc behavior; POSIX leaves it unspecified).
	if loc := FromValue(""); loc != time.UTC {
		t.Errorf("TZ=\"\" = %v, want UTC", loc)
	}
	// Not a POSIX expansion and not a known zone: UTC default rules.
	for _, tz := range []string{"bogus", "no/such/zone", "X1", "<unterminated"} {
		if n, off := zoneAt(FromValue(tz), winter); off != 0 || n != "UTC" {
			t.Errorf("TZ=%q = %s/%d, want UTC/0", tz, n, off)
		}
	}
	// Malformed rule tail degrades to the fixed std zone, not an error.
	if n, off := zoneAt(FromValue("EST5EDT,bogus"), winter); n != "EST" || off != -18000 {
		t.Errorf("TZ=\"EST5EDT,bogus\" = %s/%d, want EST/-18000", n, off)
	}
}

func TestFromValueIANA(t *testing.T) {
	if _, err := time.LoadLocation("America/New_York"); err != nil {
		t.Skip("no IANA tzdata on this host")
	}
	for _, tz := range []string{"America/New_York", ":America/New_York"} {
		loc := FromValue(tz)
		if n, off := zoneAt(loc, summer); n != "EDT" || off != -14400 {
			t.Errorf("TZ=%q summer = %s/%d, want EDT/-14400", tz, n, off)
		}
	}
}

func TestLocationEnvLookup(t *testing.T) {
	if loc := Location(nil); loc != time.Local {
		t.Errorf("no TZ = %v, want Local", loc)
	}
	if loc := Location([]string{"PATH=/bin"}); loc != time.Local {
		t.Errorf("no TZ = %v, want Local", loc)
	}
	// Last assignment wins, matching RunContext.Getenv.
	loc := Location([]string{"TZ=UTC0", "TZ=EST5"})
	if n, _ := zoneAt(loc, winter); n != "EST" {
		t.Errorf("last TZ assignment: zone %q, want EST", n)
	}
}
