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
	Time time.Time
	Type string
	PID  int
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

func parseLinuxUtmp(data []byte) []Record {
	const size = 384
	var out []Record
	for off := 0; off+size <= len(data); off += size {
		rec := data[off : off+size]
		typ := int16(binary.LittleEndian.Uint16(rec[0:2]))
		if typ != 7 {
			continue
		}
		user := cString(rec[44 : 44+32])
		line := cString(rec[8 : 8+32])
		host := cString(rec[76 : 76+256])
		sec := int64(binary.LittleEndian.Uint32(rec[340:344]))
		if user != "" {
			out = append(out, Record{User: user, TTY: line, Host: host, Time: time.Unix(sec, 0), Type: "user"})
		}
	}
	return out
}

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
		if user != "" && (typ == 7 || typ == 0) {
			out = append(out, Record{User: user, TTY: line, Host: host, Time: time.Unix(sec, 0), Type: "user"})
		}
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
