package paxcmd

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// ---------------------------------------------------------------------------
// cpio construction helpers
// ---------------------------------------------------------------------------

type cpioSpec struct {
	name  string
	mode  uint64
	uid   uint64
	gid   uint64
	nlink uint64
	dev   uint64
	ino   uint64
	mtime uint64
	data  []byte
	// crc, when set, corrupts the 070702 check field so the mismatch path is
	// reachable.
	badCRC bool
}

func (s cpioSpec) modeOrDefault() uint64 {
	if s.mode == 0 {
		return 0o100644
	}
	return s.mode
}

func (s cpioSpec) nlinkOrDefault() uint64 {
	if s.nlink == 0 {
		return 1
	}
	return s.nlink
}

// buildNewc assembles a newc (070701) or newc-CRC (070702) archive.
func buildNewc(t *testing.T, crc bool, specs []cpioSpec) []byte {
	t.Helper()
	magic := "070701"
	if crc {
		magic = "070702"
	}
	var out bytes.Buffer
	emit := func(s cpioSpec) {
		check := uint64(0)
		if crc {
			check = uint64(cpioDataChecksum(s.data))
			if s.badCRC {
				check++
			}
		}
		fields := []uint64{
			s.ino, s.modeOrDefault(), s.uid, s.gid, s.nlinkOrDefault(), s.mtime,
			uint64(len(s.data)), s.dev >> 32, s.dev & 0xffffffff, 0, 0,
			uint64(len(s.name) + 1), check,
		}
		out.WriteString(magic)
		for _, f := range fields {
			fmt.Fprintf(&out, "%08X", f)
		}
		out.WriteString(s.name)
		out.WriteByte(0)
		for out.Len()%4 != 0 {
			out.WriteByte(0)
		}
		out.Write(s.data)
		for out.Len()%4 != 0 {
			out.WriteByte(0)
		}
	}
	for _, s := range specs {
		emit(s)
	}
	emit(cpioSpec{name: "TRAILER!!!"})
	return out.Bytes()
}

// buildODC assembles a POSIX octet-oriented (070707) archive.
func buildODC(t *testing.T, specs []cpioSpec) []byte {
	t.Helper()
	var out bytes.Buffer
	emit := func(s cpioSpec) {
		fmt.Fprintf(&out, "070707%06o%06o%06o%06o%06o%06o%06o%011o%06o%011o",
			s.dev, s.ino, s.modeOrDefault(), s.uid, s.gid, s.nlinkOrDefault(), 0,
			s.mtime, len(s.name)+1, len(s.data))
		out.WriteString(s.name)
		out.WriteByte(0)
		out.Write(s.data)
	}
	for _, s := range specs {
		emit(s)
	}
	emit(cpioSpec{name: "TRAILER!!!"})
	return out.Bytes()
}

func writeFileAt(t *testing.T, dir, name, body string) string {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func listNames(t *testing.T, data []byte) []string {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(data))
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return names
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
}

// ---------------------------------------------------------------------------
// -u write lane
// ---------------------------------------------------------------------------

