// Package binmgr is the shared managed-external-binary mechanism for the dhnt
// ecosystem: resolve a (name, version, platform) tool spec → download from its
// own release → sha256-verify → cache → return the executable path. Both bashy
// (the user-facing "OS of binaries" host) and outpost (the lean mesh supervisor)
// call it IN-PROCESS — coreutils is the shared layer both already import — to run
// wrapped tools (loom/Gitea, Zot, SeaweedFS, Kopia, …) without compiling those
// heavy binaries into either. Each tool ships per-platform binaries + sha256 from
// its own CI; binmgr is the one trust/verify/version path for all of them.
//
// It complements external/podman's Resolve (which locates an already-present
// binary): binmgr is the download half. See docs/external-binary-builtins.md.
package binmgr

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Asset is one platform's download for a tool.
type Asset struct {
	// URL is the download URL (a raw binary, or a .tar.gz/.tgz/.zip archive).
	URL string `json:"url"`
	// SHA256 is the expected hex digest of the downloaded file (preferred). Empty
	// skips the sha check.
	SHA256 string `json:"sha256"`
	// MD5 is the expected hex md5 digest — the fallback integrity check for tools
	// that publish only .md5 sidecars (e.g. SeaweedFS). Used when SHA256 is empty.
	// One of SHA256/MD5 must be set; an empty Asset skips verification entirely
	// (discouraged — the verify is the trust boundary).
	MD5 string `json:"md5,omitempty"`
	// Binary is the path to the executable WITHIN an archive (e.g.
	// "gitea/gitea"); empty means the download is itself the raw binary.
	Binary string `json:"binary,omitempty"`
	// Tree requests whole-archive ("recursive") extraction instead of pulling a
	// single Binary member: the entire .tar.gz/.zip tree is unpacked into the
	// tool's cache dir, preserving modes and symlinks. Use it for tools that need
	// their full layout to run (a Go toolchain's bin/+pkg/+src/, an SDK, …).
	Tree bool `json:"tree,omitempty"`
	// Entrypoint is the executable's slash-separated path within the extracted
	// tree (e.g. "go/bin/go"); required when Tree is set, ignored otherwise.
	Entrypoint string `json:"entrypoint,omitempty"`
}

// Tool is a managed external binary: a logical name, a version (the cache key),
// and per-platform assets keyed by "goos/goarch" (e.g. "linux/amd64").
type Tool struct {
	Name    string           `json:"name"`
	Version string           `json:"version"`
	Assets  map[string]Asset `json:"assets"`
}

// Platform returns the current "goos/goarch" key.
func Platform() string { return runtime.GOOS + "/" + runtime.GOARCH }

// CacheDir is the root for downloaded binaries. Override via $DHNT_BIN_CACHE;
// otherwise <UserCacheDir>/dhnt/bin.
func CacheDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv("DHNT_BIN_CACHE")); d != "" {
		return d, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "dhnt", "bin"), nil
}

func binaryName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Ensure resolves the tool's asset for the current platform, downloading +
// sha256-verifying + caching it if not already present, and returns the path to
// the executable. Idempotent: a cache hit returns immediately with no network.
func Ensure(ctx context.Context, t Tool) (string, error) {
	if t.Name == "" || t.Version == "" {
		return "", fmt.Errorf("binmgr: tool name and version are required")
	}
	asset, ok := t.Assets[Platform()]
	if !ok || asset.URL == "" {
		return "", fmt.Errorf("binmgr: %s %s has no asset for %s", t.Name, t.Version, Platform())
	}
	root, err := CacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, t.Name, t.Version)
	dest := filepath.Join(dir, binaryName(t.Name))
	if asset.Tree {
		if asset.Entrypoint == "" {
			return "", fmt.Errorf("binmgr: %s %s: Tree asset needs an Entrypoint", t.Name, t.Version)
		}
		dest = filepath.Join(dir, filepath.FromSlash(asset.Entrypoint))
	}
	if isExecutable(dest) {
		return dest, nil // cache hit — no network
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	tmp, err := os.CreateTemp(dir, ".dl-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	sum, md5sum, derr := download(ctx, asset.URL, tmp)
	_ = tmp.Close()
	if derr != nil {
		return "", derr
	}
	if want := strings.ToLower(strings.TrimSpace(asset.SHA256)); want != "" {
		if want != sum {
			return "", fmt.Errorf("binmgr: %s %s sha256 mismatch: got %s, want %s", t.Name, t.Version, sum, want)
		}
	} else if want := strings.ToLower(strings.TrimSpace(asset.MD5)); want != "" {
		if want != md5sum {
			return "", fmt.Errorf("binmgr: %s %s md5 mismatch: got %s, want %s", t.Name, t.Version, md5sum, want)
		}
	}

	switch {
	case asset.Tree:
		// Whole-archive extraction into dir/ (preserves the archive's own
		// layout + modes); dest is dir/<entrypoint>.
		if err := extractTree(tmpName, asset.URL, dir); err != nil {
			return "", err
		}
		if !isExecutable(dest) {
			return "", fmt.Errorf("binmgr: %s %s: entrypoint missing after extract: %s", t.Name, t.Version, dest)
		}
		return dest, nil
	case asset.Binary != "":
		if err := extract(tmpName, asset.URL, asset.Binary, dest); err != nil {
			return "", err
		}
	default:
		if err := os.Rename(tmpName, dest); err != nil {
			// cross-device fallback
			if cerr := copyFile(tmpName, dest); cerr != nil {
				return "", err
			}
		}
	}
	if err := os.Chmod(dest, 0o755); err != nil {
		return "", err
	}
	return dest, nil
}

func download(ctx context.Context, url string, w io.Writer) (sha, md5sum string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("binmgr: GET %s: HTTP %d", url, resp.StatusCode)
	}
	sh := sha256.New()
	mh := md5.New()
	if _, err := io.Copy(io.MultiWriter(w, sh, mh), resp.Body); err != nil {
		return "", "", err
	}
	return hex.EncodeToString(sh.Sum(nil)), hex.EncodeToString(mh.Sum(nil)), nil
}

