package loggercmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// priority is a syslog priority: the facility in the high bits, the severity
// level in the low three. It is declared here rather than reused from
// log/syslog because log/syslog does not exist on every platform this package
// must COMPILE for; the unix sink converts at the boundary.
type priority int

const (
	severityMask = 0x07
	// The highest expressible priority: local7 (23) at debug (7).
	maxPriority = 23<<3 | severityMask
)

func (p priority) facility() priority { return p &^ severityMask }
func (p priority) severity() priority { return p & severityMask }

// defaultPriority is user.notice, the priority POSIX-era logger implementations
// use when -p is absent.
const defaultPriority = priority(1<<3 | 5)

// facilities is the standard syslog facility vocabulary. The numbers are the
// wire values, so an unknown NAME must be an error rather than a fallback:
// silently logging an audit record to the wrong facility is worse than not
// logging it, because the operator never learns it went missing.
var facilities = map[string]priority{
	"kern":     0 << 3,
	"user":     1 << 3,
	"mail":     2 << 3,
	"daemon":   3 << 3,
	"auth":     4 << 3,
	"security": 4 << 3, // historical synonym for auth
	"syslog":   5 << 3,
	"lpr":      6 << 3,
	"news":     7 << 3,
	"uucp":     8 << 3,
	"cron":     9 << 3,
	"authpriv": 10 << 3,
	"ftp":      11 << 3,
	"local0":   16 << 3,
	"local1":   17 << 3,
	"local2":   18 << 3,
	"local3":   19 << 3,
	"local4":   20 << 3,
	"local5":   21 << 3,
	"local6":   22 << 3,
	"local7":   23 << 3,
}

// severities is the standard syslog level vocabulary, including the historical
// synonyms every logger accepts.
var severities = map[string]priority{
	"emerg":   0,
	"panic":   0, // historical synonym for emerg
	"alert":   1,
	"crit":    2,
	"err":     3,
	"error":   3, // historical synonym for err
	"warning": 4,
	"warn":    4, // historical synonym for warning
	"notice":  5,
	"info":    6,
	"debug":   7,
}

// parsePriority parses the -p argument. Three spellings are accepted:
//
//	facility.level   the canonical form
//	level            facility defaults to user, as every logger does
//	number           the already-encoded wire value
//
// Anything else — an unknown facility, an unknown level, an out-of-range
// number, more than one separator — is an error. This is deliberate: -p exists
// to route a record, and a mistyped facility that quietly became user.notice
// would put the record somewhere the operator is not looking.
func parsePriority(s string) (priority, error) {
	if s == "" {
		return 0, fmt.Errorf("empty priority")
	}

	// A bare number is the encoded priority itself.
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 || n > maxPriority {
			return 0, fmt.Errorf("numeric priority %d out of range 0-%d", n, maxPriority)
		}
		return priority(n), nil
	}

	facName, levName, hasDot := strings.Cut(s, ".")
	if strings.Contains(levName, ".") {
		return 0, fmt.Errorf("invalid priority %q: expected facility.level", s)
	}

	fac := facilities["user"]
	if hasDot {
		f, ok := facilities[strings.ToLower(facName)]
		if !ok {
			return 0, fmt.Errorf("unknown facility %q (known: %s)", facName, knownNames(facilities))
		}
		fac = f
	} else {
		// No separator: the whole token must be a level. Naming a facility here
		// is a common typo (`logger -p daemon`), so say what is missing rather
		// than reporting it as an unknown level.
		levName = facName
		if _, isFacility := facilities[strings.ToLower(levName)]; isFacility {
			return 0, fmt.Errorf("priority %q names a facility but no level: use %s.<level>", s, strings.ToLower(levName))
		}
	}

	lev, ok := severities[strings.ToLower(levName)]
	if !ok {
		return 0, fmt.Errorf("unknown level %q (known: %s)", levName, knownNames(severities))
	}
	return fac | lev, nil
}

func knownNames(m map[string]priority) string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, " ")
}
