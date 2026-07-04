package binmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

// githubAPI is the GitHub REST base (overridable in tests).
var githubAPI = "https://api.github.com"

// GitHubSpec locates a tool's binary on GitHub releases so we never hand-maintain
// pinned digests: given a repo + version, ResolveGitHub picks the asset for the
// current platform and resolves its sha256 (a per-asset .sha256 sidecar, or a
// checksums file in the release). Reusable for every tool that ships GitHub
// releases (Gitea, Zot, SeaweedFS, Kopia, …).
type GitHubSpec struct {
	// Name is the logical tool name — the cache key and the cached binary name.
	Name string
	// Repo is "owner/repo" (e.g. "go-gitea/gitea").
	Repo string
	// Version is a release tag (e.g. "v1.26.1"); "" or "latest" = latest release.
	Version string
	// Member is the executable's path within a .tar.gz/.zip asset; "" means the
	// matched asset is the raw binary (Gitea, Zot).
	Member string
	// Tree requests whole-archive extraction (Asset.Tree) instead of pulling a
	// single Member: the matched .tar.gz/.zip is unpacked in full, for tools that
	// need their sibling layout to run (e.g. ollama + its ggml runner libs).
	// Requires Entrypoint; Member is ignored.
	Tree bool
	// Entrypoint is the executable's slash path within the extracted tree (e.g.
	// "ollama"); required when Tree is set.
	Entrypoint string
	// AssetMatch overrides the default platform matcher for tools whose asset
	// names don't carry the standard os/arch tokens.
	AssetMatch func(assetName, goos, goarch string) bool
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

// ResolveGitHub turns a GitHubSpec into a Tool for the current platform, ready to
// pass to Ensure/Start.
func ResolveGitHub(ctx context.Context, spec GitHubSpec) (Tool, error) {
	if spec.Name == "" || spec.Repo == "" {
		return Tool{}, fmt.Errorf("binmgr: github spec needs name and repo")
	}
	rel, err := fetchRelease(ctx, spec.Repo, spec.Version)
	if err != nil {
		return Tool{}, err
	}
	goos, goarch := splitPlatform(Platform())
	match := spec.AssetMatch
	if match == nil {
		match = defaultAssetMatch
	}
	if spec.Tree && spec.Entrypoint == "" {
		return Tool{}, fmt.Errorf("binmgr: %s: Tree spec needs an Entrypoint", spec.Repo)
	}
	var asset ghAsset
	for _, a := range rel.Assets {
		if !assetUsable(a.Name, spec.Member, spec.Tree) {
			continue
		}
		if match(a.Name, goos, goarch) {
			asset = a
			break
		}
	}
	if asset.URL == "" {
		return Tool{}, fmt.Errorf("binmgr: %s %s has no asset for %s", spec.Repo, rel.TagName, Platform())
	}
	sha, md5sum, err := resolveChecksum(ctx, rel, asset)
	if err != nil {
		return Tool{}, err
	}
	return Tool{
		Name:    spec.Name,
		Version: rel.TagName,
		Assets: map[string]Asset{
			Platform(): {URL: asset.URL, SHA256: sha, MD5: md5sum, Binary: spec.Member, Tree: spec.Tree, Entrypoint: spec.Entrypoint},
		},
	}, nil
}

func fetchRelease(ctx context.Context, repo, version string) (*ghRelease, error) {
	var path string
	if version == "" || version == "latest" {
		path = fmt.Sprintf("%s/repos/%s/releases/latest", githubAPI, repo)
	} else {
		path = fmt.Sprintf("%s/repos/%s/releases/tags/%s", githubAPI, repo, version)
	}
	body, err := httpGetBody(ctx, path, "application/vnd.github+json")
	if err != nil {
		return nil, fmt.Errorf("binmgr: fetch release %s@%s: %w", repo, version, err)
	}
	var rel ghRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("binmgr: decode release %s: %w", repo, err)
	}
	return &rel, nil
}

func splitPlatform(p string) (goos, goarch string) {
	goos, goarch, _ = strings.Cut(p, "/")
	return goos, goarch
}

