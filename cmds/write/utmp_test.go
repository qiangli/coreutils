package writecmd

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Both layouts are exercised on every platform, not just the one that happens
// to be building. A layout is a set of byte offsets: if it only ever runs on
// the machine that wrote it, an offset error in the other one ships silently
// and surfaces as write addressing the wrong terminal on someone else's host.
func layouts() []utmpLayout {
	return []utmpLayout{layoutLinuxUtmpCompat32, layoutLinuxUtmpTime64, layoutDarwinUtmpx}
}

func TestLayoutFieldsStayInsideTheStruct(t *testing.T) {
	for _, l := range layouts() {
		spans := []struct {
			name       string
			off, width int
		}{
			{"ut_user", l.UserOff, l.UserLen},
			{"ut_line", l.LineOff, l.LineLen},
			{"ut_host", l.HostOff, l.HostLen},
			{"ut_type", l.TypeOff, 2},
			{"ut_pid", l.PIDOff, 4},
			{"ut_tv.tv_sec", l.TimeOff, l.TimeLen},
		}
		for _, s := range spans {
			if s.off < 0 || s.off+s.width > l.Size {
				t.Errorf("%s: %s at [%d,%d) overruns the %d-byte struct",
					l.Name, s.name, s.off, s.off+s.width, l.Size)
			}
		}
		// Overlapping fields would decode one field's bytes as another's, which
		// is exactly the failure an offset typo produces.
		for i := range spans {
			for j := i + 1; j < len(spans); j++ {
				a, b := spans[i], spans[j]
				if a.off < b.off+b.width && b.off < a.off+a.width {
					t.Errorf("%s: %s and %s overlap", l.Name, a.name, b.name)
				}
			}
		}
	}
}

func TestDecodeRoundTripsEveryLayout(t *testing.T) {
	want := []utmpRecord{
		{User: "alice", Line: "pts/0", Host: "10.0.0.1", PID: 4242, Time: time.Unix(1_700_000_000, 0)},
		{User: "bob", Line: "ttys004", Host: "", PID: 7, Time: time.Unix(1_700_000_500, 0)},
	}
	for _, l := range layouts() {
		blob := encodeUtmp(want, l, l.UserProcess)
		if len(blob) != len(want)*l.Size {
			t.Fatalf("%s: encoded %d bytes, want %d", l.Name, len(blob), len(want)*l.Size)
		}
		got, err := decodeUtmp(bytes.NewReader(blob), l)
		if err != nil {
			t.Fatalf("%s: %v", l.Name, err)
		}
		if len(got) != len(want) {
			t.Fatalf("%s: decoded %d records, want %d", l.Name, len(got), len(want))
		}
		for i := range want {
			if got[i].User != want[i].User || got[i].Line != want[i].Line ||
				got[i].Host != want[i].Host || got[i].PID != want[i].PID ||
				!got[i].Time.Equal(want[i].Time) {
				t.Errorf("%s record %d: got %+v, want %+v", l.Name, i, got[i], want[i])
			}
		}
	}
}

func TestDecodeKeepsOnlyUserProcessRecords(t *testing.T) {
	for _, l := range layouts() {
		var blob []byte
		blob = append(blob, encodeUtmp([]utmpRecord{{User: "boot", Line: "~"}}, l, 2)...) // BOOT_TIME
		blob = append(blob, encodeUtmp([]utmpRecord{{User: "alice", Line: "pts/0"}}, l, 7)...)
		blob = append(blob, encodeUtmp([]utmpRecord{{User: "bob", Line: "pts/1"}}, l, 8)...) // DEAD_PROCESS
		got, err := decodeUtmp(bytes.NewReader(blob), l)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].User != "alice" {
			t.Errorf("%s: got %+v, want only the USER_PROCESS record", l.Name, got)
		}
	}
}

// A truncated trailing record (the database is being written as it is read)
// must be ignored, not decoded from whatever bytes follow the buffer.
func TestDecodeIgnoresATruncatedTrailingRecord(t *testing.T) {
	for _, l := range layouts() {
		blob := encodeUtmp([]utmpRecord{{User: "alice", Line: "pts/0"}}, l, l.UserProcess)
		blob = append(blob, make([]byte, l.Size/2)...)
		got, err := decodeUtmp(bytes.NewReader(blob), l)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Errorf("%s: decoded %d records from one whole + one partial", l.Name, len(got))
		}
	}
}

// ut_user and ut_line are NUL-padded fixed arrays, and some login programs pad
// with spaces instead. A tty named "pts/0   " resolves to no device at all.
func TestDecodeTrimsPaddingAndSkipsEmptyFields(t *testing.T) {
	l := layoutLinuxUtmpCompat32
	rec := make([]byte, l.Size)
	rec[l.TypeOff] = byte(l.UserProcess)
	copy(rec[l.UserOff:], "alice\x00   ")
	copy(rec[l.LineOff:], "pts/0  \x00")
	got, err := decodeUtmp(bytes.NewReader(rec), l)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].User != "alice" || got[0].Line != "pts/0" {
		t.Fatalf("got %+v, want user=alice line=pts/0", got)
	}

	// A USER_PROCESS record with no line names no terminal; it is not a login
	// this tool can address.
	noLine := make([]byte, l.Size)
	noLine[l.TypeOff] = byte(l.UserProcess)
	copy(noLine[l.UserOff:], "alice")
	if got, _ := decodeUtmp(bytes.NewReader(noLine), l); len(got) != 0 {
		t.Errorf("a record with an empty ut_line must be skipped, got %+v", got)
	}
}

func TestReadUtmpFileReportsAMissingDatabase(t *testing.T) {
	if _, err := readUtmpFile(filepath.Join(t.TempDir(), "nope"), layoutLinuxUtmpCompat32); !os.IsNotExist(err) {
		t.Errorf("missing database: err = %v, want a not-exist error", err)
	}
	if _, err := readUtmpFile("", layoutLinuxUtmpCompat32); err != errNoLayout {
		t.Errorf("no path: err = %v, want errNoLayout", err)
	}
	if _, err := decodeUtmp(bytes.NewReader(nil), utmpLayout{}); err != errNoLayout {
		t.Errorf("zero layout: err = %v, want errNoLayout", err)
	}
}

// The layout the running platform will actually use must be the one this
// package documents for it — a build-tag mistake would otherwise be invisible
// until someone read the wrong offsets on a real host.
func TestActiveLayoutMatchesThePlatform(t *testing.T) {
	switch {
	case platformSupported && defaultUtmpPath == "/var/run/utmp":
		want := layoutLinuxUtmpCompat32.Name
		if runtime.GOARCH == "arm64" {
			want = layoutLinuxUtmpTime64.Name
		}
		if activeUtmpLayout.Name != want {
			t.Errorf("linux/%s selected %q, want %q", runtime.GOARCH, activeUtmpLayout.Name, want)
		}
	default:
		if platformSupported {
			t.Errorf("a supported platform must name its database, got %q", defaultUtmpPath)
		}
		if activeUtmpLayout.Size != 0 || errPlatform == nil {
			t.Error("an unsupported platform must carry no layout and a refusal reason")
		}
	}
}
