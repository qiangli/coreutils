package timeoutcmd

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"5":    5 * time.Second,
		"5s":   5 * time.Second,
		"2m":   2 * time.Minute,
		"1h":   time.Hour,
		"1d":   24 * time.Hour,
		"0.5":  500 * time.Millisecond,
		"1.5s": 1500 * time.Millisecond,
		"0":    0,
	}
	for in, want := range cases {
		got, err := parseDuration(in)
		if err != nil || got != want {
			t.Errorf("parseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "5x", "s"} {
		if _, err := parseDuration(bad); err == nil {
			t.Errorf("parseDuration(%q) should error", bad)
		}
	}
}

func TestSignalByName(t *testing.T) {
	// The default and a couple of common names must resolve (never nil) on the
	// build platform; an unknown name must not resolve.
	for _, name := range []string{"TERM", "SIGKILL", "int", "9"} {
		if signalByName(name) == nil {
			t.Errorf("signalByName(%q) = nil, want a signal", name)
		}
	}
	if signalByName("NOSUCH") != nil {
		t.Error("signalByName(NOSUCH) should be nil")
	}
}

func TestExitStatus(t *testing.T) {
	if got := exitStatus(nil, true, false); got != 124 {
		t.Errorf("timed-out exit = %d, want 124", got)
	}
	if got := exitStatus(nil, false, false); got != 0 {
		t.Errorf("clean exit = %d, want 0", got)
	}
}