// TestWriteUpdateSupersedesOnlyNewerNames walks the three orderings a -u write
// has to distinguish and then proves the archive it produced is still usable:
// listing shows both physical copies, extraction materializes exactly the
// surviving one.
func TestWriteUpdateSupersedesOnlyNewerNames(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	for _, tc := range []struct {
		name       string
		shift      time.Duration
		superseded bool
	}{
		{"older", -time.Hour, false},
		{"equal", 0, false},
		{"newer", time.Hour, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			arc := filepath.Join(d, "archive.tar")
			path := writeFileAt(t, d, "file", "original")
			if err := os.Chtimes(path, base, base); err != nil {
				t.Fatal(err)
			}
			if _, errs, code := exec(t, d, "", "-w", "-f", arc, "file"); code != 0 {
				t.Fatalf("create: %d %s", code, errs)
			}
			if err := os.WriteFile(path, []byte("candidate"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(path, base.Add(tc.shift), base.Add(tc.shift)); err != nil {
				t.Fatal(err)
			}
			if _, errs, code := exec(t, d, "", "-w", "-u", "-f", arc, "file"); code != 0 {
				t.Fatalf("update: %d %s", code, errs)
			}

			data, err := os.ReadFile(arc)
			if err != nil {
				t.Fatal(err)
			}
			names := listNames(t, data)
			wantCopies := 1
			if tc.superseded {
				wantCopies = 2
			}
			if len(names) != wantCopies {
				t.Fatalf("archive members = %v, want %d copies", names, wantCopies)
			}

			// The archive must still LIST, one line per physical member.
			out, errs, code := exec(t, d, "", "-f", arc)
			if code != 0 || len(strings.Fields(out)) != wantCopies {
				t.Fatalf("list: code=%d out=%q err=%q", code, out, errs)
			}

			// And it must still EXTRACT, yielding the surviving copy only.
			dest := t.TempDir()
			if _, errs, code := exec(t, dest, "", "-r", "-f", arc); code != 0 {
				t.Fatalf("extract: %d %s", code, errs)
			}
			got, err := os.ReadFile(filepath.Join(dest, "file"))
			if err != nil {
				t.Fatal(err)
			}
			want := "original"
			if tc.superseded {
				want = "candidate"
			}
			if string(got) != want {
				t.Fatalf("extracted %q, want %q", got, want)
			}
		})
	}
}

func TestExtractRegularHardlinkUpdateTransitions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		headers    []*tar.Header
		bodies     []string
		want       string
		wantLinked bool
	}{
		{
			name: "regular-to-hardlink",
			headers: []*tar.Header{
				{Name: "source", Typeflag: tar.TypeReg, Mode: 0o644, Size: 6},
				{Name: "updated", Typeflag: tar.TypeReg, Mode: 0o644, Size: 5},
				{Name: "updated", Typeflag: tar.TypeLink, Linkname: "source", Mode: 0o644},
			},
			bodies:     []string{"shared", "stale", ""},
			want:       "shared",
			wantLinked: true,
		},
		{
			name: "hardlink-to-regular",
			headers: []*tar.Header{
				{Name: "source", Typeflag: tar.TypeReg, Mode: 0o644, Size: 6},
				{Name: "updated", Typeflag: tar.TypeLink, Linkname: "source", Mode: 0o644},
				{Name: "updated", Typeflag: tar.TypeReg, Mode: 0o644, Size: 6},
			},
			bodies: []string{"shared", "", "newest"},
			want:   "newest",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var archive bytes.Buffer
			tw := tar.NewWriter(&archive)
			for i, h := range tc.headers {
				if err := tw.WriteHeader(h); err != nil {
					t.Fatal(err)
				}
				if tc.bodies[i] != "" {
					if _, err := tw.Write([]byte(tc.bodies[i])); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}

			d := t.TempDir()
			arc := filepath.Join(d, "history.tar")
			if err := os.WriteFile(arc, archive.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			out, errs, code := exec(t, d, "", "-f", arc)
			if code != 0 || strings.Join(strings.Fields(out), ",") != "source,updated,updated" {
				t.Fatalf("list: code=%d out=%q err=%q", code, out, errs)
			}
			dest := t.TempDir()
			if _, errs, code := exec(t, dest, "", "-r", "-f", arc); code != 0 {
				t.Fatalf("extract: %d %s", code, errs)
			}
			if got := string(mustRead(t, filepath.Join(dest, "updated"))); got != tc.want {
				t.Fatalf("updated content = %q, want %q", got, tc.want)
			}
			if tc.wantLinked {
				source, err := os.Stat(filepath.Join(dest, "source"))
				if err != nil {
					t.Fatal(err)
				}
				updated, err := os.Stat(filepath.Join(dest, "updated"))
				if err != nil {
					t.Fatal(err)
				}
				if !os.SameFile(source, updated) {
					t.Fatal("newest hardlink occurrence was not materialized as a hardlink")
				}
			}
		})
	}
}

