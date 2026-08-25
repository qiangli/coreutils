// Package touchcmd implements touch(1) per the GNU coreutils manual:
// update the access and modification times of each FILE to the current
// time, creating missing files unless told otherwise.
//
// Implemented flags: -a -c -d -h -m -r -t --no-dereference --time.
//
// -d accepts @SECS[.FRAC], ISO/calendar timestamps, a bare time of day, and
// relative items ("now", "yesterday", "+2 hours", "3 days ago").
// Portions adapted from https://github.com/u-root/u-root cmds/core/touch (BSD-3-Clause).
package touchcmd

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/cmds/internal/tzenv"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "touch",
	Synopsis: "Update the access and modification times of each FILE to the current time. Supports -t STAMP.",
	Usage:    "touch [OPTION]... FILE...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

// referenceAtime is a platform seam for the access timestamp used by -r. Some
// Go targets do not expose a supported stat field; false means the access-only
// reference form must fail loudly instead of substituting mtime.
var referenceAtime = statAtime

type prescanned struct {
	atime bool
	mtime bool
	stamp string
	tSeen bool
	rest  []string
}

func prescan(args []string) (pre prescanned, errMsg string) {
	rest := make([]string, 0, len(args))
	valueNext := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case valueNext:
			rest = append(rest, arg)
			valueNext = false
		case arg == "--":
			rest = append(rest, args[i:]...)
			i = len(args)
		case arg == "-" || len(arg) < 2 || arg[0] != '-':
			// POSIX utility syntax stops option recognition at the first
			// operand. pflag normally permits interspersed GNU options, so
			// mark the boundary explicitly and preserve every later argument
			// (including "-m" and "--") as a pathname.
			rest = append(rest, "--")
			rest = append(rest, args[i:]...)
			i = len(args)
		case strings.HasPrefix(arg, "--"):
			if arg == "--date" || arg == "--reference" || arg == "--time" {
				valueNext = true
			}
			rest = append(rest, arg)
		default:
			keep := []byte{'-'}
			body := arg[1:]
		cluster:
			for j := 0; j < len(body); j++ {
				switch body[j] {
				case 'a':
					pre.atime = true
				case 'm':
					pre.mtime = true
				case 't':
					pre.tSeen = true
					if j+1 < len(body) {
						pre.stamp = body[j+1:]
					} else if i+1 < len(args) {
						i++
						pre.stamp = args[i]
					} else {
						return pre, "option requires an argument -- 't'"
					}
					break cluster
				case 'd', 'r':
					keep = append(keep, body[j:]...)
					if j == len(body)-1 {
						valueNext = true
					}
					break cluster
				default:
					keep = append(keep, body[j])
				}
			}
			if len(keep) > 1 {
				rest = append(rest, string(keep))
			}
		}
	}
	pre.rest = rest
	return pre, ""
}

