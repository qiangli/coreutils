package schedule

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	posixlocale "github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/pkg/lockfile"
)

// Store is a schedule store at an explicit path. Command adapters use it so
// an embedding RunContext, rather than process-global cwd/environment, selects
// the schedule namespace.
type Store struct{ path string }

// NewStore returns a path-scoped schedule store.
func NewStore(path string) *Store { return &Store{path: path} }

// StatePathFor resolves the schedule store from an invocation directory and
// environment. Relative values are resolved against dir. It returns an empty
// path when that would require consulting the process working directory.
func StatePathFor(dir string, env []string) string {
	lookup := func(name string) (string, bool) {
		prefix := name + "="
		for i := len(env) - 1; i >= 0; i-- {
			if strings.HasPrefix(env[i], prefix) {
				return env[i][len(prefix):], true
			}
		}
		return "", false
	}
	if value, ok := lookup("BASHY_SCHEDULE_STATE"); ok && value != "" {
		if filepath.IsAbs(value) {
			return filepath.Clean(value)
		}
		if dir == "" {
			return ""
		}
		return filepath.Join(dir, value)
	}
	if value, ok := lookup("XDG_CONFIG_HOME"); ok && value != "" {
		if !filepath.IsAbs(value) {
			if dir == "" {
				return ""
			}
			value = filepath.Join(dir, value)
		}
		return filepath.Join(value, "bashy", "schedule.json")
	}
	if value, ok := lookup("HOME"); ok && value != "" {
		if !filepath.IsAbs(value) {
			if dir == "" {
				return ""
			}
			value = filepath.Join(dir, value)
		}
		return filepath.Join(value, ".config", "bashy", "schedule.json")
	}
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, ".bashy", "schedule.json")
}

// StoreFor returns the isolated store selected by an invocation context.
func StoreFor(dir string, env []string) *Store { return NewStore(StatePathFor(dir, env)) }

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
	formatter, _ := posixlocale.ResolveTime(nil)
	return ParseAtTimespecInLocationWithLocale(s, now, loc, formatter)
}

