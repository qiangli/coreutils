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
	for _, value := range []string{"", "0", "+512", "-1", " 512", "512 ", "10k", "0x200", "１２", "32257", "999999999999999999999"} {
		if _, err := parseBlockSize(value); err == nil {
			t.Errorf("parseBlockSize(%q) succeeded", value)
		}
	}
	for _, value := range []string{"1", "511", "512", "513", "32256"} {
		if _, err := parseBlockSize(value); err != nil {
			t.Errorf("parseBlockSize(%q): %v", value, err)
		}
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

func TestBlockSizeControlsPhysicalWritesAndFinalPadding(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file"), bytes.Repeat([]byte("x"), 777), 0o644); err != nil {
		t.Fatal(err)
	}
	var out recordingWriter
	var errOut bytes.Buffer
	rc := &tool.RunContext{Dir: d, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
	if code := run(rc, []string{"-w", "-b", "513", "file"}); code != 0 {
		t.Fatalf("write: code=%d stderr=%q", code, errOut.String())
	}
	if len(out.writes) == 0 {
		t.Fatal("archive made no physical writes")
	}
	for i, n := range out.writes {
		if n != 513 {
			t.Errorf("write %d had %d bytes, want 513", i, n)
		}
	}
	if out.Len()%513 != 0 {
		t.Fatalf("archive length %d is not a full physical block", out.Len())
	}
	h, err := tar.NewReader(bytes.NewReader(out.Bytes())).Next()
	if err != nil || h.Name != "file" {
		t.Fatalf("blocked output is not a readable tar archive: header=%v err=%v", h, err)
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

func TestTarAppendRejectsMismatchedFormat(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	arc := filepath.Join(d, "archive.tar")
	if _, errs, code := exec(t, d, "", "-w", "-x", "ustar", "-f", arc, "file"); code != 0 {
		t.Fatalf("create ustar: %d %s", code, errs)
	}
	before, _ := os.ReadFile(arc)
	if _, errs, code := exec(t, d, "", "-w", "-a", "-x", "pax", "-f", arc, "file"); code == 0 || !strings.Contains(errs, "existing ustar") {
		t.Fatalf("mismatched append: code=%d stderr=%q", code, errs)
	}
	after, _ := os.ReadFile(arc)
	if !bytes.Equal(before, after) {
		t.Fatal("mismatched append mutated the archive")
	}
}

func TestAppendRequiresSeekableArchiveAndCPIOFailsWithoutMutation(t *testing.T) {
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
	before, _ := os.ReadFile(arc)
	if _, errs, code := exec(t, d, "", "-w", "-a", "-x", "cpio", "-f", arc, "file"); code == 0 || !strings.Contains(errs, "not supported") {
		t.Fatalf("cpio append: code=%d stderr=%q", code, errs)
	}
	after, _ := os.ReadFile(arc)
	if !bytes.Equal(before, after) {
		t.Fatal("failed cpio append mutated the archive")
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
