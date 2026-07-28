package kube

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDKSKubeconfigHonorsExplicitConfig(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "explicit.yaml"))
	t.Setenv("OUTPOST_KUBECONFIG_PATH", filepath.Join(t.TempDir(), "outpost.yaml"))
	if got := DKSKubeconfig(); got != "" {
		t.Fatalf("DKSKubeconfig() = %q, want empty for explicit KUBECONFIG", got)
	}
}

func TestDKSKubeconfigUsesCanonicalOutpostPath(t *testing.T) {
	t.Setenv("KUBECONFIG", "")
	path := filepath.Join(t.TempDir(), "outpost.yaml")
	t.Setenv("OUTPOST_KUBECONFIG_PATH", path)
	if err := os.WriteFile(path, []byte("config"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := DKSKubeconfig(); got != path {
		t.Fatalf("DKSKubeconfig() = %q, want %q", got, path)
	}
}

func TestFileFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("config"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !fileFresh(path, 50*time.Minute) {
		t.Fatal("new kubeconfig reported stale")
	}
	old := time.Now().Add(-51 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if fileFresh(path, 50*time.Minute) {
		t.Fatal("expired kubeconfig reported fresh")
	}
}
