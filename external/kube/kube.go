// Package kube holds the small pieces shared by the managed kubernetes clients
// `bashy kubectl` and `bashy helm`: cache-first binary resolution and the DKS
// kubeconfig default so both tools target the dhnt cluster out of the box.
//
// Both binaries are Apache-2.0 and are DOWNLOADED + exec'd as separate processes
// (never linked), the same trust path as every other binmgr-managed external.
package kube

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/qiangli/coreutils/pkg/binmgr"
)

const dksKubeconfigRefreshAfter = 50 * time.Minute

// ExecName is the on-disk basename for a tool (adds .exe on Windows) — the name
// binmgr.Ensure caches it under (<CacheDir>/<name>/<version>/<execname>).
func ExecName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// CachedBinary returns an already-downloaded tool binary from binmgr's cache
// (newest version wins) so a cache hit needs no network — the hot path for a CLI
// an agent calls repeatedly. Returns "" when nothing is cached.
func CachedBinary(name string) string {
	root, err := binmgr.CacheDir()
	if err != nil {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(root, name, "*", ExecName(name)))
	sort.Strings(matches) // version dirs sort lexically; good enough, newest-ish last
	for i := len(matches) - 1; i >= 0; i-- {
		if fi, err := os.Stat(matches[i]); err == nil && !fi.IsDir() {
			return matches[i]
		}
	}
	return ""
}

// DKSKubeconfig returns the DKS kubeconfig path to use, or "" to leave the tool's
// own default (~/.kube/config) in place. It NEVER overrides an explicit
// $KUBECONFIG. Otherwise it prefers the kubeconfig `outpost cluster kubeconfig`
// writes — $OUTPOST_KUBECONFIG_PATH, else ~/.kube/outpost.yaml — when it exists,
// so `bashy kubectl get nodes` targets the dhnt cluster with no export.
func DKSKubeconfig() string {
	if v := os.Getenv("KUBECONFIG"); v != "" {
		return "" // honor the user's explicit choice
	}
	if p := dksKubeconfigPath(); fileExists(p) {
		return p
	}
	return ""
}

func dksKubeconfigPath() string {
	if p := os.Getenv("OUTPOST_KUBECONFIG_PATH"); p != "" {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".kube", "outpost.yaml")
	}
	return ""
}

// RefreshDKSKubeconfig refreshes the implicit DKS per-user kubeconfig through
// Outpost's credential broker when its one-hour bearer token approaches expiry.
// Explicit KUBECONFIG always wins and is never inspected or changed.
//
// Outpost already owns account credentials and the cloudbox refresh protocol;
// keeping that authority there avoids duplicating secrets or auth logic in
// managed kubectl/helm wrappers. Hosts without Outpost retain the static
// kubeconfig/default-client behavior.
func RefreshDKSKubeconfig(ctx context.Context, stderr io.Writer) {
	if os.Getenv("KUBECONFIG") != "" {
		return
	}
	path := dksKubeconfigPath()
	if path == "" || fileFresh(path, dksKubeconfigRefreshAfter) {
		return
	}
	outpost, err := exec.LookPath("outpost")
	if err != nil {
		return
	}
	cmd := exec.CommandContext(ctx, outpost, "cluster", "userkubeconfig", "--output", path)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil && stderr != nil {
		fmt.Fprintf(stderr, "warning: could not refresh DKS kubeconfig through outpost: %v\n", err)
	}
}

// ExecEnv is the process env for a kubectl/helm passthrough: the inherited env
// plus a DKS KUBECONFIG default when one applies (and KUBECONFIG is unset).
func ExecEnv() []string {
	env := os.Environ()
	if kc := DKSKubeconfig(); kc != "" {
		env = append(env, "KUBECONFIG="+kc)
	}
	return env
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func fileFresh(path string, maxAge time.Duration) bool {
	st, err := os.Stat(path)
	return err == nil && time.Since(st.ModTime()) < maxAge
}
