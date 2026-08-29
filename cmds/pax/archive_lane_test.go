package paxcmd

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func TestBlockSizeGrammar(t *testing.T) {
	// The POSIX pax size grammar: decimal factors joined by 'x', each
	// optionally suffixed b (512), k (1024) or m (1048576). The product must be
	// positive, a multiple of 512, and no more than 32256.
	valid := map[string]int{
		"512":     512,
		"1024":    1024,
		"10k":     10240,
		"1b":      512,
		"20b":     10240,
		"10x512":  5120,
		"5x1024":  5120,
		"2x5x512": 5120,
		"512x1":   512,
		"32256":   32256,
		"63b":     32256,
	}
	for value, want := range valid {
		got, err := parseBlockSize(value)
		if err != nil {
			t.Errorf("parseBlockSize(%q): %v", value, err)
			continue
		}
		if got != want {
			t.Errorf("parseBlockSize(%q) = %d, want %d", value, got, want)
		}
	}
	invalid := []string{
		"",                                  // no value
		"0",                                 // not positive
		"0x512",                             // product is zero
		"1",                                 // not a multiple of 512
		"511",                               // not a multiple of 512
		"513",                               // not a multiple of 512
		"+512",                              // sign is not part of the grammar
		"-512",                              // sign is not part of the grammar
		" 512",                              // no surrounding blanks
		"512 ",                              // no surrounding blanks
		"0x200",                             // hexadecimal is not the grammar; 'x' joins factors
		"512x",                              // empty trailing factor
		"x512",                              // empty leading factor
		"512xx1",                            // empty inner factor
		"k",                                 // multiplier with no digits
		"10K",                               // multipliers are lowercase
		"1g",                                // no gigabyte multiplier
		"１２",                                // non-ASCII digits
		"32768",                             // above the maximum
		"1m",                                // above the maximum
		"64b",                               // above the maximum
		"999999999999999999999",             // overflows a factor
		"99999999999x99999999999x999999999", // overflows the product
	}
	for _, value := range invalid {
		if got, err := parseBlockSize(value); err == nil {
			t.Errorf("parseBlockSize(%q) = %d, want an error", value, got)
		}
	}
}

func TestBlockSizeMultiplicationIsChecked(t *testing.T) {
	if _, err := mulChecked(1<<63, 4); err == nil {
		t.Fatal("mulChecked did not report an overflow")
	}
	if got, err := mulChecked(0, 1<<63); err != nil || got != 0 {
		t.Fatalf("mulChecked(0, ...) = %d, %v", got, err)
	}
	if got, err := mulChecked(3, 7); err != nil || got != 21 {
		t.Fatalf("mulChecked(3, 7) = %d, %v", got, err)
	}
}

type recordingWriter struct {
	bytes.Buffer
	writes []int
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.writes = append(w.writes, len(p))
	return w.Buffer.Write(p)
}

