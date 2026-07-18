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

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "touch",
	Synopsis: "Update the access and modification times of each FILE to the current time. Supports -t STAMP.",
	Usage:    "touch [OPTION]... FILE...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

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
			rest = append(rest, arg)
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

	now := time.Now()
	atime, mtime := now, now
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
		atime, mtime = statAtime(fi), fi.ModTime()
	case pre.tSeen:
		t, err := parseStamp(pre.stamp, now)
		if err != nil {
			fmt.Fprintf(rc.Err, "touch: invalid date format '%s'\n", pre.stamp)
			return 1
		}
		atime, mtime = t, t
	case fs.Changed("date"):
		t, err := parseDate(*date, now)
		if err != nil {
			fmt.Fprintf(rc.Err, "touch: invalid date format '%s'\n", *date)
			return 1
		}
		atime, mtime = t, t
	}

	changeA := pre.atime || !pre.mtime
	changeM := pre.mtime || !pre.atime

	exit := 0
	for _, name := range operands {
		if name == "-" {
			return tool.NotSupported(rc, cmd, "the '-' operand (the file open on standard output)")
		}
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
			f, cerr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o666)
			if cerr != nil {
				fmt.Fprintf(rc.Err, "touch: cannot touch '%s': %v\n", name, reason(cerr))
				exit = 1
				continue
			}
			f.Close()
		}

		var at, mt time.Time
		if changeA {
			at = atime
		}
		if changeM {
			mt = mtime
		}

		if *noDeref {
			if err := applyChtimesNoDeref(path, at, mt); err != nil {
				fmt.Fprintf(rc.Err, "touch: setting times of '%s': %v\n", name, reason(err))
				exit = 1
			}
		} else {
			if err := os.Chtimes(path, at, mt); err != nil {
				fmt.Fprintf(rc.Err, "touch: setting times of '%s': %v\n", name, reason(err))
				exit = 1
			}
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
	t := time.Date(year, time.Month(month), day, hour, minute, sec, 0, time.Local)
	if t.Month() != time.Month(month) || t.Day() != day {
		return time.Time{}, errBad
	}
	return t, nil
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
		t, err := time.ParseInLocation(layout, s, time.Local)
		if err != nil {
			continue
		}
		if t.Year() == 0 {
			// A bare time of day: GNU anchors it to today.
			return time.Date(now.Year(), now.Month(), now.Day(),
				t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.Local), nil
		}
		return t, nil
	}
	return parseRelative(s, now)
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
		t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
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