// ParseAtTimespecInLocationWithLocale parses month and weekday names through
// the caller's bounded LC_TIME provider.
func ParseAtTimespecInLocationWithLocale(s string, now time.Time, loc *time.Location, formatter posixlocale.TimeFormatter) (time.Time, error) {
	orig := normalizeAtTimespec(s)
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
	if consumed < len(fields) && strings.EqualFold(fields[consumed], "utc") {
		loc = time.UTC
		now = now.In(loc)
		fields = append(fields[:consumed], fields[consumed+1:]...)
	}
	dateFields := fields[consumed:]
	when, explicitDate, err := parseAtDate(dateFields, hour, minute, now, loc, formatter)
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

var (
	incrementRe     = regexp.MustCompile(`(?i)^(.*)\+\s*([0-9]+)\s*(minute|minutes|hour|hours|day|days|week|weeks|month|months|year|years)\s*$`)
	nextIncrementRe = regexp.MustCompile(`(?i)^(.*)\bnext\s+(minute|minutes|hour|hours|day|days|week|weeks|month|months|year|years)\s*$`)
	adjacentAMPMRe  = regexp.MustCompile(`(?i)^([0-9]{4}|[0-9]{1,2}(?::[0-9]{1,2})?)(am|pm)([[:alpha:]].*)$`)
	adjacentMonthRe = regexp.MustCompile(`(?i)\b(jan|january|feb|february|mar|march|apr|april|may|jun|june|jul|july|aug|august|sep|sept|september|oct|october|nov|november|dec|december)([0-9]{1,2})(,?)\b`)
	looseColonRe    = regexp.MustCompile(`(?i)^([0-9]{1,2})\s+:\s*([0-9]{1,2}.*)$`)
	adjacentUTCRe   = regexp.MustCompile(`(?i)^([0-9]{1,4}(?::[0-9]{1,2})?)utc(.*)$`)
)

func normalizeAtTimespec(s string) string {
	s = strings.TrimSpace(s)
	// POSIX permits an unambiguous clock separator to be adjacent to either
	// component (for example, "8 :15amjan24"). Join that spelling before
	// tokenization while leaving whitespace between the other time parts alone.
	if m := looseColonRe.FindStringSubmatch(s); m != nil {
		s = m[1] + ":" + m[2]
	}
	if m := adjacentAMPMRe.FindStringSubmatch(s); m != nil {
		s = m[1] + " " + m[2] + " " + m[3]
	}
	if m := adjacentUTCRe.FindStringSubmatch(s); m != nil {
		s = m[1] + " utc " + m[2]
	}
	s = adjacentMonthRe.ReplaceAllString(s, "$1 $2$3")
	return s
}

func splitIncrement(s string) (string, *atIncrement, error) {
	if m := nextIncrementRe.FindStringSubmatch(s); m != nil {
		base := strings.TrimSpace(m[1])
		if base == "" {
			base = "now"
		}
		return base, &atIncrement{n: 1, unit: strings.ToLower(m[2])}, nil
	}
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
	if meridian == "" {
		for _, suffix := range []string{"am", "pm"} {
			if strings.HasSuffix(word, suffix) && len(word) > len(suffix) {
				meridian = suffix
				word = strings.TrimSuffix(word, suffix)
				break
			}
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

func parseAtDate(fields []string, hour, minute int, now time.Time, loc *time.Location, formatter posixlocale.TimeFormatter) (time.Time, bool, error) {
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
		if weekday, ok := formatter.ParseWeekday(fields[0]); ok {
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

	month, ok := formatter.ParseMonth(fields[0])
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

func (s *Store) LoadJobs() ([]*Job, error) {
	state, err := loadPath(s.path)
	if err != nil {
		return nil, err
	}
	return state.Jobs, nil
}

// SaveJobs atomically persists a job list.
func SaveJobs(jobs []*Job) error {
	return UpdateJobs(func([]*Job) ([]*Job, error) { return jobs, nil })
}

func (s *Store) SaveJobs(jobs []*Job) error {
	return s.UpdateJobs(func([]*Job) ([]*Job, error) { return jobs, nil })
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

func (s *Store) update(update func(*store) error) error {
	if s == nil || s.path == "" {
		return fmt.Errorf("schedule store path is empty")
	}
	lock, err := lockfile.Acquire(filepath.Join(filepath.Dir(s.path), "schedule.lock"), lockfile.Holder{
		Name: "bashy-schedule", Intent: "update schedule store",
	})
	if err != nil {
		return err
	}
	defer lock.Release()
	state, err := loadPath(s.path)
	if err != nil {
		return err
	}
	if err := update(state); err != nil {
		return err
	}
	return state.savePath(s.path)
}

func (s *Store) UpdateJobs(update func([]*Job) ([]*Job, error)) error {
	return s.update(func(state *store) error {
		jobs, err := update(state.Jobs)
		if err != nil {
			return err
		}
		state.Jobs = jobs
		return nil
	})
}

// CronTable returns the original submitted table. Legacy stores without raw
// source metadata are reconstructed from their executable cron jobs.
func (s *Store) CronTable() ([]byte, bool, error) {
	state, err := loadPath(s.path)
	if err != nil {
		return nil, false, err
	}
	if state.CronSourceSet {
		return append([]byte(nil), state.CronSource...), true, nil
	}
	var lines []string
	for _, job := range state.Jobs {
		if job.Kind != "cron" {
			continue
		}
		command := job.Command
		if len(command) == 3 && command[1] == "-c" {
			command = command[2:]
		}
		lines = append(lines, job.Spec+" "+strings.Join(command, " "))
	}
	if len(lines) == 0 {
		return nil, false, nil
	}
	return []byte(strings.Join(lines, "\n") + "\n"), true, nil
}

// ReplaceCron atomically replaces cron jobs and their byte-exact submitted
// source while leaving at/batch/general scheduler jobs untouched.
func (s *Store) ReplaceCron(source []byte, cronJobs []*Job) error {
	return s.update(func(state *store) error {
		kept := state.Jobs[:0]
		for _, job := range state.Jobs {
			if job.Kind != "cron" {
				kept = append(kept, job)
			}
		}
		state.Jobs = append(kept, cronJobs...)
		state.CronSource = append([]byte(nil), source...)
		state.CronSourceSet = true
		return nil
	})
}

// RemoveCron atomically removes only cron jobs and their source table.
func (s *Store) RemoveCron() error {
	return s.update(func(state *store) error {
		kept := state.Jobs[:0]
		for _, job := range state.Jobs {
			if job.Kind != "cron" {
				kept = append(kept, job)
			}
		}
		state.Jobs = kept
		state.CronSource = nil
		state.CronSourceSet = false
		return nil
	})
}

// Path returns the explicit backing path, primarily for diagnostics/tests.
func (s *Store) Path() string { return s.path }

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