// TestPhysicalBlockingDefaultsAndExplicitSizes pins the blocking contract:
// pax/ustar default to 10240-byte blocks and cpio to 5120, an explicit -b
// replaces the default, and every archive ends on a full zero-padded block
// regardless of how much payload it carried.
func TestPhysicalBlockingDefaultsAndExplicitSizes(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		block int
	}{
		{"pax-default", []string{"-w"}, 10240},
		{"ustar-default", []string{"-w", "-x", "ustar"}, 10240},
		{"cpio-default", []string{"-w", "-x", "cpio"}, 5120},
		{"pax-explicit", []string{"-w", "-b", "10240"}, 10240},
		{"cpio-explicit-suffix", []string{"-w", "-x", "cpio", "-b", "5k"}, 5120},
		{"explicit-factors", []string{"-w", "-b", "2x512"}, 1024},
		{"explicit-below-default", []string{"-w", "-x", "ustar", "-b", "512"}, 512},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			if err := os.WriteFile(filepath.Join(d, "file"), bytes.Repeat([]byte("x"), 777), 0o644); err != nil {
				t.Fatal(err)
			}
			var out recordingWriter
			var errOut bytes.Buffer
			rc := &tool.RunContext{Dir: d, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
			if code := run(rc, append(append([]string{}, tc.args...), "file")); code != 0 {
				t.Fatalf("write: code=%d stderr=%q", code, errOut.String())
			}
			if len(out.writes) == 0 {
				t.Fatal("archive made no physical writes")
			}
			for i, n := range out.writes {
				if n != tc.block {
					t.Errorf("physical write %d had %d bytes, want %d", i, n, tc.block)
				}
			}
			if out.Len()%tc.block != 0 {
				t.Fatalf("archive length %d is not a whole number of %d-byte blocks", out.Len(), tc.block)
			}
			if tail := out.Bytes()[out.Len()-tc.block:]; !allZero(tail[len(tail)-16:]) {
				t.Fatal("final block is not zero-padded")
			}
			archive, err := decodeArchive(out.Bytes())
			if err != nil {
				t.Fatalf("blocked output does not decode: %v", err)
			}
			h, err := tar.NewReader(bytes.NewReader(archive.tarData)).Next()
			if err != nil || h.Name != "file" || h.Size != 777 {
				t.Fatalf("blocked output is not a readable archive: header=%v err=%v", h, err)
			}
		})
	}
}