func run(rc *tool.RunContext, args []string) int {
	pre, perr := prescan(args)
	if perr != "" {
		return tool.UsageError(rc, cmd, "%s", perr)
	}
	fs := tool.NewFlags(cmd.Name)
	accessOnly := fs.BoolP("access", "a", false, "change only the access time")
	noCreate := fs.BoolP("no-create", "c", false, "do not create any files")
	_ = fs.BoolP("force", "f", false, "ignored; provided for compatibility with BSD touch(1)")
	date := fs.StringP("date", "d", "", "parse STRING and use it instead of current time")
	noDeref := fs.BoolP("no-dereference", "h", false, "affect symbolic links instead of any referenced file")
	ref := fs.StringP("reference", "r", "", "use this file's times instead of current time")
	stamp := fs.StringP("stamp", "t", "", "use [[CC]YY]MMDDhhmm[.ss] instead of current time")
	modifyOnly := fs.BoolP("modify", "m", false, "change only the modification time")
	timeWord := fs.StringP("time", "", "", "which time to change: access (or atime, use), modify (or mtime); implies -a for access, -m for modify")
	operands, code := tool.Parse(rc, cmd, fs, pre.rest)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing file operand")
	}
	if pre.tSeen && (fs.Changed("date") || *ref != "") {
		return tool.UsageError(rc, cmd, "cannot specify times from more than one source")
	}
	if fs.Changed("stamp") {
		if pre.tSeen {
			return tool.UsageError(rc, cmd, "cannot specify multiple -t values")
		}
		pre.tSeen = true
		pre.stamp = *stamp
	}
	if *accessOnly {
		pre.atime = true
	}
	if *modifyOnly {
		pre.mtime = true
	}
	if fs.Changed("date") && *ref != "" {
		return tool.NotSupported(rc, cmd, "combining --date with --reference")
	}

	if *timeWord != "" {
		switch strings.ToLower(*timeWord) {
		case "access", "atime", "use":
			pre.atime = true
			pre.mtime = false
		case "modify", "mtime":
			pre.atime = false
			pre.mtime = true
		default:
			return tool.UsageError(rc, cmd, "invalid time word %q", *timeWord)
		}
	}

	loc := touchLocation(rc)
	now := time.Now().In(loc)
	atime, mtime := now, now
	changeA := pre.atime || !pre.mtime
	changeM := pre.mtime || !pre.atime
	// useNow records that no explicit time source (-r/-t/-d) was given, so the
	// changed timestamps must be set to the current time. POSIX requires this
	// form to work for anyone with write permission (like utime(path, NULL));
	// setting an explicit timestamp instead would demand file ownership. It is
	// realized on unix with UTIME_NOW — see setFileTimes.
	useNow := true
	switch {
	case *ref != "":
		var fi os.FileInfo
		var err error
		if *noDeref {
			fi, err = os.Lstat(rc.Path(*ref))
		} else {
			fi, err = os.Stat(rc.Path(*ref))
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "touch: failed to get attributes of '%s': %v\n", *ref, reason(err))
			return 1
		}
		mtime = fi.ModTime()
		if changeA {
			var ok bool
			atime, ok = referenceAtime(fi)
			if !ok {
				fmt.Fprintf(rc.Err, "touch: cannot use access time of '%s': unsupported on this platform\n", *ref)
				return 1
			}
		}
		useNow = false
	case pre.tSeen:
		t, err := parseStamp(pre.stamp, now)
		if err != nil {
			fmt.Fprintf(rc.Err, "touch: invalid date format '%s'\n", pre.stamp)
			return 1
		}
		atime, mtime = t, t
		useNow = false
	case fs.Changed("date"):
		t, err := parseDate(*date, now)
		if err != nil {
			fmt.Fprintf(rc.Err, "touch: invalid date format '%s'\n", *date)
			return 1
		}
		atime, mtime = t, t
		useNow = false
	}

	exit := 0
	for _, name := range operands {
		// Issue 7 defines the touch operand as "A pathname of a file
		// whose times are to be modified" and gives "-" no special
		// meaning, so it names the file "-" like any other pathname.
		path := rc.Path(name)

		statFn := os.Stat
		if *noDeref {
			statFn = os.Lstat
		}
		if _, err := statFn(path); err != nil {
			if !errors.Is(err, iofs.ErrNotExist) {
				fmt.Fprintf(rc.Err, "touch: cannot touch '%s': %v\n", name, reason(err))
				exit = 1
				continue
			}
			if *noCreate {
				continue
			}
			if *noDeref {
				var lerr error
				if _, lerr = os.Lstat(path); lerr != nil && errors.Is(lerr, iofs.ErrNotExist) {
					// dangling symlink check: since the file doesn't exist and -h is set,
					// we'd need to stat the parent. For now, just try creating a regular file.
				}
			}
			f, cerr := rc.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o666)
			if cerr != nil {
				fmt.Fprintf(rc.Err, "touch: cannot touch '%s': %v\n", name, reason(cerr))
				exit = 1
				continue
			}
			f.Close()
		}

		if err := setFileTimes(path, changeA, changeM, useNow, atime, mtime, !*noDeref); err != nil {
			fmt.Fprintf(rc.Err, "touch: setting times of '%s': %v\n", name, reason(err))
			exit = 1
		}
	}
	return exit
}

func parseStamp(s string, now time.Time) (time.Time, error) {
	errBad := errors.New("invalid date format")
	main := s
	sec := 0
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		main = s[:dot]
		ss := s[dot+1:]
		if len(ss) != 2 || !allDigits(ss) {
			return time.Time{}, errBad
		}
		sec, _ = strconv.Atoi(ss)
		if sec > 60 {
			return time.Time{}, errBad
		}
	}
	if !allDigits(main) {
		return time.Time{}, errBad
	}
	var year int
	switch len(main) {
	case 8:
		year = now.Year()
	case 10:
		yy, _ := strconv.Atoi(main[:2])
		if yy >= 69 {
			year = 1900 + yy
		} else {
			year = 2000 + yy
		}
		main = main[2:]
	case 12:
		year, _ = strconv.Atoi(main[:4])
		main = main[4:]
	default:
		return time.Time{}, errBad
	}
	month, _ := strconv.Atoi(main[0:2])
	day, _ := strconv.Atoi(main[2:4])
	hour, _ := strconv.Atoi(main[4:6])
	minute, _ := strconv.Atoi(main[6:8])
	if month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 {
		return time.Time{}, errBad
	}
	// Issue 7: SS is [00,60]; an SS of 60 that is not a real leap second
	// means one second after SS=59, so build the time at :59 (keeping the
	// month/day validation meaningful at 23:59) and add the second after.
	baseSecond := sec
	if sec == 60 {
		baseSecond = 59
	}
	t := time.Date(year, time.Month(month), day, hour, minute, baseSecond, 0, now.Location())
	if t.Month() != time.Month(month) || t.Day() != day {
		return time.Time{}, errBad
	}
	if sec == 60 {
		t = t.Add(time.Second)
	}
	// Keep -t within the portable signed-32-bit time_t range. Some file
	// systems silently clamp older values while reporting success; in that
	// case touch would claim to have installed a timestamp that it did not.
	// POSIX permits rejecting times outside the implementation's range.
	if t.Unix() < -1<<31 {
		return time.Time{}, errBad
	}
	return t, nil
}

