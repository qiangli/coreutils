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

	t.Setenv("DHNT_BIN_CACHE", t.TempDir())
	tool := toolFor(srv.URL+"/bin", sha256hex(payload), "")

	path, err := Ensure(context.Background(), tool)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, payload) {
		t.Fatalf("cached content mismatch")
	}
	if info, _ := os.Stat(path); info.Mode()&0o111 == 0 {
		t.Fatalf("cached binary is not executable: %v", info.Mode())
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

	t.Setenv("DHNT_BIN_CACHE", t.TempDir())
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

	t.Setenv("DHNT_BIN_CACHE", t.TempDir())
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
