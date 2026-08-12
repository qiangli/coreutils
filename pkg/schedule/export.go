package schedule

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/lockfile"
)

// ParseAtTimespec is the public timespec parser used by the `at` and
// `batch` compatibility commands. It extends the internal parseAt with
// support for relative times ("now + N minutes/hours/days/weeks"),
// named times ("midnight", "noon", "tomorrow"), and combined
// "HH:MM YYYY-MM-DD" format.
func ParseAtTimespec(s string, now time.Time) (time.Time, error) {
	return ParseAtTimespecInLocation(s, now, now.Location())
}

// ParseAtTimespecInLocation parses the POSIX at time operand in loc. An
// explicit trailing "utc" overrides loc. The accepted grammar covers numeric
// and meridian times, named dates/weekdays, today/tomorrow, and increments.
func ParseAtTimespecInLocation(s string, now time.Time, loc *time.Location) (time.Time, error) {
	orig := strings.TrimSpace(s)
	if orig == "" {
		return time.Time{}, fmt.Errorf("invalid timespec %q", orig)
	}
	if loc == nil {
		loc = time.Local
	}
	if t, err := tryParseAt(orig, now.In(loc)); err == nil {
		return t, nil
	}

	baseText, increment, err := splitIncrement(orig)
	if err != nil {
		return time.Time{}, err
	}
	fields := strings.Fields(baseText)
	utc := false
	if len(fields) > 0 && strings.EqualFold(fields[len(fields)-1], "utc") {
		utc = true
		fields = fields[:len(fields)-1]
		loc = time.UTC
	}
	if len(fields) == 0 {
		return time.Time{}, fmt.Errorf("invalid timespec %q", orig)
	}
	now = now.In(loc)

	if strings.EqualFold(fields[0], "now") {
		if len(fields) != 1 || utc {
			return time.Time{}, fmt.Errorf("invalid timespec %q", orig)
		}
		return applyIncrement(now, increment), nil
	}

	hour, minute, consumed, err := parseClock(fields)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timespec %q: %w", orig, err)
	}
	dateFields := fields[consumed:]
	when, explicitDate, err := parseAtDate(dateFields, hour, minute, now, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timespec %q: %w", orig, err)
	}
	if !explicitDate && !when.After(now) {
		when = when.AddDate(0, 0, 1)
	}
	return applyIncrement(when, increment), nil
}

type atIncrement struct {
	n    int
	unit string
}

var incrementRe = regexp.MustCompile(`(?i)^(.*)\+\s*([0-9]+)\s+(minute|minutes|hour|hours|day|days|week|weeks|month|months|year|years)\s*$`)

func splitIncrement(s string) (string, *atIncrement, error) {
	if !strings.Contains(s, "+") {
		return strings.TrimSpace(s), nil, nil
	}
	m := incrementRe.FindStringSubmatch(s)
	if m == nil || strings.TrimSpace(m[1]) == "" {
		return "", nil, fmt.Errorf("invalid increment in timespec %q", s)
	}
	n, err := strconv.Atoi(m[2])
	if err != nil || n <= 0 {
		return "", nil, fmt.Errorf("invalid increment in timespec %q", s)
	}
	return strings.TrimSpace(m[1]), &atIncrement{n: n, unit: strings.ToLower(m[3])}, nil
}

func applyIncrement(t time.Time, inc *atIncrement) time.Time {
	if inc == nil {
		return t
	}
	switch strings.TrimSuffix(inc.unit, "s") {
	case "minute":
		return t.Add(time.Duration(inc.n) * time.Minute)
	case "hour":
		return t.Add(time.Duration(inc.n) * time.Hour)
	case "day":
		return t.AddDate(0, 0, inc.n)
	case "week":
		return t.AddDate(0, 0, 7*inc.n)
	case "month":
		return t.AddDate(0, inc.n, 0)
	case "year":
		return t.AddDate(inc.n, 0, 0)
	}
	return t
}