func TestWriteUpdateCanMoveHardlinkDataCarrier(t *testing.T) {
	d := t.TempDir()
	arc := filepath.Join(d, "archive.tar")
	first := writeFileAt(t, d, "first", "old")
	second := filepath.Join(d, "second")
	if err := os.Link(first, second); err != nil {
		t.Skipf("hardlinks unsupported here: %v", err)
	}
	base := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(first, base, base); err != nil {
		t.Fatal(err)
	}
	if _, errs, code := exec(t, d, "", "-w", "-f", arc, "first", "second"); code != 0 {
		t.Fatalf("create: %d %s", code, errs)
	}

	// Reverse the inode group's operand order during the update. The original
	// data carrier (first) becomes a hardlink occurrence, and the original
	// hardlink (second) becomes the new regular data carrier.
	if err := os.WriteFile(first, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(first, base.Add(time.Hour), base.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, errs, code := exec(t, d, "", "-w", "-u", "-f", arc, "second", "first"); code != 0 {
		t.Fatalf("update: %d %s", code, errs)
	}
	if out, errs, code := exec(t, d, "", "-f", arc); code != 0 || strings.Join(strings.Fields(out), ",") != "first,second,second,first" {
		t.Fatalf("list: code=%d out=%q err=%q", code, out, errs)
	}

	dest := t.TempDir()
	if _, errs, code := exec(t, dest, "", "-r", "-f", arc); code != 0 {
		t.Fatalf("extract: %d %s", code, errs)
	}
	if got := string(mustRead(t, filepath.Join(dest, "first"))); got != "new" {
		t.Fatalf("first content = %q", got)
	}
	if got := string(mustRead(t, filepath.Join(dest, "second"))); got != "new" {
		t.Fatalf("second content = %q", got)
	}
	firstInfo, err := os.Stat(filepath.Join(dest, "first"))
	if err != nil {
		t.Fatal(err)
	}
	secondInfo, err := os.Stat(filepath.Join(dest, "second"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(firstInfo, secondInfo) {
		t.Fatal("updated inode group was not restored as hardlinks")
	}
}

func TestRegularHardlinkCompatibilitySkipsOtherTypeChangesAndUnsafeTargets(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers []*tar.Header
		want    string
	}{
		{
			name: "regular-to-symlink",
			headers: []*tar.Header{
				{Name: "innocent", Typeflag: tar.TypeReg, Mode: 0o644, Size: 2},
				{Name: "changed", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1},
				{Name: "changed", Typeflag: tar.TypeSymlink, Linkname: "innocent"},
			},
			want: "duplicate destination",
		},
		{
			name: "unsafe-hardlink-target",
			headers: []*tar.Header{
				{Name: "innocent", Typeflag: tar.TypeReg, Mode: 0o644, Size: 2},
				{Name: "changed", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1},
				{Name: "changed", Typeflag: tar.TypeLink, Linkname: "../escape"},
			},
			want: "hardlink target",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var archive bytes.Buffer
			tw := tar.NewWriter(&archive)
			for _, h := range tc.headers {
				if err := tw.WriteHeader(h); err != nil {
					t.Fatal(err)
				}
				if h.Size != 0 {
					if _, err := tw.Write(bytes.Repeat([]byte("x"), int(h.Size))); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			dest := t.TempDir()
			_, errs, code := exec(t, dest, string(archive.Bytes()), "-r")
			if code == 0 || !strings.Contains(errs, tc.want) {
				t.Fatalf("extract: code=%d stderr=%q, want %q rejection", code, errs, tc.want)
			}
			if got, err := os.ReadFile(filepath.Join(dest, "innocent")); err != nil || string(got) != "xx" {
				t.Fatalf("safe member = (%q, %v), want extracted", got, err)
			}
			if _, err := os.Lstat(filepath.Join(dest, "changed")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("rejected target mutated destination: %v", err)
			}
		})
	}
}

// -u compares the name the member will carry IN THE ARCHIVE, so a -s rewrite
// has to be applied before the mtime lookup or every rewritten member would
// look new.
func TestWriteUpdateComparesSubstitutedNames(t *testing.T) {
	d := t.TempDir()
	arc := filepath.Join(d, "archive.tar")
	base := time.Unix(1_700_000_000, 0)
	src := writeFileAt(t, d, "src", "first")
	if err := os.Chtimes(src, base, base); err != nil {
		t.Fatal(err)
	}
	if _, errs, code := exec(t, d, "", "-w", "-f", arc, "-s", "/src/renamed/", "src"); code != 0 {
		t.Fatalf("create: %d %s", code, errs)
	}
	if names := listNames(t, mustRead(t, arc)); len(names) != 1 || names[0] != "renamed" {
		t.Fatalf("archive members = %v", names)
	}

	// Same mtime under the rewritten name: nothing may be appended.
	if _, errs, code := exec(t, d, "", "-w", "-u", "-f", arc, "-s", "/src/renamed/", "src"); code != 0 {
		t.Fatalf("equal update: %d %s", code, errs)
	}
	if names := listNames(t, mustRead(t, arc)); len(names) != 1 {
		t.Fatalf("equal update appended: %v", names)
	}

	// Newer under the rewritten name: the copy is appended and supersedes.
	if err := os.WriteFile(src, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(src, base.Add(time.Hour), base.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, errs, code := exec(t, d, "", "-w", "-u", "-f", arc, "-s", "/src/renamed/", "src"); code != 0 {
		t.Fatalf("newer update: %d %s", code, errs)
	}
	if names := listNames(t, mustRead(t, arc)); len(names) != 2 {
		t.Fatalf("newer update did not append: %v", names)
	}
	dest := t.TempDir()
	if _, errs, code := exec(t, dest, "", "-r", "-f", arc); code != 0 {
		t.Fatalf("extract: %d %s", code, errs)
	}
	if got := mustRead(t, filepath.Join(dest, "renamed")); string(got) != "second" {
		t.Fatalf("extracted %q, want %q", got, "second")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// -u has to read the archive it is updating, so a pipe cannot satisfy it. The
// refusal must be loud rather than a silent full rewrite that ignores -u.
func TestWriteUpdateRequiresSeekableArchive(t *testing.T) {
	d := t.TempDir()
	writeFileAt(t, d, "file", "x")
	if _, errs, code := exec(t, d, "", "-w", "-u", "file"); code == 0 || !strings.Contains(errs, "seekable") {
		t.Fatalf("nonseekable update: code=%d stderr=%q", code, errs)
	}
	if _, errs, code := exec(t, d, "", "-w", "-u", "-f", "-", "file"); code != 0 {
		t.Fatalf("explicit dash pathname update: code=%d stderr=%q", code, errs)
	}
	if names := listNames(t, mustRead(t, filepath.Join(d, "-"))); len(names) != 1 || names[0] != "file" {
		t.Fatalf("dash pathname archive members = %v", names)
	}
	// A seekable target that does not exist yet is a plain create.
	arc := filepath.Join(d, "archive.tar")
	if _, errs, code := exec(t, d, "", "-w", "-u", "-f", arc, "file"); code != 0 {
		t.Fatalf("seekable update: %d %s", code, errs)
	}
	if names := listNames(t, mustRead(t, arc)); len(names) != 1 || names[0] != "file" {
		t.Fatalf("archive members = %v", names)
	}
}

func TestWriteUpdateOnCPIORewritesArchive(t *testing.T) {
	d := t.TempDir()
	writeFileAt(t, d, "file", "x")
	arc := filepath.Join(d, "archive.cpio")
	if _, errs, code := exec(t, d, "", "-w", "-x", "cpio", "-f", arc, "file"); code != 0 {
		t.Fatalf("create cpio: %d %s", code, errs)
	}
	if err := os.WriteFile(filepath.Join(d, "file"), []byte("updated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(d, "file"), time.Now().Add(time.Hour), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, errs, code := exec(t, d, "", "-w", "-u", "-x", "cpio", "-f", arc, "file"); code != 0 {
		t.Fatalf("cpio update: code=%d stderr=%q", code, errs)
	}
	dest := t.TempDir()
	if _, errs, code := exec(t, dest, "", "-r", "-f", arc); code != 0 {
		t.Fatalf("extract updated cpio: code=%d stderr=%q", code, errs)
	}
	if got := string(mustRead(t, filepath.Join(dest, "file"))); got != "updated" {
		t.Fatalf("updated cpio data=%q", got)
	}
}

// ---------------------------------------------------------------------------
// append lane failure paths
// ---------------------------------------------------------------------------

type failingSink struct {
	archiveSink
	truncateErr error
	closeErr    error
}

func (s *failingSink) Truncate(size int64) error {
	if s.truncateErr != nil {
		return s.truncateErr
	}
	return s.archiveSink.Truncate(size)
}

func (s *failingSink) Close() error {
	err := s.archiveSink.Close()
	if s.closeErr != nil {
		return s.closeErr
	}
	return err
}

func TestAppendReportsTruncateAndCloseFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		wrap func(archiveSink) archiveSink
		want string
	}{
		{"truncate", func(s archiveSink) archiveSink {
			return &failingSink{archiveSink: s, truncateErr: errors.New("injected truncate failure")}
		}, "injected truncate failure"},
		{"close", func(s archiveSink) archiveSink {
			return &failingSink{archiveSink: s, closeErr: errors.New("injected close failure")}
		}, "injected close failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			arc := filepath.Join(d, "archive.tar")
			writeFileAt(t, d, "first", "one")
			writeFileAt(t, d, "second", "two")
			if _, errs, code := exec(t, d, "", "-w", "-f", arc, "first"); code != 0 {
				t.Fatalf("create: %d %s", code, errs)
			}
			original := openArchiveSink
			openArchiveSink = func(path string, flags int, perm os.FileMode) (archiveSink, error) {
				f, err := original(path, flags, perm)
				if err != nil {
					return nil, err
				}
				return tc.wrap(f), nil
			}
			defer func() { openArchiveSink = original }()

			_, errs, code := exec(t, d, "", "-w", "-a", "-f", arc, "second")
			if code == 0 || !strings.Contains(errs, tc.want) {
				t.Fatalf("append: code=%d stderr=%q, want %q", code, errs, tc.want)
			}
		})
	}
}

type appendFaultSink struct {
	archiveSink
	seekErr       error
	readErr       error
	writeErr      error
	shortWrite    bool
	closeErr      error
	seekCalls     int
	truncateCalls int
}

func (s *appendFaultSink) Seek(offset int64, whence int) (int64, error) {
	s.seekCalls++
	if s.seekErr != nil {
		return 0, s.seekErr
	}
	return s.archiveSink.Seek(offset, whence)
}

func (s *appendFaultSink) Read(p []byte) (int, error) {
	if s.readErr != nil {
		return 0, s.readErr
	}
	return s.archiveSink.Read(p)
}

func (s *appendFaultSink) Write(p []byte) (int, error) {
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	if s.shortWrite {
		return len(p) - 1, nil
	}
	return s.archiveSink.Write(p)
}

func (s *appendFaultSink) Truncate(size int64) error {
	s.truncateCalls++
	return s.archiveSink.Truncate(size)
}

func (s *appendFaultSink) Close() error {
	err := s.archiveSink.Close()
	if s.closeErr != nil {
		return s.closeErr
	}
	return err
}

func TestAppendPreparationAndWriteFailuresAreReportedWithoutExtraMutation(t *testing.T) {
	for _, tc := range []struct {
		name string
		wrap func(*appendFaultSink)
		want []string
	}{
		{
			name: "seek-and-close",
			wrap: func(s *appendFaultSink) {
				s.seekErr = errors.New("injected seek failure")
				s.closeErr = errors.New("injected close after seek failure")
			},
			want: []string{"injected seek failure", "injected close after seek failure"},
		},
		{
			name: "prefix-read",
			wrap: func(s *appendFaultSink) {
				s.readErr = errors.New("injected prefix read failure")
			},
			want: []string{"injected prefix read failure"},
		},
		{
			name: "write",
			wrap: func(s *appendFaultSink) {
				s.writeErr = errors.New("injected physical write failure")
			},
			want: []string{"injected physical write failure"},
		},
		{
			name: "short-write",
			wrap: func(s *appendFaultSink) {
				s.shortWrite = true
			},
			want: []string{io.ErrShortWrite.Error()},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			arc := filepath.Join(d, "archive.tar")
			writeFileAt(t, d, "empty", "")
			writeFileAt(t, d, "second", "payload")
			if _, errs, code := exec(t, d, "", "-w", "-x", "ustar", "-b", "1024", "-f", arc, "empty"); code != 0 {
				t.Fatalf("create: %d %s", code, errs)
			}
			before := mustRead(t, arc)
			end, _, err := scanTar(before)
			if err != nil || end%1024 == 0 {
				t.Fatalf("test archive must have a nonaligned end: end=%d err=%v", end, err)
			}

			original := openArchiveSink
			var fault *appendFaultSink
			openArchiveSink = func(path string, flags int, perm os.FileMode) (archiveSink, error) {
				sink, err := original(path, flags, perm)
				if err != nil {
					return nil, err
				}
				fault = &appendFaultSink{archiveSink: sink}
				tc.wrap(fault)
				return fault, nil
			}
			defer func() { openArchiveSink = original }()

			_, errs, code := exec(t, d, "", "-w", "-a", "-x", "ustar", "-b", "1024", "-f", arc, "second")
			if code == 0 {
				t.Fatalf("append unexpectedly succeeded: %q", errs)
			}
			for _, want := range tc.want {
				if !strings.Contains(errs, want) {
					t.Fatalf("stderr=%q, want %q", errs, want)
				}
			}
			if fault.truncateCalls != 0 {
				t.Fatalf("failed append called Truncate %d times", fault.truncateCalls)
			}
			if after := mustRead(t, arc); !bytes.Equal(before, after) {
				t.Fatal("failure before a successful physical write mutated the archive")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// identity and hardlinks
// ---------------------------------------------------------------------------

func linkedTree(t *testing.T) (dir string, id fileIdentity) {
	t.Helper()
	dir = t.TempDir()
	writeFileAt(t, dir, "src/one", "shared payload")
	if err := os.Link(filepath.Join(dir, "src", "one"), filepath.Join(dir, "src", "two")); err != nil {
		t.Skipf("hardlinks unsupported here: %v", err)
	}
	fi, err := os.Lstat(filepath.Join(dir, "src", "one"))
	if err != nil {
		t.Fatal(err)
	}
	id = identityOf(fi)
	if !id.ok {
		t.Skip("no source file identity on this platform")
	}
	return dir, id
}

// A pax archive carries the source owner in the ustar fields, and a second
// name for an already archived inode becomes a hardlink member rather than a
// second data copy. It does not invent private identity extended records.
func TestPaxWritePreservesSourceIdentityAndHardlinks(t *testing.T) {
	d, id := linkedTree(t)
	arc := filepath.Join(d, "archive.tar")
	if _, errs, code := exec(t, d, "", "-w", "-f", arc, "src"); code != 0 {
		t.Fatalf("write: %d %s", code, errs)
	}
	tr := tar.NewReader(bytes.NewReader(mustRead(t, arc)))
	links := 0
	regulars := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if int(id.uid) != h.Uid || int(id.gid) != h.Gid {
			t.Errorf("%s: uid/gid = %d/%d, want %d/%d", h.Name, h.Uid, h.Gid, id.uid, id.gid)
		}
		if h.PAXRecords["SCHILY.dev"] != "" || h.PAXRecords["SCHILY.ino"] != "" || h.PAXRecords["SCHILY.nlink"] != "" {
			t.Errorf("%s: non-POSIX identity records = %v", h.Name, h.PAXRecords)
		}
		switch h.Typeflag {
		case tar.TypeLink:
			links++
			if h.Size != 0 {
				t.Errorf("%s: hardlink member carries %d bytes of data", h.Name, h.Size)
			}
		case tar.TypeReg:
			regulars++
		}
	}
	if regulars != 1 || links != 1 {
		t.Fatalf("archive has %d regular and %d hardlink members, want 1 and 1", regulars, links)
	}

	dest := t.TempDir()
	if _, errs, code := exec(t, dest, "", "-r", "-f", arc); code != 0 {
		t.Fatalf("extract: %d %s", code, errs)
	}
	assertSameFile(t, filepath.Join(dest, "src", "one"), filepath.Join(dest, "src", "two"), "shared payload")
}

// The cpio header has native fields for all of it. Inode numbers routinely
// overflow six octal digits, so what must survive is the OWNER, the LINK COUNT
// and the hardlink IDENTITY - equal c_dev/c_ino for equal source inodes.
func TestCPIOWritePreservesSourceIdentityAndHardlinks(t *testing.T) {
	d, id := linkedTree(t)
	arc := filepath.Join(d, "archive.cpio")
	if _, errs, code := exec(t, d, "", "-w", "-x", "cpio", "-f", arc, "src"); code != 0 {
		t.Fatalf("write: %d %s", code, errs)
	}
	entries, err := readCPIOEntries(mustRead(t, arc))
	if err != nil {
		t.Fatal(err)
	}
	var files []cpioEntry
	for _, e := range entries {
		if e.mode&0o170000 == 0o100000 {
			files = append(files, e)
		}
	}
	if len(files) != 2 {
		t.Fatalf("cpio has %d regular members, want 2", len(files))
	}
	for _, e := range files {
		if e.uid != id.uid || e.gid != id.gid || e.nlink != id.nlink {
			t.Errorf("%s: uid/gid/nlink = %d/%d/%d, want %d/%d/%d",
				e.name, e.uid, e.gid, e.nlink, id.uid, id.gid, id.nlink)
		}
	}
	if files[0].dev != files[1].dev || files[0].ino != files[1].ino {
		t.Fatalf("hardlinked members got different identities: %v vs %v",
			devIno{files[0].dev, files[0].ino}, devIno{files[1].dev, files[1].ino})
	}
	// POSIX carries the data on the last member of the group.
	if len(files[0].data) != 0 || string(files[1].data) != "shared payload" {
		t.Fatalf("data placement = %q / %q", files[0].data, files[1].data)
	}

	dest := t.TempDir()
	if _, errs, code := exec(t, dest, "", "-r", "-f", arc); code != 0 {
		t.Fatalf("extract: %d %s", code, errs)
	}
	assertSameFile(t, filepath.Join(dest, "src", "one"), filepath.Join(dest, "src", "two"), "shared payload")
}

// Distinct source inodes must never collapse onto one c_dev/c_ino, and equal
// ones must never split, whether the values are written verbatim or remapped.
func TestODCIdentitiesAreOneToOne(t *testing.T) {
	fits := []cpioMember{
		{id: fileIdentity{dev: 1, ino: 2, ok: true}},
		{id: fileIdentity{dev: 1, ino: 2, ok: true}},
		{id: fileIdentity{dev: 1, ino: 3, ok: true}},
	}
	got := odcIdentities(fits)
	if got[0] != (devIno{1, 2}) || got[1] != (devIno{1, 2}) || got[2] != (devIno{1, 3}) {
		t.Fatalf("in-range identities were not written verbatim: %v", got)
	}

	overflow := []cpioMember{
		{id: fileIdentity{dev: 1, ino: 0o7777777, ok: true}},
		{id: fileIdentity{dev: 1, ino: 0o7777777, ok: true}},
		{id: fileIdentity{dev: 1, ino: 0o7777776, ok: true}},
		{id: fileIdentity{}},
		{id: fileIdentity{}},
	}
	got = odcIdentities(overflow)
	if got[0] != got[1] {
		t.Fatalf("equal source inodes were split: %v", got)
	}
	seen := map[devIno]int{}
	for i, v := range got {
		if v.ino > 0o777777 || v.dev > 0o777777 {
			t.Fatalf("remapped identity %v does not fit a POSIX cpio header", v)
		}
		if prev, ok := seen[v]; ok && !(prev == 0 && i == 1) {
			t.Fatalf("identity %v reused by members %d and %d", v, prev, i)
		}
		seen[v] = i
	}
}

func assertSameFile(t *testing.T, a, b, body string) {
	t.Helper()
	fa, err := os.Lstat(a)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := os.Lstat(b)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(fa, fb) {
		t.Fatalf("%s and %s are not the same file after extraction", a, b)
	}
	if got := mustRead(t, a); string(got) != body {
		t.Fatalf("%s = %q, want %q", a, got, body)
	}
}

// ---------------------------------------------------------------------------
// newc reader
// ---------------------------------------------------------------------------

// cpio is free to put a hardlink group's data on any member - POSIX and GNU
// cpio use the LAST. A reader that assumes the first would materialize the
// remaining names as empty files, so every placement must produce the same
// tree: one real file, the rest linked to it, all with the content.
func TestNewcHardlinkDataOnAnyMemberMaterializesEveryName(t *testing.T) {
	const body = "linked content"
	names := []string{"a", "b", "c"}
	for holder := 0; holder < len(names); holder++ {
		t.Run(fmt.Sprintf("data-on-%s", names[holder]), func(t *testing.T) {
			specs := make([]cpioSpec, len(names))
			for i, name := range names {
				specs[i] = cpioSpec{name: name, nlink: uint64(len(names)), dev: 7, ino: 99}
				if i == holder {
					specs[i].data = []byte(body)
				}
			}
			arc := buildNewc(t, false, specs)

			dest := t.TempDir()
			path := filepath.Join(dest, "archive.cpio")
			if err := os.WriteFile(path, arc, 0o644); err != nil {
				t.Fatal(err)
			}
			out, errs, code := exec(t, dest, "", "-f", path)
			if code != 0 || strings.Join(strings.Fields(out), ",") != "a,b,c" {
				t.Fatalf("list: code=%d out=%q err=%q", code, out, errs)
			}
			if _, errs, code := exec(t, dest, "", "-r", "-f", path); code != 0 {
				t.Fatalf("extract: %d %s", code, errs)
			}
			for _, name := range names {
				if got := mustRead(t, filepath.Join(dest, name)); string(got) != body {
					t.Fatalf("%s = %q, want %q", name, got, body)
				}
			}
			assertSameFile(t, filepath.Join(dest, "a"), filepath.Join(dest, "c"), body)
		})
	}
}

// TestNewcCRCIsValidated pins the 070702 contract: a matching checksum reads
// normally, a mismatched one refuses the whole archive during decode - before
// the extraction planner has looked at a single destination, so nothing is
// written even though the corrupted member is not the first.
func TestNewcCRCIsValidated(t *testing.T) {
	good := buildNewc(t, true, []cpioSpec{
		{name: "first", data: []byte("one")},
		{name: "second", data: []byte("two")},
	})
	if _, err := decodeArchive(good); err != nil {
		t.Fatalf("valid CRC archive rejected: %v", err)
	}

	bad := buildNewc(t, true, []cpioSpec{
		{name: "first", data: []byte("one")},
		{name: "second", data: []byte("two"), badCRC: true},
	})
	_, err := decodeArchive(bad)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("corrupted CRC archive: err=%v", err)
	}

	dest := t.TempDir()
	path := filepath.Join(dest, "archive.cpio")
	if err := os.WriteFile(path, bad, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errs, code := exec(t, dest, "", "-r", "-f", path)
	if code == 0 || !strings.Contains(errs, "checksum mismatch") {
		t.Fatalf("extract: code=%d stderr=%q", code, errs)
	}
	if _, err := os.Lstat(filepath.Join(dest, "first")); err == nil {
		t.Fatal("a checksum failure extracted the members preceding it")
	}
	// 070701 has no checksum field, so the same bytes without the CRC magic
	// must still read.
	plain := buildNewc(t, false, []cpioSpec{{name: "first", data: []byte("one")}})
	if _, err := decodeArchive(plain); err != nil {
		t.Fatalf("plain newc rejected: %v", err)
	}
}

// ---------------------------------------------------------------------------
// malformed cpio headers
// ---------------------------------------------------------------------------

func TestMalformedCPIOHeadersAreRejected(t *testing.T) {
	odc := buildODC(t, []cpioSpec{{name: "file", data: []byte("body")}})
	newc := buildNewc(t, false, []cpioSpec{{name: "file", data: []byte("body")}})

	corrupt := func(data []byte, at int, with string) []byte {
		out := append([]byte(nil), data...)
		copy(out[at:], with)
		return out
	}

	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"odc-non-octal-field", corrupt(odc, 6, "9abcde"), "invalid octal header"},
		{"odc-non-octal-mtime", corrupt(odc, 48, "zzzzzzzzzzz"), "invalid octal header"},
		{"odc-truncated-header", odc[:40], "unexpected EOF"},
		{"odc-zero-namesize", corrupt(odc, 59, "000000"), "invalid name size"},
		{"odc-oversized-namesize", corrupt(odc, 59, "777777"), "invalid name size"},
		{"odc-unterminated-name", corrupt(odc, 76, "fileX"), "not NUL-terminated"},
		{"odc-filesize-past-end", corrupt(odc, 65, "77777777777"), "unexpected EOF"},
		{"odc-missing-trailer", odc[:76+5+4], "missing TRAILER!!!"},
		{"newc-non-hex-field", corrupt(newc, 6, "ZZZZZZZZ"), "invalid hexadecimal header"},
		{"newc-non-hex-namesize", corrupt(newc, 6+11*8, "-0000005"), "invalid hexadecimal header"},
		{"newc-truncated-header", newc[:60], "unexpected EOF"},
		{"newc-zero-namesize", corrupt(newc, 6+11*8, "00000000"), "invalid name size"},
		{"newc-filesize-past-end", corrupt(newc, 6+6*8, "7FFFFFFF"), "unexpected EOF"},
		{"newc-missing-trailer", newc[:120], "missing TRAILER!!!"},
		// A later member with an unrecognized magic: the archive opens as cpio
		// and must not be silently truncated at the bad header.
		{"unknown-magic", corrupt(odc, len(odc)-87, "070777"), "invalid magic"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeArchive(tc.data)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("decodeArchive = %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// cpio escape attempts
// ---------------------------------------------------------------------------

// A cpio archive reaches extraction through the same planner as tar, so a
// symlink or hardlink that leaves the destination root is diagnosed and skipped
// without preventing unrelated safe members from being extracted.
func TestCPIOEscapeAttemptsAreDiagnosedAndSkipped(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		specs []cpioSpec
		want  string
	}{
		{
			"symlink-escape",
			[]cpioSpec{
				{name: "innocent", data: []byte("ok")},
				{name: "escape", mode: 0o120777, data: []byte("../../elsewhere")},
			},
			"escapes root",
		},
		{
			"absolute-symlink",
			[]cpioSpec{
				{name: "innocent", data: []byte("ok")},
				{name: "escape", mode: 0o120777, data: []byte(secret)},
			},
			"is absolute",
		},
		{
			"hardlink-escape",
			[]cpioSpec{
				{name: "../outside", nlink: 2, dev: 3, ino: 4, data: []byte("payload")},
				{name: "innocent", data: []byte("ok")},
				{name: "inside", nlink: 2, dev: 3, ino: 4},
			},
			"escapes or names root",
		},
		{
			"absolute-member",
			[]cpioSpec{
				{name: "innocent", data: []byte("ok")},
				{name: "/etc/passwd", data: []byte("root")},
			},
			"absolute member path",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, form := range []struct {
				label string
				data  []byte
			}{
				{"odc", buildODC(t, tc.specs)},
				{"newc", buildNewc(t, false, tc.specs)},
			} {
				t.Run(form.label, func(t *testing.T) {
					dest := t.TempDir()
					path := filepath.Join(dest, "archive.cpio")
					if err := os.WriteFile(path, form.data, 0o644); err != nil {
						t.Fatal(err)
					}
					_, errs, code := exec(t, dest, "", "-r", "-f", path)
					if code == 0 || !strings.Contains(errs, tc.want) {
						t.Fatalf("extract: code=%d stderr=%q, want %q", code, errs, tc.want)
					}
					if got, err := os.ReadFile(filepath.Join(dest, "innocent")); err != nil || string(got) != "ok" {
						t.Fatalf("safe cpio member = (%q, %v), want extracted", got, err)
					}
				})
			}
		})
	}
}

// ---------------------------------------------------------------------------
// standard-input pathname list
// ---------------------------------------------------------------------------

func TestStdinPathnameListDiagnosesEmptyAndUnterminatedNames(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		want  string
		names []string
	}{
		{"empty-line", "a\n\nb\n", "empty pathname", []string{"a", "b"}},
		{"leading-empty-line", "\na\n", "empty pathname", []string{"a"}},
		{"only-empty-line", "\n", "empty pathname", nil},
		{"unterminated", "a\nb", "unterminated pathname", []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			for _, name := range []string{"a", "b"} {
				writeFileAt(t, d, name, name)
			}
			arc := filepath.Join(d, "archive.tar")
			_, errs, code := exec(t, d, tc.stdin, "-w", "-f", arc)
			if code != 1 || !strings.Contains(errs, tc.want) {
				t.Fatalf("write: code=%d stderr=%q, want %q", code, errs, tc.want)
			}
			got := listNames(t, mustRead(t, arc))
			if strings.Join(got, ",") != strings.Join(tc.names, ",") {
				t.Fatalf("archived %v, want %v", got, tc.names)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RunContext relative paths
// ---------------------------------------------------------------------------

// Every operand is resolved against the CALLER's directory, never the
// process's. An embedded shell moves its own cwd freely, so a tool that
// consulted os.Getwd would archive and extract in the wrong tree.
func TestRelativeSourceAndArchivePathsResolveAgainstRunContext(t *testing.T) {
	processDir := t.TempDir()
	restore, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(processDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(restore) })

	caller := t.TempDir()
	writeFileAt(t, caller, "tree/leaf", "payload")

	// Write: both the source operand and -f are relative to rc.Dir.
	if _, errs, code := exec(t, caller, "", "-w", "-f", "archive.tar", "tree"); code != 0 {
		t.Fatalf("write: %d %s", code, errs)
	}
	arc := filepath.Join(caller, "archive.tar")
	if _, err := os.Lstat(arc); err != nil {
		t.Fatalf("archive was not created in the caller's directory: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(processDir, "archive.tar")); err == nil {
		t.Fatal("archive was created in the process's directory")
	}
	if names := listNames(t, mustRead(t, arc)); strings.Join(names, ",") != "tree/,tree/leaf" {
		t.Fatalf("archive members = %v", names)
	}

	// List: -f is relative to rc.Dir too.
	if out, errs, code := exec(t, caller, "", "-f", "archive.tar"); code != 0 || !strings.Contains(out, "tree/leaf") {
		t.Fatalf("list: code=%d out=%q err=%q", code, out, errs)
	}

	// Read: the archive is relative to rc.Dir and extraction lands there.
	dest := t.TempDir()
	if err := os.Rename(arc, filepath.Join(dest, "archive.tar")); err != nil {
		t.Fatal(err)
	}
	if _, errs, code := exec(t, dest, "", "-r", "-f", "archive.tar"); code != 0 {
		t.Fatalf("read: %d %s", code, errs)
	}
	if got := mustRead(t, filepath.Join(dest, "tree", "leaf")); string(got) != "payload" {
		t.Fatalf("extracted %q", got)
	}
	if _, err := os.Lstat(filepath.Join(processDir, "tree")); err == nil {
		t.Fatal("extraction escaped into the process's directory")
	}

	// Copy mode: source and destination operands are both relative.
	copyDest := filepath.Join(dest, "copy")
	if err := os.Mkdir(copyDest, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, errs, code := exec(t, dest, "", "-r", "-w", "tree", "copy"); code != 0 {
		t.Fatalf("copy: %d %s", code, errs)
	}
	if got := mustRead(t, filepath.Join(copyDest, "tree", "leaf")); string(got) != "payload" {
		t.Fatalf("copied %q", got)
	}
}

// A RunContext whose Out is not a file still gets whole physical blocks, and
// the tool never touches os.Stdin/os.Stdout to get them.
func TestWriteToRunContextStdoutIsBlocked(t *testing.T) {
	d := t.TempDir()
	writeFileAt(t, d, "file", "payload")
	var out bytes.Buffer
	var errOut bytes.Buffer
	rc := &tool.RunContext{Dir: d, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
	if code := run(rc, []string{"-w", "file"}); code != 0 {
		t.Fatalf("write: code=%d stderr=%q", code, errOut.String())
	}
	if out.Len() != 10240 {
		t.Fatalf("stdout archive is %d bytes, want one 10240-byte block", out.Len())
	}
}
