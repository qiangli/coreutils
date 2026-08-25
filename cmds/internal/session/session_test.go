package session

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestReadTextRecords(t *testing.T) {
	p := filepath.Join(t.TempDir(), "utmp.txt")
	if err := os.WriteFile(p, []byte("alice tty1 100 host\nbob pts/2 200 remote user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].User != "alice" || records[1].Host != "remote" {
		t.Fatalf("records=%#v", records)
	}
	users, err := Users(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 || users[0] != "alice" || users[1] != "bob" {
		t.Fatalf("users=%#v", users)
	}
}

// linuxABIRecord builds records from the independently measured glibc ABI
// table used by the adversarial review. It deliberately does not consume the
// parser's linuxLayout, so a parser/layout regression cannot rewrite its own
// oracle.
func linuxABIRecord(goarch string, typ int16, pid int32, line, id, user, host string, term, exit int16, sec int64) []byte {
	size, secOff, secSize := 384, 340, 4
	var order binary.ByteOrder = binary.LittleEndian
	switch goarch {
	case "arm64", "riscv64", "loong64", "mips64", "mips64le":
		size, secOff, secSize = 400, 344, 8
	}
	switch goarch {
	case "s390x", "ppc", "ppc64", "mips", "mips64":
		order = binary.BigEndian
	}
	rec := make([]byte, size)
	put16 := func(off int, v int16) { order.PutUint16(rec[off:], uint16(v)) }
	put32 := func(off int, v uint32) { order.PutUint32(rec[off:], v) }
	putStr := func(off, n int, s string) {
		b := []byte(s)
		if len(b) > n {
			b = b[:n]
		}
		copy(rec[off:off+n], b)
	}
	put16(0, typ)
	put32(4, uint32(pid))
	putStr(8, 32, line)
	putStr(40, 4, id)
	putStr(44, 32, user)
	putStr(76, 256, host)
	put16(332, term)
	put16(334, exit)
	if secSize == 8 {
		order.PutUint64(rec[secOff:], uint64(sec))
	} else {
		put32(secOff, uint32(sec))
	}
	return rec
}

// TestParseLinuxUtmpRecordTypes proves the binary parser retains every
// record type who needs (boot/dead/login/init/runlevel/time), not only
// USER_PROCESS, and carries PID plus the dead-process exit_status.
func TestParseLinuxUtmpRecordTypes(t *testing.T) {
	const arch = "arm64" // Ubuntu 24.04 aarch64 ABI: 400-byte time64 records.
	var data []byte
	data = append(data, linuxABIRecord(arch, 2, 0, "~", "~~", "reboot", "", 0, 0, 1000)...)             // BOOT_TIME
	data = append(data, linuxABIRecord(arch, 1, 'S'+'5'*256, "~", "~~", "runlevel", "", 0, 0, 1001)...) // RUN_LVL
	data = append(data, linuxABIRecord(arch, 6, 111, "tty1", "1", "LOGIN", "", 0, 0, 1002)...)          // LOGIN_PROCESS
	data = append(data, linuxABIRecord(arch, 5, 222, "", "si", "", "", 0, 0, 1003)...)                  // INIT_PROCESS
	data = append(data, linuxABIRecord(arch, 7, 333, "pts/0", "/0", "bob", "1.2.3.4", 0, 0, 1004)...)   // USER_PROCESS
	data = append(data, linuxABIRecord(arch, 8, 444, "pts/9", "/9", "ghost", "host", 9, 3, 1005)...)    // DEAD_PROCESS
	data = append(data, linuxABIRecord(arch, 3, 0, "", "", "", "", 0, 0, 1006)...)                      // NEW_TIME
	data = append(data, linuxABIRecord(arch, 0, 0, "", "", "", "", 0, 0, 0)...)                         // EMPTY -> skipped

	recs := parseLinuxUtmpArch(data, arch)
	if len(recs) != 7 {
		t.Fatalf("got %d records, want 7 (EMPTY skipped): %#v", len(recs), recs)
	}

	byType := map[string]Record{}
	for _, r := range recs {
		byType[r.Type] = r
	}
	for _, want := range []string{"BOOT_TIME", "RUN_LVL", "LOGIN_PROCESS", "INIT_PROCESS", "USER_PROCESS", "DEAD_PROCESS", "NEW_TIME"} {
		if _, ok := byType[want]; !ok {
			t.Fatalf("missing record type %s in %#v", want, byType)
		}
	}

	user := byType["USER_PROCESS"]
	if user.User != "bob" || user.TTY != "pts/0" || user.PID != 333 || user.Host != "1.2.3.4" {
		t.Fatalf("USER_PROCESS mis-parsed: %#v", user)
	}
	if !IsUser(user) {
		t.Fatalf("USER_PROCESS should satisfy IsUser: %#v", user)
	}

	dead := byType["DEAD_PROCESS"]
	if dead.PID != 444 || dead.Term != 9 || dead.Exit != 3 || !dead.ExitKnown {
		t.Fatalf("DEAD_PROCESS exit_status mis-parsed: %#v", dead)
	}
	if IsUser(dead) {
		t.Fatalf("DEAD_PROCESS must not count as a user: %#v", dead)
	}

	// Only the USER_PROCESS record should be reported by Users-style filtering.
	nUsers := 0
	for _, r := range recs {
		if IsUser(r) {
			nUsers++
		}
	}
	if nUsers != 1 {
		t.Fatalf("expected exactly 1 user among mixed types, got %d", nUsers)
	}
}

// darwinABIRecord uses the offsets measured with offsetof(struct utmpx) on
// macOS libc, including ut_id and ut_pid. These are an external ABI oracle,
// not constants imported from the parser.
func darwinABIRecord(typ int16, pid int32, user, id, line, host string, sec int64) []byte {
	rec := make([]byte, 628)
	putStr := func(off, n int, s string) {
		b := []byte(s)
		if len(b) > n {
			b = b[:n]
		}
		copy(rec[off:off+n], b)
	}
	putStr(0, 256, user)
	putStr(256, 4, id)
	putStr(260, 32, line)
	binary.LittleEndian.PutUint32(rec[292:], uint32(pid))
	binary.LittleEndian.PutUint16(rec[296:], uint16(typ))
	binary.LittleEndian.PutUint32(rec[300:], uint32(sec))
	putStr(308, 256, host)
	return rec
}

// TestParseDarwinUtmpxRecordTypes proves the macOS parser likewise retains
// non-USER record types (previously it dropped everything but USER_PROCESS).
func TestParseDarwinUtmpxRecordTypes(t *testing.T) {
	var data []byte
	data = append(data, darwinABIRecord(2, 1, "reboot", "~~", "~", "", 1000)...)          // BOOT_TIME
	data = append(data, darwinABIRecord(7, 42, "bob", "s000", "ttys000", "h", 1001)...)   // USER_PROCESS
	data = append(data, darwinABIRecord(8, 43, "ghost", "s009", "ttys009", "h", 1002)...) // DEAD_PROCESS
	data = append(data, darwinABIRecord(10, 0, "utmpx-1.00", "", "", "", 0)...)           // SIGNATURE -> skipped
	data = append(data, darwinABIRecord(0, 0, "", "", "", "", 0)...)                      // EMPTY -> skipped

	recs := parseDarwinUtmpx(data)
	if len(recs) != 3 {
		t.Fatalf("got %d records, want 3 (EMPTY skipped): %#v", len(recs), recs)
	}
	types := map[string]bool{}
	for _, r := range recs {
		types[r.Type] = true
	}
	for _, want := range []string{"BOOT_TIME", "USER_PROCESS", "DEAD_PROCESS"} {
		if !types[want] {
			t.Fatalf("missing record type %s: %#v", want, recs)
		}
	}
	if got := recs[1]; got.ID != "s000" || got.PID != 42 || got.TTY != "ttys000" || got.Host != "h" {
		t.Fatalf("darwin USER_PROCESS ABI fields mis-parsed: %#v", got)
	}
	if recs[2].ExitKnown {
		t.Fatalf("darwin must not claim unavailable ut_exit: %#v", recs[2])
	}
}

func TestLinuxUtmpLayoutMatrix(t *testing.T) {
	cases := []struct {
		arch         string
		size, sec, n int
		big          bool
	}{
		{"amd64", 384, 340, 4, false},
		{"arm64", 400, 344, 8, false},
		{"s390x", 384, 340, 4, true},
		{"mips64", 400, 344, 8, true},
	}
	for _, tc := range cases {
		got, ok := linuxUtmpLayout(tc.arch)
		if !ok || got.size != tc.size || got.secOffset != tc.sec || got.secSize != tc.n || (got.order == binary.BigEndian) != tc.big {
			t.Fatalf("linuxUtmpLayout(%s)=%#v,%v", tc.arch, got, ok)
		}
	}
	if _, ok := linuxUtmpLayout("wasm"); ok {
		t.Fatal("unsupported architecture silently accepted")
	}
}