func archAliases(goarch string) []string {
	switch goarch {
	case "amd64":
		return []string{"amd64", "x86_64", "x64"}
	case "arm64":
		return []string{"arm64", "aarch64"}
	case "386":
		return []string{"386", "i386", "x86"}
	default:
		return []string{goarch}
	}
}

func defaultAssetMatch(name, goos, goarch string) bool {
	n := strings.ToLower(name)
	if !strings.Contains(n, goos) {
		return false
	}
	for _, a := range archAliases(goarch) {
		if strings.Contains(n, a) {
			return true
		}
	}
	return false
}

// assetUsable filters out sidecars/signatures and, for a raw-binary tool (no
// Member, no Tree), compressed/archived assets; for an archive tool (a Member or
// a Tree request) it requires an extractable archive.
func assetUsable(name, member string, tree bool) bool {
	n := strings.ToLower(name)
	if isSidecar(n) {
		return false
	}
	archive := strings.HasSuffix(n, ".tar.gz") || strings.HasSuffix(n, ".tgz") || strings.HasSuffix(n, ".zip")
	compressed := strings.HasSuffix(n, ".xz") || strings.HasSuffix(n, ".gz") || strings.HasSuffix(n, ".bz2")
	if member == "" && !tree {
		return !archive && !compressed
	}
	return archive
}

func isSidecar(n string) bool {
	for _, s := range []string{".sha256", ".sha512", ".md5", ".asc", ".sig", ".pem", ".cert", ".sbom", ".json", ".txt"} {
		if strings.HasSuffix(n, s) {
			return true
		}
	}
	return false
}

var (
	hex64 = regexp.MustCompile(`[a-fA-F0-9]{64}`)
	hex32 = regexp.MustCompile(`[a-fA-F0-9]{32}`)
)

// resolveChecksum finds the asset's checksum, preferring sha256 (a per-asset
// .sha256 sidecar, then a checksums file) and falling back to a .md5 sidecar for
// tools that publish only md5 (e.g. SeaweedFS). Returns (sha256, md5) — exactly
// one is non-empty.
func resolveChecksum(ctx context.Context, rel *ghRelease, asset ghAsset) (sha, md5sum string, err error) {
	if body, e := httpGetBody(ctx, asset.URL+".sha256", ""); e == nil {
		if h := hex64.FindString(string(body)); h != "" {
			return strings.ToLower(h), "", nil
		}
	}
	for _, a := range rel.Assets {
		ln := strings.ToLower(a.Name)
		if !strings.Contains(ln, "checksum") && !strings.Contains(ln, "sha256sum") && ln != "shasums" && !strings.HasSuffix(ln, ".sha256sum") {
			continue
		}
		body, e := httpGetBody(ctx, a.URL, "")
		if e != nil {
			continue
		}
		if h := digestForFile(string(body), asset.Name, hex64); h != "" {
			return h, "", nil
		}
	}
	// Fallback: a per-asset .md5 sidecar (the only integrity check some tools ship).
	if body, e := httpGetBody(ctx, asset.URL+".md5", ""); e == nil {
		if h := hex32.FindString(string(body)); h != "" {
			return "", strings.ToLower(h), nil
		}
	}
	return "", "", fmt.Errorf("binmgr: no checksum (sha256/md5) found for %s", asset.Name)
}

// digestForFile finds the checksum line naming filename in a `<digest>  filename`
// list and extracts the digest matching re.
func digestForFile(checksums, filename string, re *regexp.Regexp) string {
	for line := range strings.SplitSeq(checksums, "\n") {
		if strings.Contains(line, filename) {
			if h := re.FindString(line); h != "" {
				return strings.ToLower(h)
			}
		}
	}
	return ""
}

func httpGetBody(ctx context.Context, url, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if tok := githubToken(); tok != "" && strings.HasPrefix(url, githubAPI) {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

func githubToken() string {
	for _, k := range []string{"GITHUB_TOKEN", "GIT_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
