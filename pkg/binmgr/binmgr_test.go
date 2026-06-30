package binmgr

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func sha256hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func toolFor(url, sha, member string) Tool {
	return Tool{
		Name: "faketool", Version: "1.0.0",
		Assets: map[string]Asset{Platform(): {URL: url, SHA256: sha, Binary: member}},
	}
}

// Download → verify → cache → return; second call is a cache hit (no network).
func TestEnsure_DownloadVerifyCacheHit(t *testing.T) {
	payload := []byte("#!/bin/sh\necho hello\n")
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	t.Setenv("BASHY_BIN_CACHE", t.TempDir())
	tool := toolFor(srv.URL+"/bin", sha256hex(payload), "")

	path, err := Ensure(context.Background(), tool)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, payload) {
		t.Fatalf("cached content mismatch")
	}
	if !isExecutable(path) { // exec-bit on unix; .exe/any-file on Windows
		t.Fatalf("cached binary is not executable: %s", path)
	}
	if hits != 1 {
		t.Fatalf("expected 1 download, got %d", hits)
	}

	// Second call: cache hit, no new download (server would 500 if hit, but it
	// shouldn't be).
	path2, err := Ensure(context.Background(), tool)
	if err != nil || path2 != path {
		t.Fatalf("cache-hit Ensure: path=%q err=%v", path2, err)
	}
	if hits != 1 {
		t.Fatalf("cache hit re-downloaded: %d hits", hits)
	}
}

// A sha256 mismatch is rejected and nothing is cached.
func TestEnsure_ShaMismatch(t *testing.T) {
	payload := []byte("real bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	t.Setenv("BASHY_BIN_CACHE", t.TempDir())
	tool := toolFor(srv.URL+"/bin", sha256hex([]byte("DIFFERENT")), "")

	if _, err := Ensure(context.Background(), tool); err == nil {
		t.Fatal("expected sha256 mismatch error, got nil")
	}
	// A subsequent correct fetch must still work (no poisoned cache).
	tool.Assets[Platform()] = Asset{URL: srv.URL + "/bin", SHA256: sha256hex(payload)}
	if _, err := Ensure(context.Background(), tool); err != nil {
		t.Fatalf("correct fetch after mismatch: %v", err)
	}
}

// No asset for the current platform → a clear error.
func TestEnsure_NoAssetForPlatform(t *testing.T) {
	tool := Tool{Name: "x", Version: "1", Assets: map[string]Asset{"plan9/foo": {URL: "http://x"}}}
	if _, err := Ensure(context.Background(), tool); err == nil {
		t.Fatal("expected no-asset error")
	}
}

// Archive extraction: a .tar.gz with the binary at a member path.
func TestEnsure_ExtractTarGz(t *testing.T) {
	bin := []byte("binary-inside-archive")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "faketool/faketool", Mode: 0o755, Size: int64(len(bin))})
	_, _ = tw.Write(bin)
	_ = tw.Close()
	_ = gz.Close()
	archive := buf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	t.Setenv("BASHY_BIN_CACHE", t.TempDir())
	tool := toolFor(srv.URL+"/faketool.tar.gz", sha256hex(archive), "faketool/faketool")

	path, err := Ensure(context.Background(), tool)
	if err != nil {
		t.Fatalf("Ensure(tar.gz): %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, bin) {
		t.Fatalf("extracted content mismatch: %q", got)
	}
}

// Tree extraction ("recursive" Ensure): the WHOLE archive is unpacked and the
// Entrypoint is returned — a Go-toolchain-style layout where the binary needs
// its sibling tree to run.
func TestEnsure_ExtractTree(t *testing.T) {
	binv := []byte("go-binary")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "go/bin/go", Mode: 0o755, Size: int64(len(binv))})
	_, _ = tw.Write(binv)
	_ = tw.WriteHeader(&tar.Header{Name: "go/VERSION", Mode: 0o644, Size: 7})
	_, _ = tw.Write([]byte("go1.2.3"))
	_ = tw.Close()
	_ = gz.Close()
	archive := buf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	t.Setenv("BASHY_BIN_CACHE", t.TempDir())
	tool := Tool{
		Name: "go", Version: "1.2.3",
		Assets: map[string]Asset{Platform(): {
			URL: srv.URL + "/go.tar.gz", SHA256: sha256hex(archive),
			Tree: true, Entrypoint: "go/bin/go",
		}},
	}
	path, err := Ensure(context.Background(), tool)
	if err != nil {
		t.Fatalf("Ensure(tree): %v", err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, binv) {
		t.Fatalf("entrypoint content mismatch: %q", got)
	}
	// The sibling tree file proves the whole archive was extracted, not just the
	// entrypoint member.
	ver, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(path)), "VERSION"))
	if err != nil || string(ver) != "go1.2.3" {
		t.Fatalf("sibling tree file not extracted: %v %q", err, ver)
	}
	// Cache hit: a second Ensure resolves with no network (server closed).
	srv.Close()
	if path2, err := Ensure(context.Background(), tool); err != nil || path2 != path {
		t.Fatalf("tree cache-hit failed: %v %q", err, path2)
	}
}

// Member matches by basename: the binary is nested in a versioned subdir
// (Kopia's layout), and Member "kopia" finds it without knowing the version.
func TestEnsure_ExtractTarGz_NestedMember(t *testing.T) {
	bin := []byte("kopia-binary")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "kopia-0.18.0-linux-x64/LICENSE", Mode: 0o644, Size: 3})
	_, _ = tw.Write([]byte("MIT"))
	_ = tw.WriteHeader(&tar.Header{Name: "kopia-0.18.0-linux-x64/kopia", Mode: 0o755, Size: int64(len(bin))})
	_, _ = tw.Write(bin)
	_ = tw.Close()
	_ = gz.Close()
	archive := buf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	t.Setenv("BASHY_BIN_CACHE", t.TempDir())
	tool := Tool{
		Name: "kopia", Version: "0.18.0",
		Assets: map[string]Asset{Platform(): {URL: srv.URL + "/kopia.tar.gz", SHA256: sha256hex(archive), Binary: "kopia"}},
	}
	path, err := Ensure(context.Background(), tool)
	if err != nil {
		t.Fatalf("Ensure(nested): %v", err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, bin) {
		t.Fatalf("nested extract mismatch: %q", got)
	}
}
