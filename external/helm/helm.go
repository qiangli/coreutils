// Package helm runs the Helm CLI (kubernetes package manager) as a managed
// external binary (pkg/binmgr): downloaded from get.helm.sh → sha256-verified →
// cached, never compiled in. `bashy helm …` is a transparent passthrough that
// targets the dhnt (DKS) cluster by default (external/kube). helm/helm is
// Apache-2.0.
package helm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/external/kube"
	"github.com/qiangli/coreutils/pkg/binmgr"
)

// EnvVersion pins the helm release (e.g. v3.16.3); unset resolves the latest from
// GitHub.
const EnvVersion = "HELM_VERSION"

const latestURL = "https://api.github.com/repos/helm/helm/releases/latest"

// Spec is the binmgr URL spec. get.helm.sh serves a per-platform .tar.gz that
// unpacks to <goos>-<goarch>/helm, with a "<digest>  <file>" .sha256sum sidecar.
// helm's os/arch tokens match Go's, so no aliases are needed.
func Spec(version string) binmgr.URLSpec {
	member := fmt.Sprintf("%s-%s/%s", runtime.GOOS, runtime.GOARCH, kube.ExecName("helm"))
	return binmgr.URLSpec{
		Name:                "helm",
		Version:             version,
		URLTemplate:         "https://get.helm.sh/helm-{version}-{goos}-{goarch}.tar.gz",
		ChecksumURLTemplate: "https://get.helm.sh/helm-{version}-{goos}-{goarch}.tar.gz.sha256sum",
		Member:              member,
	}
}

// latestVersion resolves the newest helm release tag from GitHub.
func latestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("helm: GET latest release: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(b, &rel); err != nil {
		return "", err
	}
	if !strings.HasPrefix(rel.TagName, "v") {
		return "", fmt.Errorf("helm: unexpected latest tag %q", rel.TagName)
	}
	return rel.TagName, nil
}

// Ensure fetches (if needed) the helm binary and returns its cached path.
// Cache-first: an already-downloaded helm (any version) is used with no network,
// unless a version is pinned via $HELM_VERSION.
func Ensure(ctx context.Context, version string) (string, error) {
	if version == "" {
		if p := kube.CachedBinary("helm"); p != "" {
			return p, nil
		}
		v, err := latestVersion(ctx)
		if err != nil {
			return "", fmt.Errorf("helm: resolve latest version: %w", err)
		}
		version = v
	}
	tool, err := binmgr.ResolveURL(ctx, Spec(version))
	if err != nil {
		return "", fmt.Errorf("helm: resolve: %w", err)
	}
	return binmgr.Ensure(ctx, tool)
}

// NewHelmCmd is the `bashy helm` front-door: a transparent passthrough to the
// managed helm binary, pointed at the DKS cluster by default. $HELM_VERSION pins
// the release; all args pass through unchanged.
func NewHelmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "helm",
		Short: "Helm (kubernetes package manager) targeting the DKS cluster (managed external binary)",
		Long: `helm (helm/helm, Apache-2.0) downloaded from get.helm.sh, sha256-verified, and
cached by binmgr (not compiled in). By default it targets the dhnt/DKS cluster:
an unset $KUBECONFIG falls back to outpost's DKS kubeconfig
($OUTPOST_KUBECONFIG_PATH or ~/.kube/outpost.yaml — write it with
'outpost cluster kubeconfig'). $HELM_VERSION pins the release; all args pass
through to helm.`,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			bin, err := Ensure(cmd.Context(), strings.TrimSpace(os.Getenv(EnvVersion)))
			if err != nil {
				return err
			}
			c := exec.CommandContext(cmd.Context(), bin, args...)
			c.Env = kube.ExecEnv()
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			return c.Run()
		},
	}
}