// touchLocation resolves the time zone touch should interpret -t/-d/current-
// time values in, from rc.Env rather than the process's own environment —
// tools must not read os.Environ directly (see tool.RunContext), since an
// embedding shell's env for one invocation (e.g. a "TZ=... touch ..."
// prefix) can differ from the host process's. Resolution is tzenv's full
// POSIX TZ handling: unset defers to the host's default location, "" means
// UTC, and both IANA names ("America/New_York") and POSIX expansions
// ("PST8", "EST5EDT,M3.2.0,M11.1.0") are honored.
func touchLocation(rc *tool.RunContext) *time.Location {
	return tzenv.Location(rc.Env)
}

var errBadDate = errors.New("invalid date format")

// parseDate implements the subset of GNU date-string syntax that touch -d is
// documented to accept: seconds-since-epoch (@SECS, optionally fractional),
// calendar/ISO timestamps, bare times of day, and relative items
// ("now", "yesterday", "+2 hours", "3 days ago").
func parseDate(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errBadDate
	}
	if strings.HasPrefix(s, "@") {
		return parseEpoch(s[1:])
	}
	if t, recognized, err := parseISODate(s, now.Location()); recognized {
		return t, err
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999 -0700",
		"2006-01-02 15:04:05.999999999 MST",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
		"2006-01-02",
		"2006/01/02 15:04:05",
		"2006/01/02",
		"15:04:05.999999999",
		"15:04",
	}
	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, s, now.Location())
		if err != nil {
			continue
		}
		if t.Year() == 0 {
			// A bare time of day: GNU anchors it to today.
			return time.Date(now.Year(), now.Month(), now.Day(),
				t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), now.Location()), nil
		}
		return t, nil
	}
	return parseRelative(s, now)
}

// parseISODate handles the POSIX touch ISO forms directly. time.Parse rejects
// a seconds field of 60, but POSIX requires accepting it and carrying into the
// following minute. Both '.' and ',' are valid fractional separators.
func parseISODate(s string, local *time.Location) (time.Time, bool, error) {
	if len(s) < 19 || s[4] != '-' || s[7] != '-' ||
		(s[10] != 'T' && s[10] != ' ') || s[13] != ':' || s[16] != ':' {
		return time.Time{}, false, nil
	}
	for _, span := range [][2]int{{0, 4}, {5, 7}, {8, 10}, {11, 13}, {14, 16}, {17, 19}} {
		if !allDigits(s[span[0]:span[1]]) {
			return time.Time{}, true, errBadDate
		}
	}
	year, _ := strconv.Atoi(s[0:4])
	month, _ := strconv.Atoi(s[5:7])
	day, _ := strconv.Atoi(s[8:10])
	hour, _ := strconv.Atoi(s[11:13])
	minute, _ := strconv.Atoi(s[14:16])
	second, _ := strconv.Atoi(s[17:19])
	if month < 1 || month > 12 || day < 1 || day > 31 ||
		hour > 23 || minute > 59 || second > 60 {
		return time.Time{}, true, errBadDate
	}

	rest := s[19:]
	nsec := 0
	if len(rest) > 0 && (rest[0] == '.' || rest[0] == ',') {
		rest = rest[1:]
		i := 0
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			i++
		}
		if i == 0 {
			return time.Time{}, true, errBadDate
		}
		frac := rest[:i]
		rest = rest[i:]
		if len(frac) > 9 {
			frac = frac[:9]
		}
		for len(frac) < 9 {
			frac += "0"
		}
		nsec, _ = strconv.Atoi(frac)
	}

	loc := local
	switch {
	case rest == "":
	case rest == "Z":
		loc = time.UTC
	case len(rest) == 6 && (rest[0] == '+' || rest[0] == '-') && rest[3] == ':' &&
		allDigits(rest[1:3]) && allDigits(rest[4:6]):
		tzh, _ := strconv.Atoi(rest[1:3])
		tzm, _ := strconv.Atoi(rest[4:6])
		if tzh > 23 || tzm > 59 {
			return time.Time{}, true, errBadDate
		}
		offset := (tzh*60 + tzm) * 60
		if rest[0] == '-' {
			offset = -offset
		}
		loc = time.FixedZone("", offset)
	default:
		// Leave date syntaxes with named or space-separated zones to the
		// general layout parser below.
		return time.Time{}, false, nil
	}

	baseSecond := second
	if second == 60 {
		baseSecond = 59
	}
	t := time.Date(year, time.Month(month), day, hour, minute, baseSecond, nsec, loc)
	if t.Year() != year || t.Month() != time.Month(month) || t.Day() != day ||
		t.Hour() != hour || t.Minute() != minute || t.Second() != baseSecond {
		return time.Time{}, true, errBadDate
	}
	if second == 60 {
		t = t.Add(time.Second)
	}
	return t, true, nil
}