// TestAppendKeepsArchiveBlockAlignment covers the append lane, where the block
// boundary belongs to the ARCHIVE and not to this invocation: the end markers
// sit at a 512-byte offset that is rarely a block boundary, so a writer that
// restarted its block accounting at zero would leave a misaligned file.
func TestAppendKeepsArchiveBlockAlignment(t *testing.T) {
	d := t.TempDir()
	arc := filepath.Join(d, "archive.tar")
	for _, name := range []string{"first", "second"} {
		if err := os.WriteFile(filepath.Join(d, name), bytes.Repeat([]byte(name), 900), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, errs, code := exec(t, d, "", "-w", "-b", "1024", "-f", arc, "first"); code != 0 {
		t.Fatalf("create: %d %s", code, errs)
	}
	if _, errs, code := exec(t, d, "", "-w", "-a", "-b", "1024", "-f", arc, "second"); code != 0 {
		t.Fatalf("append: %d %s", code, errs)
	}
	fi, err := os.Stat(arc)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size()%1024 != 0 {
		t.Fatalf("appended archive length %d is not block aligned", fi.Size())
	}
	out, errs, code := exec(t, d, "", "-f", arc)
	if code != 0 || strings.Join(strings.Fields(out), ",") != "first,second" {
		t.Fatalf("appended archive lists %q (code=%d err=%q)", out, code, errs)
	}
}

func TestBlockWriterHonorsNonBlockStartOffset(t *testing.T) {
	var out recordingWriter
	prefix := bytes.Repeat([]byte("p"), 512)
	w := newBlockWriter(&out, 1024, prefix)
	if _, err := w.Write(bytes.Repeat([]byte("a"), 1024)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	// Starting 512 bytes into a 1024-byte block, the preserved prefix and new
	// bytes are rewritten together. Every physical write is exactly -b bytes.
	want := []int{1024, 1024}
	if fmt.Sprint(out.writes) != fmt.Sprint(want) {
		t.Fatalf("writes = %v, want %v", out.writes, want)
	}
	if !bytes.Equal(out.Bytes()[:len(prefix)], prefix) {
		t.Fatal("first physical block did not preserve its existing prefix")
	}
	if out.Len()%1024 != 0 {
		t.Fatalf("archive output %d is not block aligned", out.Len())
	}
}

func TestBlockWriterAlignedStartUsesExactPhysicalWrites(t *testing.T) {
	var out recordingWriter
	w := newBlockWriter(&out, 1024, nil)
	if _, err := w.Write(bytes.Repeat([]byte("a"), 1024)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if want := []int{1024}; fmt.Sprint(out.writes) != fmt.Sprint(want) {
		t.Fatalf("writes = %v, want %v", out.writes, want)
	}
}

type writeRecordingSink struct {
	archiveSink
	writes []int
}

func (s *writeRecordingSink) Write(p []byte) (int, error) {
	s.writes = append(s.writes, len(p))
	return s.archiveSink.Write(p)
}

func TestAppendAndUpdateUseOnlyCompletePhysicalWrites(t *testing.T) {
	for _, format := range []string{"pax", "ustar"} {
		for _, update := range []bool{false, true} {
			for _, aligned := range []bool{false, true} {
				name := fmt.Sprintf("%s-update=%t-aligned=%t", format, update, aligned)
				t.Run(name, func(t *testing.T) {
					d := t.TempDir()
					arc := filepath.Join(d, "archive.tar")
					first := filepath.Join(d, "first")
					base := time.Unix(1_700_000_000, 0)
					formatArgs := []string{"-x", format, "-b", "1024"}

					// Crossing a 512-byte data-record boundary toggles the end-marker
					// offset modulo 1024. Find the requested case without depending on
					// whether pax extended headers were needed on this platform.
					found := false
					for _, size := range []int{0, 1, 513, 1025, 1537} {
						if err := os.WriteFile(first, bytes.Repeat([]byte("x"), size), 0o644); err != nil {
							t.Fatal(err)
						}
						if err := os.Chtimes(first, base, base); err != nil {
							t.Fatal(err)
						}
						args := append([]string{"-w"}, formatArgs...)
						args = append(args, "-f", arc, "first")
						if _, errs, code := exec(t, d, "", args...); code != 0 {
							t.Fatalf("create: %d %s", code, errs)
						}
						end, _, err := scanTar(mustRead(t, arc))
						if err != nil {
							t.Fatal(err)
						}
						if (end%1024 == 0) == aligned {
							found = true
							break
						}
					}
					if !found {
						t.Fatal("could not construct requested append alignment")
					}

					operand := "second"
					mode := "-a"
					if update {
						operand = "first"
						mode = "-u"
						if err := os.WriteFile(first, []byte("newer"), 0o644); err != nil {
							t.Fatal(err)
						}
						if err := os.Chtimes(first, base.Add(time.Hour), base.Add(time.Hour)); err != nil {
							t.Fatal(err)
						}
					} else if err := os.WriteFile(filepath.Join(d, operand), []byte("appended"), 0o644); err != nil {
						t.Fatal(err)
					}

					original := openArchiveSink
					var recorded *writeRecordingSink
					openArchiveSink = func(path string, flags int, perm os.FileMode) (archiveSink, error) {
						sink, err := original(path, flags, perm)
						if err != nil {
							return nil, err
						}
						recorded = &writeRecordingSink{archiveSink: sink}
						return recorded, nil
					}
					defer func() { openArchiveSink = original }()

					args := append([]string{"-w", mode}, formatArgs...)
					args = append(args, "-f", arc, operand)
					if _, errs, code := exec(t, d, "", args...); code != 0 {
						t.Fatalf("extend: %d %s", code, errs)
					}
					if recorded == nil || len(recorded.writes) == 0 {
						t.Fatal("append made no physical writes")
					}
					for i, n := range recorded.writes {
						if n != 1024 {
							t.Fatalf("physical writes = %v; write %d is short", recorded.writes, i)
						}
					}
					if got := len(mustRead(t, arc)) % 1024; got != 0 {
						t.Fatalf("final archive is misaligned by %d bytes", got)
					}
				})
			}
		}
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestBlockedOutputDiagnosesShortWrite(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var errOut bytes.Buffer
	rc := &tool.RunContext{Dir: d, Stdio: tool.Stdio{In: strings.NewReader(""), Out: shortWriter{}, Err: &errOut}}
	if code := run(rc, []string{"-w", "-b", "512", "file"}); code == 0 {
		t.Fatal("short archive write succeeded")
	}
	if !strings.Contains(errOut.String(), io.ErrShortWrite.Error()) {
		t.Fatalf("stderr=%q, want short-write diagnostic", errOut.String())
	}
}

func TestTarAppendRewritesEndMarkers(t *testing.T) {
	d := t.TempDir()
	arc := filepath.Join(d, "archive.tar")
	for name, body := range map[string]string{"first": "one", "second": "two"} {
		if err := os.WriteFile(filepath.Join(d, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, errs, code := exec(t, d, "", "-w", "-f", arc, "first"); code != 0 {
		t.Fatalf("create: %d %s", code, errs)
	}
	if _, errs, code := exec(t, d, "", "-w", "-a", "-f", arc, "second"); code != 0 {
		t.Fatalf("append: %d %s", code, errs)
	}
	out, errs, code := exec(t, d, "", "-f", arc)
	if code != 0 || strings.Fields(out)[0] != "first" || !strings.Contains(out, "second") {
		t.Fatalf("appended member is not visible: code=%d out=%q err=%q", code, out, errs)
	}
	data, err := os.ReadFile(arc)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(bytes.NewReader(data))
	var names []string
	for {
		h, nextErr := tr.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			t.Fatal(nextErr)
		}
		names = append(names, h.Name)
	}
	if strings.Join(names, ",") != "first,second" {
		t.Fatalf("tar members=%v", names)
	}
}

func TestTarAppendAcceptsPhysicallyIndistinguishableBasicPAX(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	arc := filepath.Join(d, "archive.tar")
	if _, errs, code := exec(t, d, "", "-w", "-x", "ustar", "-f", arc, "file"); code != 0 {
		t.Fatalf("create ustar: %d %s", code, errs)
	}
	// With no extended records, POSIX pax and ustar have the same physical
	// representation, so the default pax format must accept this archive.
	if _, errs, code := exec(t, d, "", "-w", "-a", "-f", arc, "file"); code != 0 || errs != "" {
		t.Fatalf("basic pax append: code=%d stderr=%q", code, errs)
	}
}

func TestCPIOAppendRejectsMismatchedAndUnsupportedFormatsWithoutMutation(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		data []byte
		args []string
		want string
	}{
		{
			name: "odc-default-pax-output",
			data: buildODC(t, []cpioSpec{{name: "old", data: []byte("old")}}),
			args: []string{"-w", "-a"},
			want: "existing cpio",
		},
		{
			name: "newc",
			data: buildNewc(t, false, []cpioSpec{{name: "old", data: []byte("old")}}),
			args: []string{"-w", "-a", "-x", "cpio"},
			want: "newc/crc",
		},
		{
			name: "crc",
			data: buildNewc(t, true, []cpioSpec{{name: "old", data: []byte("old")}}),
			args: []string{"-w", "-u", "-x", "cpio"},
			want: "newc/crc",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			arc := filepath.Join(d, tc.name+".cpio")
			if err := os.WriteFile(arc, tc.data, 0o644); err != nil {
				t.Fatal(err)
			}
			args := append(append([]string{}, tc.args...), "-f", arc, "file")
			if _, errs, code := exec(t, d, "", args...); code == 0 || !strings.Contains(errs, tc.want) {
				t.Fatalf("format rejection: code=%d stderr=%q", code, errs)
			}
			after, err := os.ReadFile(arc)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(tc.data, after) {
				t.Fatal("rejected cpio rewrite changed archive bytes")
			}
		})
	}
}

func TestAppendRequiresSeekableArchiveAndRewritesCPIO(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, errs, code := exec(t, d, "", "-w", "-a", "file"); code == 0 || !strings.Contains(errs, "seekable") {
		t.Fatalf("nonseekable append: code=%d stderr=%q", code, errs)
	}
	arc := filepath.Join(d, "archive.cpio")
	if _, errs, code := exec(t, d, "", "-w", "-x", "cpio", "-f", arc, "file"); code != 0 {
		t.Fatalf("create cpio: %d %s", code, errs)
	}
	if _, errs, code := exec(t, d, "", "-w", "-a", "-x", "cpio", "-f", arc, "file"); code != 0 {
		t.Fatalf("cpio append: code=%d stderr=%q", code, errs)
	}
	if out, errs, code := exec(t, d, "", "-f", arc); code != 0 || strings.Count(out, "file\n") != 2 {
		t.Fatalf("cpio append list: code=%d out=%q err=%q", code, out, errs)
	}
}

func TestCPIOListAndExtractRoundTrip(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file"), []byte("payload"), 0o640); err != nil {
		t.Fatal(err)
	}
	arc := filepath.Join(d, "archive.cpio")
	if _, errs, code := exec(t, d, "", "-w", "-x", "cpio", "-f", arc, "file"); code != 0 {
		t.Fatalf("create: %d %s", code, errs)
	}
	if out, errs, code := exec(t, d, "", "-f", arc); code != 0 || strings.TrimSpace(out) != "file" {
		t.Fatalf("list: code=%d out=%q err=%q", code, out, errs)
	}
	dest := t.TempDir()
	if _, errs, code := exec(t, dest, "", "-r", "-f", arc); code != 0 {
		t.Fatalf("extract: %d %s", code, errs)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "file")); err != nil || string(got) != "payload" {
		t.Fatalf("extracted data=%q err=%v", got, err)
	}
}

func TestUstarListAndExtractRoundTrip(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file"), []byte("ustar payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	arc := filepath.Join(d, "archive.tar")
	if _, errs, code := exec(t, d, "", "-w", "-x", "ustar", "-f", arc, "file"); code != 0 {
		t.Fatalf("create: %d %s", code, errs)
	}
	if out, errs, code := exec(t, d, "", "-f", arc); code != 0 || strings.TrimSpace(out) != "file" {
		t.Fatalf("list: code=%d out=%q err=%q", code, out, errs)
	}
	dest := t.TempDir()
	if _, errs, code := exec(t, dest, "", "-r", "-f", arc); code != 0 {
		t.Fatalf("extract: %d %s", code, errs)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "file")); err != nil || string(got) != "ustar payload" {
		t.Fatalf("extracted data=%q err=%v", got, err)
	}
}

func newcArchive(t *testing.T, name, body string) []byte {
	t.Helper()
	var out bytes.Buffer
	offset := 0
	add := func(entryName, data string, mode uint32) {
		header := fmt.Sprintf("070701%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x",
			1, mode, 0, 0, 1, 1, len(data), 0, 0, 0, 0, len(entryName)+1, 0)
		out.WriteString(header)
		out.WriteString(entryName)
		out.WriteByte(0)
		offset += 110 + len(entryName) + 1
		for offset%4 != 0 {
			out.WriteByte(0)
			offset++
		}
		out.WriteString(data)
		offset += len(data)
		for offset%4 != 0 {
			out.WriteByte(0)
			offset++
		}
	}
	add(name, body, 0o100644)
	add("TRAILER!!!", "", 0o100000)
	return out.Bytes()
}

func TestReadsLegacyNewcAndStillUsesExtractionPlanner(t *testing.T) {
	data := newcArchive(t, "legacy", "value")
	dest := t.TempDir()
	if _, errs, code := exec(t, dest, string(data), "-r"); code != 0 {
		t.Fatalf("extract newc: %d %s", code, errs)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "legacy")); err != nil || string(got) != "value" {
		t.Fatalf("legacy newc data=%q err=%v", got, err)
	}

	hostile := newcArchive(t, "../escape", "bad")
	safeRoot := t.TempDir()
	if _, _, code := exec(t, safeRoot, string(hostile), "-r"); code == 0 {
		t.Fatal("escaping cpio archive succeeded")
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(safeRoot), "escape")); err == nil {
		t.Fatal("cpio member escaped extraction root")
	}
}

func TestWriteWithoutOperandsReadsStdinAndContinuesAfterPathFailure(t *testing.T) {
	d := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(d, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	arc := filepath.Join(d, "archive.tar")
	_, errs, code := exec(t, d, "b\nmissing\na\n", "-w", "-f", arc)
	if code == 0 || !strings.Contains(errs, "missing") {
		t.Fatalf("stdin pathname failure: code=%d stderr=%q", code, errs)
	}
	out, listErr, listCode := exec(t, d, "", "-f", arc)
	if listCode != 0 || strings.Join(strings.Fields(out), ",") != "b,a" {
		t.Fatalf("continued archive order: code=%d out=%q err=%q", listCode, out, listErr)
	}
}

type errorAfterReader struct {
	data []byte
	done bool
}

func (r *errorAfterReader) Read(p []byte) (int, error) {
	if len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	if !r.done {
		r.done = true
		return 0, errors.New("injected read failure")
	}
	return 0, io.EOF
}

func TestStdinPathReadErrorAndArchiveReadErrorAreDiagnosed(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	arc := filepath.Join(d, "archive.tar")
	var errOut bytes.Buffer
	rc := &tool.RunContext{Dir: d, Stdio: tool.Stdio{In: &errorAfterReader{data: []byte("file\n")}, Out: io.Discard, Err: &errOut}}
	if code := run(rc, []string{"-w", "-f", arc}); code == 0 || !strings.Contains(errOut.String(), "injected read failure") {
		t.Fatalf("pathname input error: code=%d stderr=%q", code, errOut.String())
	}
	if out, errs, code := exec(t, d, "", "-f", arc); code != 0 || strings.TrimSpace(out) != "file" {
		t.Fatalf("path before read error was not archived: code=%d out=%q err=%q", code, out, errs)
	}

	errOut.Reset()
	rc = &tool.RunContext{Dir: d, Stdio: tool.Stdio{In: &errorAfterReader{data: []byte("not an archive")}, Out: io.Discard, Err: &errOut}}
	if code := run(rc, nil); code == 0 || !strings.Contains(errOut.String(), "injected read failure") {
		t.Fatalf("archive input error: code=%d stderr=%q", code, errOut.String())
	}
}

func TestWriteUpdateUsesArchiveMemberMtimes(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "file")
	arc := filepath.Join(d, "archive.tar")
	base := time.Unix(1_700_000_000, 0)
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, base, base); err != nil {
		t.Fatal(err)
	}
	if _, errs, code := exec(t, d, "", "-w", "-f", arc, "file"); code != 0 {
		t.Fatalf("create: %d %s", code, errs)
	}
	if err := os.WriteFile(path, []byte("older filesystem data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, base.Add(-time.Hour), base.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, errs, code := exec(t, d, "", "-w", "-u", "-f", arc, "file"); code != 0 {
		t.Fatalf("older update: %d %s", code, errs)
	}
	if got := lastTarBody(t, arc, "file"); got != "old" {
		t.Fatalf("older file superseded archive member: %q", got)
	}
	if err := os.WriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, base.Add(time.Hour), base.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, errs, code := exec(t, d, "", "-w", "-u", "-f", arc, "file"); code != 0 {
		t.Fatalf("newer update: %d %s", code, errs)
	}
	if got := lastTarBody(t, arc, "file"); got != "new" {
		t.Fatalf("newer file did not supersede archive member: %q", got)
	}
}

func lastTarBody(t *testing.T, archive, name string) string {
	t.Helper()
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(bytes.NewReader(data))
	last := ""
	for {
		h, nextErr := tr.Next()
		if nextErr == io.EOF {
			return last
		}
		if nextErr != nil {
			t.Fatal(nextErr)
		}
		body, readErr := io.ReadAll(tr)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if h.Name == name {
			last = string(body)
		}
	}
}
