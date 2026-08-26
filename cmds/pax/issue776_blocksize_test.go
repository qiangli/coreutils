//go:build unix

package paxcmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Profile C / Sprint 79: POSIX Issue 7 fixes the default physical block size
// only "for character special archive files", and for the pax format that
// value is 5120 — NOT the 10240 that ustar (and our implementation-defined
// default for regular files and stdout) uses. The writer must lower the
// default to the spec value when the -f sink is a device.
//
// The spec text (The Open Group Base Specifications Issue 7, pax, -x format):
//
//	pax   : "The default blocksize for this format for character special
//	         archive files shall be 5120."
//	ustar : "The default blocksize ... for character special archive files
//	         shall be 10240."
//	cpio  : "The default blocksize ... for character special archive files
//	         shall be 5120."
//
// For every other sink POSIX leaves the default implementation-defined, so
// bashy keeps 10240 (pax/ustar) / 5120 (cpio) there. These tests discriminate
// the two lanes end to end.

// captureSink is an in-memory archiveSink so a write aimed at a device path
// (whose bytes are otherwise discarded) can be measured. Only Write/Close are
// exercised on the non-append create path; the rest satisfy the interface.
type captureSink struct{ buf bytes.Buffer }

func (c *captureSink) Read(p []byte) (int, error)     { return c.buf.Read(p) }
func (c *captureSink) Write(p []byte) (int, error)    { return c.buf.Write(p) }
func (c *captureSink) Seek(int64, int) (int64, error) { return 0, errors.New("seek unsupported") }
func (c *captureSink) Truncate(int64) error           { return nil }
func (c *captureSink) Close() error                   { return nil }

var _ archiveSink = (*captureSink)(nil)
var _ io.ReadWriteSeeker = (*captureSink)(nil)

// charSpecialDevice returns a path that stats as a character-special file, or
// skips the test when the environment exposes none (e.g. a locked-down
// sandbox). /dev/null is the portable choice on the unix targets.
func charSpecialDevice(t *testing.T) string {
	t.Helper()
	const dev = "/dev/null"
	info, err := os.Stat(dev)
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		t.Skipf("no character-special device available (%s: %v)", dev, err)
	}
	return dev
}

// writeCapturingDevice runs a -w to a character-special -f path, capturing the
// physical bytes the block writer emits so the block size is observable.
func writeCapturingDevice(t *testing.T, dir, dev string, args ...string) int {
	t.Helper()
	cap := &captureSink{}
	original := openArchiveSink
	openArchiveSink = func(string, int, os.FileMode) (archiveSink, error) { return cap, nil }
	defer func() { openArchiveSink = original }()
	full := append([]string{"-w", "-f", dev}, args...)
	if _, errs, code := exec(t, dir, "", full...); code != 0 {
		t.Fatalf("write to %s: code=%d stderr=%q", dev, code, errs)
	}
	return cap.buf.Len()
}

// TestCharSpecialDefaultBlockSize is the load-bearing discriminator: the pax
// format written to a device blocks at 5120, while the same format written to
// a regular file keeps the implementation-defined 10240. A single small member
// is 3072 bytes of logical archive, so each lane emits exactly one physical
// block and the two sizes cannot be confused.
func TestCharSpecialDefaultBlockSize(t *testing.T) {
	dev := charSpecialDevice(t)

	for _, tc := range []struct {
		name   string
		format []string // -x flag pair, empty for the default (pax)
		want   int
	}{
		{"pax-char-special", nil, 5120},
		{"ustar-char-special", []string{"-x", "ustar"}, 10240},
		{"cpio-char-special", []string{"-x", "cpio"}, 5120},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			writeFileAt(t, d, "file", "payload")
			got := writeCapturingDevice(t, d, dev, append(tc.format, "file")...)
			if got != tc.want {
				t.Fatalf("device archive is %d bytes, want one %d-byte block", got, tc.want)
			}
		})
	}
}

// TestRegularFileDefaultBlockSize pins the other lane: pax to a regular file is
// unchanged at 10240. Paired with the char-special case above, this is the
// discrimination the sprint asked for — identical format, block size decided by
// the sink type alone.
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
}

// TestCharSpecialExplicitBlockSizeWins confirms the override is a DEFAULT, not
// a clamp: an explicit -b to a device is honored verbatim, so a 512-byte block
// yields the 3072-byte (six-block) logical archive with no padding to 5120.
func TestCharSpecialExplicitBlockSizeWins(t *testing.T) {
	dev := charSpecialDevice(t)
	d := t.TempDir()
	writeFileAt(t, d, "file", "payload")
	got := writeCapturingDevice(t, d, dev, "-b", "512", "file")
	if got != 3072 {
		t.Fatalf("device archive with -b 512 is %d bytes, want the 3072-byte logical size verbatim", got)
	}
}

// TestBlockSizeSelectors covers the two pure selectors directly so the exact
// POSIX values are pinned independent of any sink plumbing.
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