func parseClock(fields []string) (hour, minute, consumed int, err error) {
	word := strings.ToLower(fields[0])
	switch word {
	case "noon":
		return 12, 0, 1, nil
	case "midnight":
		return 0, 0, 1, nil
	}

	meridian := ""
	consumed = 1
	if len(fields) > 1 {
		candidate := strings.ToLower(fields[1])
		if candidate == "am" || candidate == "pm" {
			meridian, consumed = candidate, 2
		}
	}

	if strings.Contains(word, ":") {
		parts := strings.Split(word, ":")
		if len(parts) != 2 || len(parts[0]) < 1 || len(parts[0]) > 2 || len(parts[1]) < 1 || len(parts[1]) > 2 {
			return 0, 0, 0, fmt.Errorf("invalid clock")
		}
		hour, err = strconv.Atoi(parts[0])
		if err == nil {
			minute, err = strconv.Atoi(parts[1])
		}
	} else {
		if len(word) < 1 || len(word) > 4 || (len(word) == 3) {
			return 0, 0, 0, fmt.Errorf("invalid clock")
		}
		if len(word) == 4 {
			hour, err = strconv.Atoi(word[:2])
			if err == nil {
				minute, err = strconv.Atoi(word[2:])
			}
		} else {
			hour, err = strconv.Atoi(word)
		}
	}
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, 0, fmt.Errorf("invalid clock")
	}
	if meridian != "" {
		if hour < 1 || hour > 12 {
			return 0, 0, 0, fmt.Errorf("invalid meridian hour")
		}
		if hour == 12 {
			hour = 0
		}
		if meridian == "pm" {
			hour += 12
		}
	} else if hour < 0 || hour > 23 {
		return 0, 0, 0, fmt.Errorf("invalid hour")
	}
	return hour, minute, consumed, nil
}

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "sunday": time.Sunday,
	"mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tues": time.Tuesday, "tuesday": time.Tuesday,
	"wed": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
}

var months = map[string]time.Month{
	"jan": time.January, "january": time.January,
	"feb": time.February, "february": time.February,
	"mar": time.March, "march": time.March,
	"apr": time.April, "april": time.April,
	"may": time.May,
	"jun": time.June, "june": time.June,
	"jul": time.July, "july": time.July,
	"aug": time.August, "august": time.August,
	"sep": time.September, "sept": time.September, "september": time.September,
	"oct": time.October, "october": time.October,
	"nov": time.November, "november": time.November,
	"dec": time.December, "december": time.December,
}

func parseAtDate(fields []string, hour, minute int, now time.Time, loc *time.Location) (time.Time, bool, error) {
	makeTime := func(year int, month time.Month, day int) (time.Time, error) {
		t := time.Date(year, month, day, hour, minute, 0, 0, loc)
		if t.Year() != year || t.Month() != month || t.Day() != day {
			return time.Time{}, fmt.Errorf("invalid date")
		}
		return t, nil
	}
	if len(fields) == 0 {
		t, _ := makeTime(now.Year(), now.Month(), now.Day())
		return t, false, nil
	}
	if len(fields) == 1 {
		switch strings.ToLower(fields[0]) {
		case "today":
			t, err := makeTime(now.Year(), now.Month(), now.Day())
			return t, true, err
		case "tomorrow":
			t := now.AddDate(0, 0, 1)
			out, err := makeTime(t.Year(), t.Month(), t.Day())
			return out, true, err
		}
		if weekday, ok := weekdays[strings.ToLower(fields[0])]; ok {
			days := (int(weekday) - int(now.Weekday()) + 7) % 7
			candidate := now.AddDate(0, 0, days)
			out, err := makeTime(candidate.Year(), candidate.Month(), candidate.Day())
			if err == nil && !out.After(now) {
				candidate = candidate.AddDate(0, 0, 7)
				out, err = makeTime(candidate.Year(), candidate.Month(), candidate.Day())
			}
			return out, true, err
		}
		return time.Time{}, false, fmt.Errorf("invalid date")
	}

	month, ok := months[strings.ToLower(strings.TrimSuffix(fields[0], ","))]
	if !ok {
		return time.Time{}, false, fmt.Errorf("invalid month")
	}
	dayText := strings.TrimSuffix(fields[1], ",")
	day, err := strconv.Atoi(dayText)
	if err != nil || day < 1 || day > 31 {
		return time.Time{}, false, fmt.Errorf("invalid day")
	}
	rest := fields[2:]
	if len(rest) > 0 && rest[0] == "," {
		rest = rest[1:]
	}
	year := now.Year()
	explicitYear := false
	if len(rest) > 0 {
		if len(rest) != 1 {
			return time.Time{}, false, fmt.Errorf("invalid date")
		}
		year, err = strconv.Atoi(strings.TrimSuffix(rest[0], ","))
		if err != nil || year < 1970 || year > 9999 {
			return time.Time{}, false, fmt.Errorf("invalid year")
		}
		explicitYear = true
	}
	out, err := makeTime(year, month, day)
	if err == nil && !explicitYear && !out.After(now) {
		out, err = makeTime(year+1, month, day)
	}
	return out, true, err
}

