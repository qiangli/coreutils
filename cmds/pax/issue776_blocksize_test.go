package paxcmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// Profile C / Sprint 79: POSIX Issue 7 fixes the default physical block size
// only "for character special archive files": 5120 for pax and cpio, 10240
// for ustar. The relevant file is the actual selected output, including
// standard output when -f is absent or names "-".

type reportedFileInfo struct{ mode os.FileMode }

func (i reportedFileInfo) Name() string       { return "archive" }
func (i reportedFileInfo) Size() int64        { return 0 }
func (i reportedFileInfo) Mode() os.FileMode  { return i.mode }
func (i reportedFileInfo) ModTime() time.Time { return time.Time{} }
func (i reportedFileInfo) IsDir() bool        { return false }
func (i reportedFileInfo) Sys() any           { return nil }

// captureSink reports the mode of the selected output object while retaining
// every physical write, making blocking observable without writing an archive
// stream to a terminal or discarding it through a device.
type captureSink struct {
	buf     bytes.Buffer
	mode    os.FileMode
	statErr error
	closes  int
}

func (c *captureSink) Read(p []byte) (int, error)     { return c.buf.Read(p) }
func (c *captureSink) Write(p []byte) (int, error)    { return c.buf.Write(p) }
func (c *captureSink) Seek(int64, int) (int64, error) { return 0, errors.New("seek unsupported") }
func (c *captureSink) Truncate(int64) error           { return nil }
func (c *captureSink) Close() error                   { c.closes++; return nil }
func (c *captureSink) Stat() (os.FileInfo, error) {
	if c.statErr != nil {
		return nil, c.statErr
	}
	return reportedFileInfo{mode: c.mode}, nil
}

var _ archiveSink = (*captureSink)(nil)
var _ io.ReadWriteSeeker = (*captureSink)(nil)

func runWithOutput(t *testing.T, dir string, out io.Writer, args ...string) {
	t.Helper()
	var errs bytes.Buffer
	rc := &tool.RunContext{
		Dir: dir,
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: out,
			Err: &errs,
		},
	}
	if code := run(rc, args); code != 0 {
		t.Fatalf("pax %v: code=%d stderr=%q", args, code, errs.String())
	}
}

func writeCapturingNamedSink(t *testing.T, dir string, mode os.FileMode, args ...string) int {
	t.Helper()
	cap := &captureSink{mode: mode}
	original := openArchiveSink
	openArchiveSink = func(string, int, os.FileMode) (archiveSink, error) { return cap, nil }
	defer func() { openArchiveSink = original }()
	full := append([]string{"-w", "-f", filepath.Join(dir, "selected-archive")}, args...)
	runWithOutput(t, dir, io.Discard, full...)
	return cap.buf.Len()
}

func writeCapturingStdout(t *testing.T, dir string, mode os.FileMode, dash bool, args ...string) int {
	t.Helper()
	cap := &captureSink{mode: mode}
	full := []string{"-w"}
	if dash {
		full = append(full, "-f", "-")
	}
	full = append(full, args...)
	runWithOutput(t, dir, cap, full...)
	return cap.buf.Len()
}

// TestCharSpecialDefaultBlockSize is the load-bearing discriminator. The
// named lane deliberately uses a nonexistent pathname whose opened sink
// reports character-special; a pre-open pathname stat therefore cannot pass.
// Both stdout spellings prove that stdout is the archive file, not a separate
// implementation-defined category.
func TestCharSpecialDefaultBlockSize(t *testing.T) {
	for _, sink := range []struct {
		name  string
		write func(*testing.T, string, os.FileMode, ...string) int
	}{
		{"named", writeCapturingNamedSink},
		{"stdout", func(t *testing.T, dir string, mode os.FileMode, args ...string) int {
			return writeCapturingStdout(t, dir, mode, false, args...)
		}},
		{"stdout-dash", func(t *testing.T, dir string, mode os.FileMode, args ...string) int {
			return writeCapturingStdout(t, dir, mode, true, args...)
		}},
	} {
		for _, format := range []struct {
			name string
			args []string
			want int
		}{
			{"pax", nil, 5120},
			{"ustar", []string{"-x", "ustar"}, 10240},
			{"cpio", []string{"-x", "cpio"}, 5120},
		} {
			t.Run(sink.name+"-"+format.name, func(t *testing.T) {
				d := t.TempDir()
				writeFileAt(t, d, "file", "payload")
				got := sink.write(t, d, os.ModeCharDevice, append(format.args, "file")...)
				if got != format.want {
					t.Fatalf("archive is %d bytes, want one %d-byte block", got, format.want)
				}
			})
		}
	}
}

