package lscmd

import (
	"testing"
	"time"
)

func TestResolveTZLocationPosixOffsetForms(t *testing.T) {
	cases := []struct {
		tz         string
		wantOffset int // seconds east of UTC
	}{
		{"UTC0", 0},
		{"EST5", -5 * 3600},
		{"GMT0", 0},
		{"CST6", -6 * 3600},
		{"WET-1", 1 * 3600}, // POSIX offset sign is inverted from common usage
	}
	ref := time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range cases {
		loc := resolveTZLocation(c.tz)
		_, off := ref.In(loc).Zone()
		if off != c.wantOffset {
			t.Errorf("resolveTZLocation(%q) offset = %d, want %d", c.tz, off, c.wantOffset)
		}
	}
}

func TestResolveTZLocationFallsBackToUTCOnGarbage(t *testing.T) {
	loc := resolveTZLocation("not a real zone !!")
	ref := time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
	if _, off := ref.In(loc).Zone(); off != 0 {
		t.Errorf("resolveTZLocation(garbage) offset = %d, want 0 (UTC fallback)", off)
	}
}

func TestParsePosixTZRejectsUnimplementedDSTSuffix(t *testing.T) {
	if _, ok := parsePosixTZ("XYZ5DST"); ok {
		t.Fatal("parsePosixTZ accepted a DST suffix it cannot implement")
	}
}

func TestResolveTZLocationEmptyUsesLocal(t *testing.T) {
	if resolveTZLocation("") != time.Local {
		t.Errorf("resolveTZLocation(\"\") should be time.Local")
	}
}
