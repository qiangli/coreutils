package weave

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const weaveThrottleTailBytes = 16 * 1024

// These are substring heuristics for provider/subscription throttles. Many
// subscription-billed CLIs report rolling-window caps in terminal text instead
// of surfacing a clean HTTP status.
var weaveThrottleGenericSignals = []string{
	"usage limit",
	"rate limit",
	"rate_limit",
	"rate-limited",
	"quota",
	"too many requests",
	"429",
	"you've reached your",
	"weekly limit",
	"daily limit",
	"limit reached",
	"try again later",
	"overloaded",
	"insufficient_quota",
	"resource_exhausted",
}

var weaveThrottleToolSignals = map[string][]string{}

// weaveClassifyThrottle reports whether a worker's termination looks like a
// provider/subscription throttle (vs a normal finish or a crash), and the
// matched signal string for the audit trail. tool is the argv[0] basename
// (claude/codex/opencode/aider/gemini/bash/...); exitCode is the worker's
// exit; logTail is the last chunk of its PTY log.
func weaveClassifyThrottle(tool string, exitCode int, logTail string) (throttled bool, signal string) {
	_ = exitCode // A phrase match is required; bare non-zero exits are crashes.
	if logTail == "" {
		return false, ""
	}
	haystack := strings.ToLower(weaveThrottleTail(logTail, weaveThrottleTailBytes))
	for _, phrase := range weaveThrottleGenericSignals {
		if strings.Contains(haystack, phrase) {
			return true, phrase
		}
	}
	tool = filepath.Base(strings.ToLower(strings.TrimSpace(tool)))
	for _, phrase := range weaveThrottleToolSignals[tool] {
		if strings.Contains(haystack, strings.ToLower(phrase)) {
			return true, phrase
		}
	}
	return false, ""
}

// Reset-time extractors. Subscription-billed CLIs phrase the "try again"
// time as a wall-clock time ("at 11:41 AM"), a relative duration ("in 5
// minutes"), or a Retry-After seconds value. We parse the first match of
// each shape against the (lowercased) message and return the soonest
// usable instant. Stdlib-only — no new dependency.
var (
	// "at 11:41 am" / "at 11:41 pm" / "at 11 am" (am/pm required so we
	// don't swallow unrelated colon-separated numbers like a URL port).
	reThrottleClock = regexp.MustCompile(`\bat\s+(\d{1,2})(?::(\d{2}))?\s*([ap])\.?m\.?`)
	// "in 5 minutes" / "in 30 seconds" / "in 2 hours" (+ singular forms).
	reThrottleRelative = regexp.MustCompile(`\bin\s+(\d+)\s*(second|minute|hour|sec|min|hr)s?\b`)
	// "retry-after: 120" / "retry after 120" (seconds, per RFC 9110).
	reThrottleRetryAfter = regexp.MustCompile(`retry[\s-]*after:?\s+(\d+)`)
)

// parseThrottleReset extracts a "next available" instant from a throttle
// message, relative to now. It handles wall-clock times ("try again at
// 11:41 AM" → today, or tomorrow if already past), relative durations
// ("in N seconds|minutes|hours"), and "Retry-After: N" (seconds). It is
// tolerant of surrounding text and case. Returns (zero, false) when no
// reset is parseable. now's location is used for wall-clock resolution.
func parseThrottleReset(msg string, now time.Time) (time.Time, bool) {
	lower := strings.ToLower(msg)

	// Retry-After (seconds) — most explicit, check first.
	if m := reThrottleRetryAfter.FindStringSubmatch(lower); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n >= 0 {
			return now.Add(time.Duration(n) * time.Second), true
		}
	}

	// Relative duration ("in N <unit>").
	if m := reThrottleRelative.FindStringSubmatch(lower); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n >= 0 {
			var unit time.Duration
			switch m[2] {
			case "second", "sec":
				unit = time.Second
			case "minute", "min":
				unit = time.Minute
			case "hour", "hr":
				unit = time.Hour
			}
			if unit != 0 {
				return now.Add(time.Duration(n) * unit), true
			}
		}
	}

	// Wall-clock ("at HH[:MM] am|pm") in now's local zone. Resolve to
	// today; if that instant is already in the past, roll to tomorrow.
	if m := reThrottleClock.FindStringSubmatch(lower); m != nil {
		hour, err := strconv.Atoi(m[1])
		if err == nil && hour >= 1 && hour <= 12 {
			minute := 0
			if m[2] != "" {
				minute, _ = strconv.Atoi(m[2])
			}
			if minute >= 0 && minute < 60 {
				switch m[3] {
				case "a": // 12 AM == 00:00
					if hour == 12 {
						hour = 0
					}
				case "p": // 12 PM == 12:00; 1-11 PM == +12
					if hour != 12 {
						hour += 12
					}
				}
				reset := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
				if !reset.After(now) {
					reset = reset.AddDate(0, 0, 1)
				}
				return reset, true
			}
		}
	}

	return time.Time{}, false
}

func weaveThrottleTail(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	return s[len(s)-maxBytes:]
}

func weaveReadThrottleLogTail(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	start := int64(0)
	if st.Size() > weaveThrottleTailBytes {
		start = st.Size() - weaveThrottleTailBytes
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(f, weaveThrottleTailBytes))
	if err != nil {
		return ""
	}
	return string(b)
}
