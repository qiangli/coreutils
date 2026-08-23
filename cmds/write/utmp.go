package writecmd

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
	"time"
	"unicode"
)

// The login-accounting database is a flat array of fixed-size C structs
// written by the local C library in HOST byte order. There is no header, no
// version field and no self-description: the ONLY way to read it is to know
// the struct layout of the platform that wrote it, which is why the layout is
// data here (utmpLayout) and the decoder below is layout-driven rather than
// per-platform. That keeps both layouts decodable — and therefore testable —
// on every build, while platform_*.go picks the one matching the running
// system's database.
//
// Layouts are transcribed from the DOCUMENTED public struct definitions
// (utmp(5) on Linux, <utmpx.h> on Darwin) and verified against a real database
// by offset. No implementation's source was consulted or copied.
type utmpLayout struct {
	Name string // human name, used in diagnostics
	Size int    // sizeof(struct utmp[x])

	UserOff, UserLen int
	LineOff, LineLen int
	HostOff, HostLen int
	TypeOff          int // int16 ut_type
	PIDOff           int // int32 ut_pid; -1 when the layout has no pid
	TimeOff          int // int32 seconds since the epoch (ut_tv.tv_sec)

	UserProcess int16 // the ut_type value meaning "a user is logged in here"
}

// layoutLinuxUtmp is glibc's `struct utmp` (utmp(5)), 384 bytes:
//
//	off   size  field
//	  0      2  short ut_type
//	  2      2  (padding)
//	  4      4  pid_t ut_pid
//	  8     32  char  ut_line[UT_LINESIZE]
//	 40      4  char  ut_id[4]
//	 44     32  char  ut_user[UT_NAMESIZE]
//	 76    256  char  ut_host[UT_HOSTSIZE]
//	332      4  struct exit_status ut_exit
//	336      4  int32 ut_session
//	340      4  int32 ut_tv.tv_sec
//	344      4  int32 ut_tv.tv_usec
//	348     16  int32 ut_addr_v6[4]
//	364     20  char  __glibc_reserved[20]
//
// ut_tv is a 32-bit pair even on 64-bit Linux, which is why TimeOff is read as
// an int32 and not as the platform's time_t.
var layoutLinuxUtmp = utmpLayout{
	Name: "linux utmp",
	Size: 384,

	UserOff: 44, UserLen: 32,
	LineOff: 8, LineLen: 32,
	HostOff: 76, HostLen: 256,
	TypeOff: 0,
	PIDOff:  4,
	TimeOff: 340,

	UserProcess: 7, // USER_PROCESS
}

// layoutDarwinUtmpx is Darwin's `struct utmpx` (<utmpx.h>), 628 bytes:
//
//	off   size  field
//	  0    256  char   ut_user[_UTX_USERSIZE]
//	256      4  char   ut_id[_UTX_IDSIZE]
//	260     32  char   ut_line[_UTX_LINESIZE]
//	292      4  pid_t  ut_pid
//	296      2  short  ut_type
//	298      2  (padding to 4-byte alignment)
//	300      4  int32  ut_tv.tv_sec
//	304      4  int32  ut_tv.tv_usec
//	308    256  char   ut_host[_UTX_HOSTSIZE]
//	564     64  uint32 ut_pad[16]
//
// The struct is 4-byte aligned, so ut_tv is a 32-bit pair here too and the
// total is 628 rather than the 640 an 8-byte-aligned timeval would produce.
// Verified against a live /var/run/utmpx: the file was 17584 bytes = 28 * 628
// exactly, and at these offsets every record decoded to a real login name, a
// real tty name and a plausible timestamp.
var layoutDarwinUtmpx = utmpLayout{
	Name: "darwin utmpx",
	Size: 628,

	UserOff: 0, UserLen: 256,
	LineOff: 260, LineLen: 32,
	HostOff: 308, HostLen: 256,
	TypeOff: 296,
	PIDOff:  292,
	TimeOff: 300,

	UserProcess: 7, // USER_PROCESS
}

