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

// linuxUtmpRecord builds one 384-byte Linux utmp record for fixtures.
func linuxUtmpRecord(typ int16, pid int32, line, id, user, host string, term, exit int16, sec int64) []byte {
	rec := make([]byte, 384)
	put16 := func(off int, v int16) { binary.LittleEndian.PutUint16(rec[off:], uint16(v)) }
	put32 := func(off int, v uint32) { binary.LittleEndian.PutUint32(rec[off:], v) }
	putStr := func(off, n int, s string) {
		b := []byte(s)
		if len(b) > n-1 {
			b = b[:n-1]
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
	put32(340, uint32(sec))
	return rec
}

// TestParseLinuxUtmpRecordTypes proves the binary parser retains every
// record type who needs (boot/dead/login/init/runlevel/time), not only
// USER_PROCESS, and carries PID plus the dead-process exit_status.
func TestParseLinuxUtmpRecordTypes(t *testing.T) {
	var data []byte
	data = append(data, linuxUtmpRecord(2, 0, "~", "~~", "reboot", "", 0, 0, 1000)...)           // BOOT_TIME
	data = append(data, linuxUtmpRecord(1, 0, "~", "~~", "runlevel", "", 0, 0, 1001)...)         // RUN_LVL
	data = append(data, linuxUtmpRecord(6, 111, "tty1", "1", "LOGIN", "", 0, 0, 1002)...)        // LOGIN_PROCESS
	data = append(data, linuxUtmpRecord(5, 222, "", "si", "", "", 0, 0, 1003)...)                // INIT_PROCESS
	data = append(data, linuxUtmpRecord(7, 333, "pts/0", "/0", "bob", "1.2.3.4", 0, 0, 1004)...) // USER_PROCESS
	data = append(data, linuxUtmpRecord(8, 444, "pts/9", "/9", "ghost", "host", 9, 3, 1005)...)  // DEAD_PROCESS
	data = append(data, linuxUtmpRecord(3, 0, "", "", "", "", 0, 0, 1006)...)                    // NEW_TIME
	data = append(data, linuxUtmpRecord(0, 0, "", "", "", "", 0, 0, 0)...)                       // EMPTY -> skipped

	recs := parseLinuxUtmp(data)
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
	if dead.PID != 444 || dead.Term != 9 || dead.Exit != 3 {
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

// darwinUtmpxRecord builds one 628-byte macOS utmpx record for fixtures,
// matching parseDarwinUtmpx's offsets.
func darwinUtmpxRecord(typ int16, user, line, host string, sec int64) []byte {
	rec := make([]byte, 628)
	putStr := func(off, n int, s string) {
		b := []byte(s)
		if len(b) > n-1 {
			b = b[:n-1]
		}
		copy(rec[off:off+n], b)
	}
	putStr(0, 256, user)
	putStr(256, 32, line)
	putStr(296, 256, host)
	binary.LittleEndian.PutUint16(rec[552:], uint16(typ))
	binary.LittleEndian.PutUint32(rec[560:], uint32(sec))
	return rec
}

// TestParseDarwinUtmpxRecordTypes proves the macOS parser likewise retains
// non-USER record types (previously it dropped everything but USER_PROCESS).
func TestParseDarwinUtmpxRecordTypes(t *testing.T) {
	var data []byte
	data = append(data, darwinUtmpxRecord(2, "reboot", "~", "", 1000)...)       // BOOT_TIME
	data = append(data, darwinUtmpxRecord(7, "bob", "ttys000", "h", 1001)...)   // USER_PROCESS
	data = append(data, darwinUtmpxRecord(8, "ghost", "ttys009", "h", 1002)...) // DEAD_PROCESS
	data = append(data, darwinUtmpxRecord(0, "", "", "", 0)...)                 // EMPTY -> skipped

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
}
