package session

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
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
	// ExitKnown distinguishes a real ut_exit value from platforms whose
	// session ABI has no exit-status field. Consumers must not print a
	// fabricated zero status when this is false.
	ExitKnown bool
}

var ErrUnsupportedFormat = errors.New("session database format is unsupported on this platform")

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
	case "darwin":
		return "/var/run/utmpx"
	case "freebsd":
		return "/var/run/utx.active"
	case "netbsd":
		return "/var/run/utmpx"
	default:
		return ""
	}
}

func Read(path string) ([]Record, error) {
	explicit := path != ""
	if path == "" {
		path = DefaultFile()
	}
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !explicit && (os.IsNotExist(err) || os.IsPermission(err)) {
			return nil, nil
		}
		return nil, err
	}
	if textRecords(data) {
		return parseText(data), nil
	}
	return parseBinary(data)
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
		var metadata []string
		var haveTerm, haveExit bool
		if len(f) > 5 {
			metadata = f[5:]
		}
		for _, field := range metadata {
			key, value, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			switch key {
			case "id":
				r.ID = value
			case "pid":
				r.PID, _ = strconv.Atoi(value)
			case "term":
				if parsed, err := strconv.Atoi(value); err == nil {
					r.Term, haveTerm = parsed, true
				}
			case "exit":
				if parsed, err := strconv.Atoi(value); err == nil {
					r.Exit, haveExit = parsed, true
				}
			}
		}
		r.ExitKnown = haveTerm && haveExit
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

func parseBinary(data []byte) ([]Record, error) {
	switch runtime.GOOS {
	case "linux":
		if _, ok := linuxUtmpLayout(runtime.GOARCH); !ok {
			return nil, fmt.Errorf("%w: linux/%s", ErrUnsupportedFormat, runtime.GOARCH)
		}
		return parseLinuxUtmp(data), nil
	case "darwin":
		return parseDarwinUtmpx(data), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, runtime.GOOS)
	}
}

type linuxLayout struct {
	size, secOffset, secSize int
	order                    binary.ByteOrder
}

// linuxUtmpLayout captures glibc's two utmp ABIs. Architectures with
// __WORDSIZE_TIME64_COMPAT32 retain the historical 384-byte record and
// 32-bit timeval. Native time64 architectures use a 400-byte record.
func linuxUtmpLayout(goarch string) (linuxLayout, bool) {
	var order binary.ByteOrder = binary.LittleEndian
	switch goarch {
	case "s390x", "ppc", "ppc64", "mips", "mips64":
		order = binary.BigEndian
	}
	switch goarch {
	case "arm64", "riscv64", "loong64", "mips64", "mips64le":
		return linuxLayout{size: 400, secOffset: 344, secSize: 8, order: order}, true
	case "amd64", "386", "arm", "ppc", "ppc64", "ppc64le", "s390x", "mips", "mipsle":
		return linuxLayout{size: 384, secOffset: 340, secSize: 4, order: order}, true
	default:
		return linuxLayout{}, false
	}
}

// parseLinuxUtmp decodes Linux's architecture-specific glibc utmp/utmpx
// on-disk format (384-byte compat-time or 400-byte native-time64 records).
// Every non-EMPTY record type is retained and tagged
// with its canonical name so who's -b/-d/-l/-p/-r/-t selectors have data
// to work with; PID and the dead-process exit_status are carried through.
func parseLinuxUtmp(data []byte) []Record {
	return parseLinuxUtmpArch(data, runtime.GOARCH)
}

func parseLinuxUtmpArch(data []byte, goarch string) []Record {
	layout, ok := linuxUtmpLayout(goarch)
	if !ok {
		return nil
	}
	var out []Record
	for off := 0; off+layout.size <= len(data); off += layout.size {
		rec := data[off : off+layout.size]
		typ := int16(layout.order.Uint16(rec[0:2]))
		if typ == 0 { // EMPTY: unused slot
			continue
		}
		kind := utmpType(typ)
		if kind == "EMPTY" { // unknown/vendor record, not a POSIX who entry
			continue
		}
		pid := int32(layout.order.Uint32(rec[4:8]))
		line := cString(rec[8 : 8+32])
		id := cString(rec[40 : 40+4])
		user := cString(rec[44 : 44+32])
		host := cString(rec[76 : 76+256])
		term := int16(layout.order.Uint16(rec[332:334]))
		exit := int16(layout.order.Uint16(rec[334:336]))
		var sec int64
		if layout.secSize == 8 {
			sec = int64(layout.order.Uint64(rec[layout.secOffset : layout.secOffset+8]))
		} else {
			sec = int64(int32(layout.order.Uint32(rec[layout.secOffset : layout.secOffset+4])))
		}
		out = append(out, Record{
			User: user, TTY: line, Host: host, ID: id,
			Time: time.Unix(sec, 0), Type: kind,
			PID: int(pid), Term: int(term), Exit: int(exit), ExitKnown: true,
		})
	}
	return out
}

// parseDarwinUtmpx decodes the macOS utmpx on-disk format (628 bytes/record).
// Like the Linux path it now retains every non-EMPTY record type (previously
// only USER_PROCESS survived, which silently starved -b/-d/-l/-p/-r/-t). The
// macOS utmpx carries no ut_exit field, so ExitKnown remains false.
func parseDarwinUtmpx(data []byte) []Record {
	const size = 628
	var out []Record
	for off := 0; off+size <= len(data); off += size {
		rec := data[off : off+size]
		user := cString(rec[0:256])
		id := cString(rec[256 : 256+4])
		line := cString(rec[260 : 260+32])
		pid := int32(binary.LittleEndian.Uint32(rec[292:296]))
		typ := int16(binary.LittleEndian.Uint16(rec[296:298]))
		sec := int64(int32(binary.LittleEndian.Uint32(rec[300:304])))
		host := cString(rec[308 : 308+256])
		if typ == 0 { // EMPTY: unused slot
			continue
		}
		kind := utmpType(typ)
		if kind == "EMPTY" { // includes Apple's SIGNATURE record (type 10)
			continue
		}
		out = append(out, Record{
			User: user, TTY: line, Host: host, ID: id, PID: int(pid),
			Time: time.Unix(sec, 0), Type: kind,
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
