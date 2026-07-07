// Package touchcmd implements touch(1) per the GNU coreutils manual:
// update the access and modification times of each FILE to the current
// time, creating missing files unless told otherwise.
//
// Implemented flags: -a -c -d -h -m -r -t --no-dereference --time.
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
	Synopsis: "Update the access and modification times of each FILE to the current time.",
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
	noCreate := fs.BoolP("no-create", "c", false, "do not create any files")
	date := fs.StringP("date", "d", "", "parse STRING and use it instead of current time")
	noDeref := fs.BoolP("no-dereference", "h", false, "affect symbolic links instead of any referenced file")
	ref := fs.StringP("reference", "r", "", "use this file's times instead of current time")
	timeWord := fs.StringP("time", "", "", "which time to change: access (or atime, use), modify (or mtime); implies -a for access, -m for modify")
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
	case *date != "":
		t, err := parseDate(*date)
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

func parseDate(s string) (time.Time, error) {
	if strings.HasPrefix(s, "@") {
		secs, err := strconv.ParseInt(s[1:], 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		return time.Unix(secs, 0), nil
	}
	layouts := []string{
		time.RFC3339Nano,
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

func reason(err error) error {
	return tool.SysErr(err)
}