// ParseAtTouchTime parses [[CC]YY]MMDDhhmm[.SS], the POSIX -t format.
func ParseAtTouchTime(s string, now time.Time, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.Local
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 || (len(parts) == 2 && len(parts[1]) != 2) {
		return time.Time{}, fmt.Errorf("invalid -t time %q", s)
	}
	digits := parts[0]
	if len(digits) != 8 && len(digits) != 10 && len(digits) != 12 {
		return time.Time{}, fmt.Errorf("invalid -t time %q", s)
	}
	for _, r := range s {
		if r != '.' && (r < '0' || r > '9') {
			return time.Time{}, fmt.Errorf("invalid -t time %q", s)
		}
	}
	second := 0
	var err error
	if len(parts) == 2 {
		second, err = strconv.Atoi(parts[1])
		if err != nil || second > 60 {
			return time.Time{}, fmt.Errorf("invalid -t time %q", s)
		}
	}
	year := now.In(loc).Year()
	offset := 0
	switch len(digits) {
	case 10:
		yy, _ := strconv.Atoi(digits[:2])
		if yy >= 69 {
			year = 1900 + yy
		} else {
			year = 2000 + yy
		}
		offset = 2
	case 12:
		year, _ = strconv.Atoi(digits[:4])
		offset = 4
	}
	monthNumber, _ := strconv.Atoi(digits[offset : offset+2])
	day, _ := strconv.Atoi(digits[offset+2 : offset+4])
	hour, _ := strconv.Atoi(digits[offset+4 : offset+6])
	minute, _ := strconv.Atoi(digits[offset+6 : offset+8])
	// POSIX defines -t's format as touch -t's, which accepts a seconds field
	// of 60 for a leap second and carries it into the following minute.
	// time.Date would silently normalize a raw 60 instead of rejecting a bad
	// input, so validate against a clamped base second (as cmds/touch's
	// parseISODate does) and apply the carry only after validation passes.
	baseSecond := second
	if second == 60 {
		baseSecond = 59
	}
	when := time.Date(year, time.Month(monthNumber), day, hour, minute, baseSecond, 0, loc)
	if when.Year() != year || int(when.Month()) != monthNumber || when.Day() != day || when.Hour() != hour || when.Minute() != minute || when.Second() != baseSecond {
		return time.Time{}, fmt.Errorf("invalid -t time %q", s)
	}
	if second == 60 {
		when = when.Add(time.Second)
	}
	return when, nil
}

func tryParseAt(s string, now time.Time) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04",
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
	}
	for _, layout := range formats {
		if t, err := time.ParseInLocation(layout, s, now.Location()); err == nil {
			return t, nil
		}
	}

	if t, err := time.ParseInLocation("15:04", s, now.Location()); err == nil {
		today := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
		if !today.After(now) {
			today = today.Add(24 * time.Hour)
		}
		return today, nil
	}

	if t, err := time.ParseInLocation("15:04 2006-01-02", s, now.Location()); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, now.Location()); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("01/02/06 15:04", s, now.Location()); err == nil {
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
	return UpdateJobs(func([]*Job) ([]*Job, error) { return jobs, nil })
}

// UpdateJobs applies one read-modify-write transaction while holding the
// schedule store's cross-process lock. Writers must use this instead of a
// LoadJobs/SaveJobs pair so a daemon tick cannot overwrite a submission from
// a stale snapshot.
func UpdateJobs(update func([]*Job) ([]*Job, error)) error {
	lock, err := lockfile.Acquire(filepath.Join(filepath.Dir(statePath()), "schedule.lock"), lockfile.Holder{
		Name: "bashy-schedule", Intent: "update schedule store",
	})
	if err != nil {
		return err
	}
	defer lock.Release()
	s, err := load()
	if err != nil {
		return err
	}
	jobs, err := update(s.Jobs)
	if err != nil {
		return err
	}
	s.Jobs = jobs
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
