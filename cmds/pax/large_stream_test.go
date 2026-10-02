package paxcmd

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func TestStreamedPAXHeaderPassMatchesMemoryPass(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "headers-*.pax")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	tw := tar.NewWriter(file)
	if err := tw.WriteHeader(&tar.Header{
		Name: "member", Mode: 0o644, Size: 1, Format: tar.FormatPAX,
		PAXRecords: map[string]string{"comment": "force extended header"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	want, err := patchExtendedHeaderNames(before, "")
	if err != nil {
		t.Fatal(err)
	}
	want, err = patchRawMemberNames(want, []string{"member"})
	if err != nil {
		t.Fatal(err)
	}
	want, err = normalizeTarChecksums(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := patchStreamedPAXHeaders(file, []string{"member"}, ""); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("streamed header rewrite differs from existing PAX byte contract")
	}
}

func TestStreamedPAXEightGiBPhysicalSize(t *testing.T) {
	const size = int64(8 << 30)
	file, err := os.CreateTemp(t.TempDir(), "large-headers-*.pax")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	tw := tar.NewWriter(file)
	if err := tw.WriteHeader(&tar.Header{
		Name: "member", Mode: 0o644, Size: size, Format: tar.FormatPAX,
	}); err != nil {
		t.Fatal(err)
	}
	// The header pass only seeks over member data. A sparse file exercises
	// the real 8 GiB offsets without allocating or copying the payload.
	const dataOffset = int64(3 * 512)
	if err := file.Truncate(dataOffset + size + 1024); err != nil {
		t.Fatal(err)
	}
	if err := patchStreamedPAXHeaders(file, []string{"member"}, ""); err != nil {
		t.Fatal(err)
	}
	physical := make([]byte, 512)
	if _, err := file.ReadAt(physical, 2*512); err != nil {
		t.Fatal(err)
	}
	got, err := rawTarSize(physical)
	if err != nil || got != 0o77777777777 {
		t.Fatalf("physical size = (%d, %v), want maximum octal size", got, err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	header, err := tar.NewReader(file).Next()
	if err != nil || header.Size != size {
		t.Fatalf("logical size = (%v, %v), want %d", header, err, size)
	}
}

func TestLargePAXWriteKeepsArchiveOutOfHeap(t *testing.T) {
	const size = 64 << 20
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	f, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte("Z"), size-1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "archive.pax")
	o := &options{
		write: true, archive: archive, format: "pax", blockBytes: 10240,
		now: time.Now, paxOptions: paxOptions{invalid: "bypass"},
	}
	var diagnostics bytes.Buffer
	rc := &tool.RunContext{Dir: dir, Stdio: tool.Stdio{
		In: strings.NewReader(""), Out: io.Discard, Err: &diagnostics,
	}}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if code := writeModeWithStreamThreshold(rc, o, []string{"source"}, 1); code != 0 {
		t.Fatalf("streamed pax exited %d: %s", code, diagnostics.String())
	}
	runtime.ReadMemStats(&after)
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 32<<20 {
		t.Fatalf("streamed %d-byte member allocated %d bytes", size, allocated)
	}
	ar, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer ar.Close()
	tr := tar.NewReader(ar)
	h, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if h.Name != "source" || h.Size != size {
		t.Fatalf("archive header = (%q, %d)", h.Name, h.Size)
	}
	if n, err := io.Copy(io.Discard, tr); err != nil || n != size {
		t.Fatalf("archive payload = (%d, %v)", n, err)
	}
}

func TestPAXWriterEmitsEightGiBSizeRecord(t *testing.T) {
	var header bytes.Buffer
	tw := tar.NewWriter(&header)
	if err := tw.WriteHeader(&tar.Header{
		Name: "large", Mode: 0o644, Size: 8 << 30, Format: tar.FormatPAX,
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(header.Bytes(), []byte("size=8589934592\n")) {
		t.Fatal("PAX writer omitted the extended size record required by the 8 GiB fixture")
	}
}