// utmpRecord is one decoded login session.
type utmpRecord struct {
	User string
	Line string // ut_line, e.g. "pts/3" or "ttys004"; never carries a /dev prefix
	Host string
	PID  int
	Time time.Time
}

var errNoLayout = errors.New("no login-accounting database on this platform")

// decodeUtmp decodes every USER_PROCESS record in r using layout.
//
// Records of any other type (BOOT_TIME, DEAD_PROCESS, the utmpx signature
// record, …) are skipped. A DEAD_PROCESS entry keeps the user name of the
// session that ended, so accepting it would let write address a terminal
// nobody is sitting at.
func decodeUtmp(r io.Reader, layout utmpLayout) ([]utmpRecord, error) {
	if layout.Size == 0 {
		return nil, errNoLayout
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	// Host byte order. Every platform this repository targets (amd64, arm64,
	// 386, arm) is little-endian; a big-endian port must revisit this line,
	// because the database is written by the local libc and is not portable
	// across byte orders in the first place.
	order := binary.LittleEndian

	var out []utmpRecord
	for off := 0; off+layout.Size <= len(data); off += layout.Size {
		rec := data[off : off+layout.Size]
		if int16(order.Uint16(rec[layout.TypeOff:layout.TypeOff+2])) != layout.UserProcess {
			continue
		}
		user := cString(rec[layout.UserOff : layout.UserOff+layout.UserLen])
		line := cString(rec[layout.LineOff : layout.LineOff+layout.LineLen])
		if user == "" || line == "" {
			continue
		}
		pid := 0
		if layout.PIDOff >= 0 {
			pid = int(int32(order.Uint32(rec[layout.PIDOff : layout.PIDOff+4])))
		}
		out = append(out, utmpRecord{
			User: user,
			Line: line,
			Host: cString(rec[layout.HostOff : layout.HostOff+layout.HostLen]),
			PID:  pid,
			Time: time.Unix(int64(int32(order.Uint32(rec[layout.TimeOff:layout.TimeOff+4]))), 0),
		})
	}
	return out, nil
}

// readUtmpFile opens path and decodes it.
//
// A missing or unreadable database is reported as an error rather than as an
// empty record set: "the accounting database is not there" and "the user is
// not logged in" are different facts, and collapsing the first into the second
// hands the operator the wrong diagnostic and the wrong fix.
func readUtmpFile(path string, layout utmpLayout) ([]utmpRecord, error) {
	if path == "" {
		return nil, errNoLayout
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return decodeUtmp(f, layout)
}

// encodeUtmp is the inverse of decodeUtmp. It exists so tests can build a
// synthetic database for either layout — without a logged-in user, and without
// reading /var/run — and so both directions are exercised against one set of
// offsets instead of two hand-written ones.
func encodeUtmp(recs []utmpRecord, layout utmpLayout, typ int16) []byte {
	order := binary.LittleEndian
	buf := make([]byte, 0, len(recs)*layout.Size)
	for _, r := range recs {
		rec := make([]byte, layout.Size)
		order.PutUint16(rec[layout.TypeOff:layout.TypeOff+2], uint16(typ))
		putCString(rec[layout.UserOff:layout.UserOff+layout.UserLen], r.User)
		putCString(rec[layout.LineOff:layout.LineOff+layout.LineLen], r.Line)
		putCString(rec[layout.HostOff:layout.HostOff+layout.HostLen], r.Host)
		if layout.PIDOff >= 0 {
			order.PutUint32(rec[layout.PIDOff:layout.PIDOff+4], uint32(int32(r.PID)))
		}
		order.PutUint32(rec[layout.TimeOff:layout.TimeOff+4], uint32(int32(r.Time.Unix())))
		buf = append(buf, rec...)
	}
	return buf
}

func putCString(dst []byte, s string) {
	if len(s) >= len(dst) {
		s = s[:len(dst)-1]
	}
	copy(dst, s)
}

// cString reads a NUL-terminated, NUL-padded C string. Trailing whitespace is
// trimmed as well: some login programs pad ut_line with spaces rather than
// NULs, and a tty named "ttys004   " resolves to no device at all.
func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimRightFunc(string(b), unicode.IsSpace)
}
