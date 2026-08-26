//go:build unix

package paxcmd

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCPIORoundTripPreservesFIFO(t *testing.T) {
	source := t.TempDir()
	pipe := filepath.Join(source, "pipe")
	if err := makeFIFO(pipe, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(pipe, 0o640); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "fifo.cpio")
	if _, errOut, code := exec(t, source, "", "-w", "-x", "cpio", "-f", archive, "pipe"); code != 0 || errOut != "" {
		t.Fatalf("write cpio: code=%d stderr=%q", code, errOut)
	}

	destination := t.TempDir()
	if _, errOut, code := exec(t, destination, "", "-r", "-f", archive); code != 0 || errOut != "" {
		t.Fatalf("extract cpio: code=%d stderr=%q", code, errOut)
	}
	info, err := os.Lstat(filepath.Join(destination, "pipe"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("extracted mode %v is not a FIFO", info.Mode())
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("extracted FIFO mode = %03o, want 640", got)
	}
}

func TestFIFOExtractionStillRejectsEscapingPathBeforeMutation(t *testing.T) {
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	if err := tw.WriteHeader(&tar.Header{Name: "../escape-pipe", Typeflag: tar.TypeFifo, Mode: 0o600}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	_, errOut, code := exec(t, destination, archive.String(), "-r")
	if code == 0 || !strings.Contains(errOut, "nothing was extracted") {
		t.Fatalf("escaping FIFO: code=%d stderr=%q", code, errOut)
	}
	if _, err := os.Lstat(filepath.Join(destination, "..", "escape-pipe")); !os.IsNotExist(err) {
		t.Fatalf("escaping FIFO was created or produced unexpected error: %v", err)
	}

	for _, tc := range []struct {
		name    string
		headers []*tar.Header
	}{
		{
			name: "regular then FIFO",
			headers: []*tar.Header{
				{Name: "same", Typeflag: tar.TypeReg, Size: 1},
				{Name: "same", Typeflag: tar.TypeFifo, Mode: 0o600},
			},
		},
		{
			name: "FIFO then regular",
			headers: []*tar.Header{
				{Name: "same", Typeflag: tar.TypeFifo, Mode: 0o600},
				{Name: "same", Typeflag: tar.TypeReg, Size: 1},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var archive bytes.Buffer
			tw := tar.NewWriter(&archive)
			for _, h := range tc.headers {
				if err := tw.WriteHeader(h); err != nil {
					t.Fatal(err)
				}
				if h.Size > 0 {
					if _, err := tw.Write([]byte("x")); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			destination := t.TempDir()
			_, errOut, code := exec(t, destination, archive.String(), "-r")
			if code == 0 || !strings.Contains(errOut, "duplicate destination") || !strings.Contains(errOut, "nothing was extracted") {
				t.Fatalf("mixed-kind duplicate: code=%d stderr=%q", code, errOut)
			}
			if _, err := os.Lstat(filepath.Join(destination, "same")); !os.IsNotExist(err) {
				t.Fatalf("mixed-kind archive mutated destination: %v", err)
			}
		})
	}

	t.Run("unsupported platform preserves existing target", func(t *testing.T) {
		old := fifoSupportedForExtraction
		fifoSupportedForExtraction = func() bool { return false }
		t.Cleanup(func() { fifoSupportedForExtraction = old })

		var archive bytes.Buffer
		tw := tar.NewWriter(&archive)
		if err := tw.WriteHeader(&tar.Header{Name: "keep", Typeflag: tar.TypeFifo, Mode: 0o600}); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		destination := t.TempDir()
		target := filepath.Join(destination, "keep")
		if err := os.WriteFile(target, []byte("preserve me"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, errOut, code := exec(t, destination, archive.String(), "-r")
		if code == 0 || !strings.Contains(errOut, "FIFO extraction is not supported") {
			t.Fatalf("unsupported FIFO: code=%d stderr=%q", code, errOut)
		}
		body, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "preserve me" {
			t.Fatalf("existing target body = %q", body)
		}

		var nestedArchive bytes.Buffer
		tw = tar.NewWriter(&nestedArchive)
		if err := tw.WriteHeader(&tar.Header{Name: "before", Typeflag: tar.TypeReg, Size: 1, Mode: 0o600}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		if err := tw.WriteHeader(&tar.Header{Name: "new-parent/pipe", Typeflag: tar.TypeFifo, Mode: 0o600}); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		cleanDestination := t.TempDir()
		_, _, code = exec(t, cleanDestination, nestedArchive.String(), "-r")
		if code == 0 {
			t.Fatal("unsupported nested FIFO unexpectedly succeeded")
		}
		if _, err := os.Lstat(filepath.Join(cleanDestination, "before")); !os.IsNotExist(err) {
			t.Fatalf("unsupported FIFO archive extracted an earlier member: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(cleanDestination, "new-parent")); !os.IsNotExist(err) {
			t.Fatalf("unsupported FIFO created a parent directory: %v", err)
		}
	})
}
