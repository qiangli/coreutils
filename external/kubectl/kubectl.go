// Package kubectl runs the kubernetes CLI (kubectl) as a managed external binary
// (pkg/binmgr): downloaded from dl.k8s.io → sha256-verified → cached, never
// compiled in. `bashy kubectl …` is a transparent passthrough that targets the
// dhnt (DKS) cluster by default (external/kube: KUBECONFIG → outpost's DKS
// kubeconfig when set up). kubernetes/kubectl is Apache-2.0.
package kubectl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/external/kube"
	"github.com/qiangli/coreutils/pkg/binmgr"
)

// EnvVersion pins the kubectl release (e.g. v1.31.0) for reproducibility; unset
// resolves the current stable from dl.k8s.io. (kubectl's version-skew policy is
// ±1 minor from the cluster, so pinning to match DKS is sometimes wanted.)
const EnvVersion = "KUBECTL_VERSION"

const stableURL = "https://dl.k8s.io/release/stable.txt"

// Spec is the binmgr URL spec: dl.k8s.io serves the raw kubectl binary per
// platform with a bare-digest .sha256 sidecar (binmgr's default when
// ChecksumURLTemplate is empty).
func Spec(version string) binmgr.URLSpec {
	return binmgr.URLSpec{
		Name:        "kubectl",
		Version:     version,
		URLTemplate: "https://dl.k8s.io/release/{version}/bin/{goos}/{goarch}/kubectl{ext}",
	}
}

// stableVersion resolves the current stable kubectl (dl.k8s.io/release/stable.txt).
func stableVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stableURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kubectl: GET stable.txt: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(b))
	if !strings.HasPrefix(v, "v") {
		return "", fmt.Errorf("kubectl: unexpected stable version %q", v)
	}
	return v, nil
}

// Ensure fetches (if needed) the kubectl binary and returns its cached path.
// Cache-first: an already-downloaded kubectl (any version) is used with no
// network, unless a version is pinned via $KUBECTL_VERSION.
func Ensure(ctx context.Context, version string) (string, error) {
	if version == "" {
		if p := kube.CachedBinary("kubectl"); p != "" {
			return p, nil
		}
		v, err := stableVersion(ctx)
		if err != nil {
			return "", fmt.Errorf("kubectl: resolve stable version: %w", err)
		}
		version = v
	}
	tool, err := binmgr.ResolveURL(ctx, Spec(version))
	if err != nil {
		return "", fmt.Errorf("kubectl: resolve: %w", err)
	}
	return binmgr.Ensure(ctx, tool)
}

// NewKubectlCmd is the `bashy kubectl` front-door: a transparent passthrough to
// the managed kubectl binary, pointed at the DKS cluster by default. $KUBECTL_VERSION
// pins the release; all args pass through unchanged.
func NewKubectlCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kubectl",
		Short: "Kubernetes CLI targeting the DKS cluster (managed external binary)",
		Long: `kubectl (kubernetes/kubectl, Apache-2.0) downloaded from dl.k8s.io,
sha256-verified, and cached by binmgr (not compiled in). By default it targets
the dhnt/DKS cluster: an unset $KUBECONFIG falls back to outpost's DKS kubeconfig
($OUTPOST_KUBECONFIG_PATH or ~/.kube/outpost.yaml — write it with
'outpost cluster kubeconfig'). $KUBECTL_VERSION pins the release; all args pass
through to kubectl.`,
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
