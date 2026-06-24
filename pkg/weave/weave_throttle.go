package weave

import (
	"io"
	"os"
	"path/filepath"
	"strings"
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