func TestRegularFileDefaultBlockSize(t *testing.T) {
	d := t.TempDir()
	writeFileAt(t, d, "file", "payload")
	arc := filepath.Join(d, "out.pax")
	if _, errs, code := exec(t, d, "", "-w", "-f", arc, "file"); code != 0 {
		t.Fatalf("write: code=%d stderr=%q", code, errs)
	}
	info, err := os.Stat(arc)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 10240 {
		t.Fatalf("regular-file pax archive is %d bytes, want one 10240-byte block", info.Size())
	}

	if got := writeCapturingStdout(t, d, 0, false, "file"); got != 10240 {
		t.Fatalf("regular stdout archive is %d bytes, want one 10240-byte block", got)
	}
}

func TestCharSpecialExplicitBlockSizeWins(t *testing.T) {
	for _, named := range []bool{true, false} {
		name := "stdout"
		if named {
			name = "named"
		}
		t.Run(name, func(t *testing.T) {
			d := t.TempDir()
			writeFileAt(t, d, "file", "payload")
			cap := &captureSink{
				mode:    os.ModeCharDevice,
				statErr: errors.New("explicit -b must bypass sink inspection"),
			}
			args := []string{"-w", "-b", "512", "file"}
			if named {
				original := openArchiveSink
				openArchiveSink = func(string, int, os.FileMode) (archiveSink, error) { return cap, nil }
				defer func() { openArchiveSink = original }()
				args = []string{"-w", "-b", "512", "-f", filepath.Join(d, "selected-archive"), "file"}
			}
			runWithOutput(t, d, cap, args...)
			if got := cap.buf.Len(); got != 3072 {
				t.Fatalf("archive with -b 512 is %d bytes, want 3072", got)
			}
		})
	}
}

func TestArchiveSinkStatFailureIsFailClosed(t *testing.T) {
	for _, named := range []bool{true, false} {
		name := "stdout"
		if named {
			name = "named"
		}
		t.Run(name, func(t *testing.T) {
			d := t.TempDir()
			writeFileAt(t, d, "file", "payload")
			cap := &captureSink{statErr: errors.New("injected stat failure")}
			args := []string{"-w", "file"}
			if named {
				original := openArchiveSink
				openArchiveSink = func(string, int, os.FileMode) (archiveSink, error) { return cap, nil }
				defer func() { openArchiveSink = original }()
				args = []string{"-w", "-f", filepath.Join(d, "selected-archive"), "file"}
			}
			var errs bytes.Buffer
			rc := &tool.RunContext{
				Dir: d,
				Stdio: tool.Stdio{
					In:  strings.NewReader(""),
					Out: cap,
					Err: &errs,
				},
			}
			if code := run(rc, args); code == 0 || !strings.Contains(errs.String(), "injected stat failure") {
				t.Fatalf("code=%d stderr=%q, want stat failure", code, errs.String())
			}
			if cap.buf.Len() != 0 {
				t.Fatalf("wrote %d bytes after stat failure", cap.buf.Len())
			}
			wantCloses := 0
			if named {
				wantCloses = 1
			}
			if cap.closes != wantCloses {
				t.Fatalf("close calls=%d, want %d", cap.closes, wantCloses)
			}
		})
	}
}

func TestBlockSizeSelectors(t *testing.T) {
	for _, tc := range []struct {
		format               string
		charSpecial, regular int
	}{
		{"pax", 5120, 10240},
		{"ustar", 10240, 10240},
		{"cpio", 5120, 5120},
	} {
		if got := charSpecialBlockSize(tc.format); got != tc.charSpecial {
			t.Errorf("charSpecialBlockSize(%q) = %d, want %d", tc.format, got, tc.charSpecial)
		}
		if got := defaultBlockSize(tc.format); got != tc.regular {
			t.Errorf("defaultBlockSize(%q) = %d, want %d", tc.format, got, tc.regular)
		}
	}
}
