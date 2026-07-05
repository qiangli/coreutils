package schedule

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseAtTimespec is the public timespec parser used by the `at` and
// `batch` compatibility commands. It extends the internal parseAt with
// support for relative times ("now + N minutes/hours/days/weeks"),
// named times ("midnight", "noon", "tomorrow"), and combined
// "HH:MM YYYY-MM-DD" format.
func ParseAtTimespec(s string, now time.Time) (time.Time, error) {
	orig := strings.TrimSpace(s)

	if t, err := tryParseAt(orig, now); err == nil {
		return t, nil
	}

	lower := strings.ToLower(orig)
	switch lower {
	case "midnight":
		t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		if !t.After(now) {
			t = t.Add(24 * time.Hour)
		}
		return t, nil
	case "noon":
		t := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.Local)
		if !t.After(now) {
			t = t.Add(24 * time.Hour)
		}
		return t, nil
	case "tomorrow":
		return time.Date(now.Year(), now.Month(), now.Day()+1, now.Hour(), now.Minute(), 0, 0, time.Local), nil
	case "now":
		return now.Add(1 * time.Second), nil
	}

	if strings.HasPrefix(lower, "now") {
		return parseRelative(now, strings.TrimSpace(orig[3:]))
	}

	return time.Time{}, fmt.Errorf("invalid timespec %q", orig)
}

var relativeRe = regexp.MustCompile(`^\+\s*(\d+)\s*(minute|minutes|hour|hours|day|days|week|weeks|month|months)\s*$`)

func parseRelative(now time.Time, s string) (time.Time, error) {
	if s == "" || s == "now" {
		return now.Add(1 * time.Second), nil
	}
	m := relativeRe.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, fmt.Errorf("invalid relative time %q (expected: \"+ N minutes/hours/days/weeks/months\")", s)
	}
	n, _ := strconv.Atoi(m[1])
	unit := m[2]
	switch {
	case strings.HasPrefix(unit, "minute"):
		return now.Add(time.Duration(n) * time.Minute), nil
	case strings.HasPrefix(unit, "hour"):
		return now.Add(time.Duration(n) * time.Hour), nil
	case strings.HasPrefix(unit, "day"):
		return now.Add(time.Duration(n*24) * time.Hour), nil
	case strings.HasPrefix(unit, "week"):
		return now.Add(time.Duration(n*24*7) * time.Hour), nil
	case strings.HasPrefix(unit, "month"):
		return now.AddDate(0, n, 0), nil
	}
	return time.Time{}, fmt.Errorf("unknown relative unit %q", unit)
}

func tryParseAt(s string, now time.Time) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04",
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
	}
	for _, layout := range formats {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}

	if t, err := time.ParseInLocation("15:04", s, time.Local); err == nil {
		today := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
		if !today.After(now) {
			today = today.Add(24 * time.Hour)
		}
		return today, nil
	}

	if t, err := time.ParseInLocation("15:04 2006-01-02", s, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("01/02/06 15:04", s, time.Local); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("unrecognized time")
}

// LoadJobs loads all scheduled jobs from the persistent store.
func LoadJobs() ([]*Job, error) {
	s, err := load()
	if err != nil {
		return nil, err
	}
	return s.Jobs, nil
}

// SaveJobs atomically persists a job list.
func SaveJobs(jobs []*Job) error {
	s := &store{Jobs: jobs}
	return s.save()
}

// FindJob returns the job with the given id or name, or nil.
func FindJob(jobs []*Job, id string) *Job {
	s := &store{Jobs: jobs}
	return s.find(id)
}

// ComputeNext delegates to the job's internal computeNext, returning
// the next fire time at or after now.
func ComputeNext(j *Job, now time.Time) (time.Time, error) {
	return j.computeNext(now)
}