func parseEpoch(s string) (time.Time, error) {
	frac := ""
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		frac, s = s[dot+1:], s[:dot]
		if !allDigits(frac) {
			return time.Time{}, errBadDate
		}
	}
	if s == "" || (s[0] == '-' && len(s) == 1) {
		return time.Time{}, errBadDate
	}
	secs, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, errBadDate
	}
	nsec := int64(0)
	if frac != "" {
		for len(frac) < 9 {
			frac += "0"
		}
		nsec, _ = strconv.ParseInt(frac[:9], 10, 64)
	}
	return time.Unix(secs, nsec), nil
}

// relUnits maps the unit words GNU's parse-datetime accepts to a multiplier
// pair: a duration (for sub-day units) or a months/days count, since calendar
// months and years are not fixed-length.
var relUnits = map[string]struct {
	d      time.Duration
	months int
	days   int
}{
	"sec": {d: time.Second}, "secs": {d: time.Second},
	"second": {d: time.Second}, "seconds": {d: time.Second},
	"min": {d: time.Minute}, "mins": {d: time.Minute},
	"minute": {d: time.Minute}, "minutes": {d: time.Minute},
	"hour": {d: time.Hour}, "hours": {d: time.Hour},
	"day": {days: 1}, "days": {days: 1},
	"week": {days: 7}, "weeks": {days: 7}, "fortnight": {days: 14},
	"month": {months: 1}, "months": {months: 1},
	"year": {months: 12}, "years": {months: 12},
}

// parseRelative handles keyword and relative-item date strings such as
// "now", "tomorrow", "+1 week", "2 days ago", or "1 hour 30 minutes ago".
func parseRelative(s string, now time.Time) (time.Time, error) {
	fields := strings.Fields(strings.ToLower(s))
	if len(fields) == 0 {
		return time.Time{}, errBadDate
	}
	sign := 1
	if fields[len(fields)-1] == "ago" {
		sign = -1
		fields = fields[:len(fields)-1]
	}

	t := now
	midnight := false
	matched := false
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		switch f {
		case "now", "today":
			matched = true
			continue
		case "yesterday":
			t, midnight, matched = t.AddDate(0, 0, -1), true, true
			continue
		case "tomorrow":
			t, midnight, matched = t.AddDate(0, 0, 1), true, true
			continue
		}

		// A relative item: an optional count, then a unit word. GNU allows the
		// count to be attached ("+2days"), separate ("+2 days"), or absent
		// ("next day" is not supported; a bare unit means one).
		num, unit := splitCount(f)
		if unit == "" {
			if i+1 >= len(fields) {
				return time.Time{}, errBadDate
			}
			i++
			unit = fields[i]
		}
		u, ok := relUnits[unit]
		if !ok {
			return time.Time{}, errBadDate
		}
		n := sign * num
		switch {
		case u.months != 0:
			t = t.AddDate(0, n*u.months, 0)
		case u.days != 0:
			t = t.AddDate(0, 0, n*u.days)
		default:
			t = t.Add(time.Duration(n) * u.d)
		}
		matched = true
	}
	if !matched {
		return time.Time{}, errBadDate
	}
	if midnight && len(fields) == 1 {
		// "yesterday"/"tomorrow" alone mean that day at 00:00:00.
		t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())
	}
	return t, nil
}

// splitCount peels a leading signed integer off a field, returning the count
// and whatever unit text was attached. A field that is only a number yields an
// empty unit, telling the caller to consume the next field.
func splitCount(f string) (num int, unit string) {
	i := 0
	if i < len(f) && (f[i] == '+' || f[i] == '-') {
		i++
	}
	start := i
	for i < len(f) && f[i] >= '0' && f[i] <= '9' {
		i++
	}
	if i == start {
		return 1, f // no digits: a bare unit word means one of it
	}
	n, err := strconv.Atoi(f[:i])
	if err != nil {
		return 0, f
	}
	return n, f[i:]
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func reason(err error) error {
	return tool.SysErr(err)
}