func extract(archivePath, url, member, dest string) error {
	switch {
	case strings.HasSuffix(url, ".zip"):
		return extractZip(archivePath, member, dest)
	case strings.HasSuffix(url, ".tar.gz"), strings.HasSuffix(url, ".tgz"):
		return extractTarGz(archivePath, member, dest)
	default:
		return fmt.Errorf("binmgr: archive member requested but %s is not a .zip/.tar.gz", url)
	}
}

func extractTarGz(archivePath, member, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("binmgr: %q not found in archive", member)
		}
		if err != nil {
			return err
		}
		if matchMember(hdr.Name, member) {
			return writeFrom(tr, dest)
		}
	}
}

func extractZip(archivePath, member, dest string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, zf := range zr.File {
		if matchMember(zf.Name, member) {
			rc, err := zf.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			return writeFrom(rc, dest)
		}
	}
	return fmt.Errorf("binmgr: %q not found in archive", member)
}

// matchMember matches an archive entry against the requested member by full
// cleaned path OR by basename — so a Member like "kopia" finds both a root-level
// "kopia" and a nested "kopia-0.18-linux-x64/kopia" without knowing the version.
func matchMember(entryName, member string) bool {
	return filepath.Clean(entryName) == filepath.Clean(member) || filepath.Base(entryName) == member
}

func writeFrom(r io.Reader, dest string) error {
	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

// extractTree unpacks an entire .tar.gz/.zip into destDir, preserving the
// archive's directory layout, file modes, and symlinks. This is the "recursive"
// counterpart to the single-member extract, for tools that need their whole
// tree (e.g. a Go toolchain).
func extractTree(archivePath, url, destDir string) error {
	switch {
	case strings.HasSuffix(url, ".zip"):
		return extractTreeZip(archivePath, destDir)
	case strings.HasSuffix(url, ".tar.gz"), strings.HasSuffix(url, ".tgz"):
		return extractTreeTarGz(archivePath, destDir)
	default:
		return fmt.Errorf("binmgr: tree extraction needs a .zip/.tar.gz, got %s", url)
	}
}

func extractTreeTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeJoinTree(destDir, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeTreeFile(target, tr, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			_ = os.MkdirAll(filepath.Dir(target), 0o755)
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		}
	}
}

func extractTreeZip(archivePath, destDir string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, zf := range zr.File {
		target, err := safeJoinTree(destDir, zf.Name)
		if err != nil {
			return err
		}
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		err = writeTreeFile(target, rc, zf.Mode())
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeTreeFile(target string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// safeJoinTree guards against path traversal (zip-slip) in archive entries.
func safeJoinTree(destDir, name string) (string, error) {
	target := filepath.Join(destDir, name)
	clean := filepath.Clean(destDir)
	if target != clean && !strings.HasPrefix(target, clean+string(os.PathSeparator)) {
		return "", fmt.Errorf("binmgr: unsafe path in archive: %s", name)
	}
	return target, nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	return writeFrom(in, dest)
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}
