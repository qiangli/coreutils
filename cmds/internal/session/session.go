package session

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Record struct {
	User string
	TTY  string
	Host string
	ID   string
	Time time.Time
	Type string
	PID  int
	// Term and Exit carry the exit_status of a DEAD_PROCESS record
	// (ut_exit.e_termination / ut_exit.e_exit on Linux utmp). They are
	// only meaningful when Type is DEAD_PROCESS; who -d reports them.
	Term int
	Exit int
}

// utmpType maps a binary ut_type value to the canonical POSIX record-type
// name used throughout who's filtering. Keeping the names (rather than a
// single "user" bucket) is what lets who honor -b/-d/-l/-p/-r/-t.
func utmpType(t int16) string {
	switch t {
	case 1:
		return "RUN_LVL"
	case 2:
		return "BOOT_TIME"
	case 3:
		return "NEW_TIME"
	case 4:
		return "OLD_TIME"
	case 5:
		return "INIT_PROCESS"
	case 6:
		return "LOGIN_PROCESS"
	case 7:
		return "USER_PROCESS"
	case 8:
		return "DEAD_PROCESS"
	case 9:
		return "ACCOUNTING"
	default:
		return "EMPTY"
	}
}

func DefaultFile() string {
	switch runtime.GOOS {
	case "linux":
		return "/var/run/utmp"
	case "darwin", "freebsd", "netbsd":
		return "/var/run/utmpx"
	default:
		return ""
	}
}

func Read(path string) ([]Record, error) {
	if path == "" {
		path = DefaultFile()
	}
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return nil, nil
		}
		return nil, err
	}
	if textRecords(data) {
		return parseText(data), nil
	}
	return parseBinary(data), nil
}

func Users(path string) ([]string, error) {
	records, err := Read(path)
	if err != nil {
		return nil, err
	}
	var users []string
	for _, r := range records {
		if IsUser(r) {
			users = append(users, r.User)
		}
	}
	sort.Strings(users)
	return users, nil
}

func IsUser(r Record) bool {
	return r.User != "" && (r.Type == "" || r.Type == "user" || r.Type == "USER_PROCESS")
}

func parseText(data []byte) []Record {
	var records []Record
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		r := Record{User: f[0], TTY: f[1], Type: "user"}
		if len(f) > 2 {
			if sec, err := strconv.ParseInt(f[2], 10, 64); err == nil {
				r.Time = time.Unix(sec, 0)
			} else {
				r.Host = f[2]
			}
		}
		if len(f) > 3 {
			r.Host = f[3]
		}
		if len(f) > 4 {
			r.Type = f[4]
		}
		records = append(records, r)
	}
	return records
}

func textRecords(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	nul := bytes.IndexByte(data, 0)
	if nul >= 0 && nul < 128 {
		return false
	}
	for _, b := range data {
		if b == '\n' || b == '\t' || b == '\r' {
			continue
		}
		if b < 0x20 || b > 0x7e {
			return false
		}
	}
	return true
}

func parseBinary(data []byte) []Record {
	switch runtime.GOOS {
	case "linux":
		return parseLinuxUtmp(data)
	case "darwin":
		return parseDarwinUtmpx(data)
	default:
		return nil
	}
}

// parseLinuxUtmp decodes the Linux utmp/utmpx on-disk format (struct utmp,
// 384 bytes/record). Every non-EMPTY record type is retained and tagged
// with its canonical name so who's -b/-d/-l/-p/-r/-t selectors have data
// to work with; PID and the dead-process exit_status are carried through.
func parseLinuxUtmp(data []byte) []Record {
	const size = 384
	var out []Record
	for off := 0; off+size <= len(data); off += size {
		rec := data[off : off+size]
		typ := int16(binary.LittleEndian.Uint16(rec[0:2]))
		if typ == 0 { // EMPTY: unused slot
			continue
		}
		pid := int32(binary.LittleEndian.Uint32(rec[4:8]))
		line := cString(rec[8 : 8+32])
		id := cString(rec[40 : 40+4])
		user := cString(rec[44 : 44+32])
		host := cString(rec[76 : 76+256])
		term := int16(binary.LittleEndian.Uint16(rec[332:334]))
		exit := int16(binary.LittleEndian.Uint16(rec[334:336]))
		sec := int64(binary.LittleEndian.Uint32(rec[340:344]))
		out = append(out, Record{
			User: user, TTY: line, Host: host, ID: id,
			Time: time.Unix(sec, 0), Type: utmpType(typ),
			PID: int(pid), Term: int(term), Exit: int(exit),
		})
	}
	return out
}

// parseDarwinUtmpx decodes the macOS utmpx on-disk format (628 bytes/record).
// Like the Linux path it now retains every non-EMPTY record type (previously
// only USER_PROCESS survived, which silently starved -b/-d/-l/-p/-r/-t). The
// byte offsets are unchanged from the validated layout; macOS utmpx carries no
// ut_exit field, so Term/Exit stay zero.
func parseDarwinUtmpx(data []byte) []Record {
	const size = 628
	var out []Record
	for off := 0; off+size <= len(data); off += size {
		rec := data[off : off+size]
		user := cString(rec[0:256])
		line := cString(rec[256 : 256+32])
		host := cString(rec[296 : 296+256])
		typ := int16(binary.LittleEndian.Uint16(rec[552:554]))
		sec := int64(binary.LittleEndian.Uint32(rec[560:564]))
		if typ == 0 { // EMPTY: unused slot
			continue
		}
		out = append(out, Record{
			User: user, TTY: line, Host: host,
			Time: time.Unix(sec, 0), Type: utmpType(typ),
		})
	}
	return out
}

func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimRightFunc(string(b), func(r rune) bool { return r == 0 || unicode.IsSpace(r) })
}

func TTYPath(tty string) string {
	if tty == "" {
		return ""
	}
	if filepath.IsAbs(tty) {
		return tty
	}
	return filepath.Join("/dev", tty)
}
