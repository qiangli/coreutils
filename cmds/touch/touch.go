// Package touchcmd implements touch(1) per the GNU coreutils manual:
// update the access and modification times of each FILE to the current
// time, creating missing files unless told otherwise.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/touch (BSD-3-Clause).
// Changes: rewired to tool framework; -a/-m/-t pre-parsed manually (they
// have no GNU long forms); added -r and -t; -d limited to a documented
// ISO-8601-style subset; selective atime/mtime updates rely on
// os.Chtimes zero-time semantics instead of pre-stat.
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
	Synopsis: "Update the access and modification times of each FILE to the current time.",
	Usage:    "touch [OPTION]... FILE...",
}

// Run is wired in init: a literal would create an initialization cycle.
func init() { cmd.Run = run; tool.Register(cmd) }

type prescanned struct {
	atime bool // -a
	mtime bool // -m
	stamp string
	tSeen bool // -t
	rest  []string
}

// prescan extracts the short-only GNU flags (-a, -m, -t STAMP) before
// pflag sees the arguments — GNU defines no long forms for them and
// inventing long names is forbidden. -c/-d/-r (which do have long
// forms) are left in place for pflag, including inside clusters.
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
			if arg == "--date" || arg == "--reference" {
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
					// Value-taking flags pflag owns: the rest of the
					// cluster (or the next argument) is the value.
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
	noCreate := fs.BoolP("no-create", "c", false, "do not create any files")
	date := fs.StringP("date", "d", "", "parse STRING and use it instead of current time")
	ref := fs.StringP("reference", "r", "", "use this file's times instead of current time")
	operands, code := tool.Parse(rc, cmd, fs, pre.rest)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing file operand")
	}
	if pre.tSeen && (*date != "" || *ref != "") {
		return tool.UsageError(rc, cmd, "cannot specify times from more than one source")
	}
	if *date != "" && *ref != "" {
		// GNU interprets --date relative to the reference file's time;
		// relative date strings are not implemented here.
		return tool.NotSupported(rc, cmd, "combining --date with --reference")
	}

	now := time.Now()
	atime, mtime := now, now
	switch {
	case *ref != "":
		fi, err := os.Stat(rc.Path(*ref))
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
	case *date != "":
		t, err := parseDate(*date)
		if err != nil {
			fmt.Fprintf(rc.Err, "touch: invalid date format '%s'\n", *date)
			return 1
		}
		atime, mtime = t, t
	}

	// Default (neither -a nor -m): change both.
	changeA := pre.atime || !pre.mtime
	changeM := pre.mtime || !pre.atime

	exit := 0
	for _, name := range operands {
		if name == "-" {
			// GNU touch '-' changes the file open on standard output;
			// there is no such file in an embedded invocation.
			return tool.NotSupported(rc, cmd, "the '-' operand (the file open on standard output)")
		}
		path := rc.Path(name)
		if _, err := os.Stat(path); err != nil {
			if !errors.Is(err, iofs.ErrNotExist) {
				fmt.Fprintf(rc.Err, "touch: cannot touch '%s': %v\n", name, reason(err))
				exit = 1
				continue
			}
			if *noCreate {
				continue // GNU: missing file with -c is not an error
			}
			f, cerr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o666)
			if cerr != nil {
				fmt.Fprintf(rc.Err, "touch: cannot touch '%s': %v\n", name, reason(cerr))
				exit = 1
				continue
			}
			f.Close()
		}
		// Zero time values leave the corresponding timestamp unchanged.
		var at, mt time.Time
		if changeA {
			at = atime
		}
		if changeM {
			mt = mtime
		}
		if err := os.Chtimes(path, at, mt); err != nil {
			fmt.Fprintf(rc.Err, "touch: setting times of '%s': %v\n", name, reason(err))
			exit = 1
		}
	}
	return exit
}

// parseStamp parses the -t operand: [[CC]YY]MMDDhhmm[.ss].
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
		if sec > 60 { // 60 allowed for leap seconds
			return time.Time{}, errBad
		}
	}
	if !allDigits(main) {
		return time.Time{}, errBad
	}
	var year int
	switch len(main) {
	case 8: // MMDDhhmm
		year = now.Year()
	case 10: // YYMMDDhhmm
		yy, _ := strconv.Atoi(main[:2])
		if yy >= 69 {
			year = 1900 + yy
		} else {
			year = 2000 + yy
		}
		main = main[2:]
	case 12: // CCYYMMDDhhmm
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
		return time.Time{}, errBad // e.g. Feb 30 normalized away
	}
	return t, nil
}

// parseDate parses the supported -d/--date subset: ISO-8601-style
// date/time strings (with optional fractional seconds and zone) and
// '@SECONDS' since the epoch. Each accepted form means exactly what it
// means to GNU date; anything else is an invalid-date error.
func parseDate(s string) (time.Time, error) {
	if strings.HasPrefix(s, "@") {
		secs, err := strconv.ParseInt(s[1:], 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		return time.Unix(secs, 0), nil
	}
	layouts := []string{
		time.RFC3339Nano, // zone carried in the string
		"2006-01-02T15:04:05.999999999",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("invalid date format")
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

// reason unwraps os wrapper errors so diagnostics read like GNU's
// ("No such file or directory" instead of "stat /x: no such file...").
func reason(err error) error {
	return tool.SysErr(err)
}
