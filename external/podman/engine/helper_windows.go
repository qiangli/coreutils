//go:build windows

package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/qiangli/coreutils/pkg/binmgr"
)

const gvisorTapVsockVersion = "v0.8.8"

type windowsHelperAsset struct {
	name       string
	assetName  string
	sha256     string
	stagedName string
}

func ensurePlatformHelperBinaries(cacheDir string) error {
	assets, err := windowsHelperAssets()
	if err != nil {
		return err
	}
	for _, asset := range assets {
		path, err := binmgr.Ensure(context.Background(), binmgr.Tool{
			Name:    asset.name,
			Version: gvisorTapVsockVersion,
			Assets: map[string]binmgr.Asset{
				binmgr.Platform(): {
					URL:    "https://github.com/containers/gvisor-tap-vsock/releases/download/" + gvisorTapVsockVersion + "/" + asset.assetName,
					SHA256: asset.sha256,
				},
			},
		})
		if err != nil {
			return fmt.Errorf("ensure %s: %w", asset.stagedName, err)
		}
		stagedPath := filepath.Join(cacheDir, asset.stagedName)
		if err := copyRegularFile(path, stagedPath, 0o755); err != nil {
			return fmt.Errorf("stage %s: %w", asset.stagedName, err)
		}
		slog.Info("container: using managed windows helper", "path", stagedPath)
	}
	return nil
}

func windowsHelperAssets() ([]windowsHelperAsset, error) {
	switch runtime.GOARCH {
	case "amd64":
		return []windowsHelperAsset{
			{
				name:       "gvproxy",
				assetName:  "gvproxy-windowsgui.exe",
				sha256:     "8803caf895325dc2ea52337fa2c7c835c1f7f115b0bde71fdb1479d1b3710526",
				stagedName: "gvproxy.exe",
			},
			{
				name:       "win-sshproxy",
				assetName:  "win-sshproxy.exe",
				sha256:     "afa4c0d97787f2a4e6509cfe472e9d2ceb5fcfd41a870e66687aa314909b4d10",
				stagedName: "win-sshproxy.exe",
			},
		}, nil
	case "arm64":
		return []windowsHelperAsset{
			{
				name:       "gvproxy",
				assetName:  "gvproxy-windows-arm64.exe",
				sha256:     "c2ee761781e58604438b2686531ba2572dce4933f2a4cbccf5da79247bc93412",
				stagedName: "gvproxy.exe",
			},
			{
				name:       "win-sshproxy",
				assetName:  "win-sshproxy-arm64.exe",
				sha256:     "f38633a252a8916342db95f697d0f992a7494e0e74cf11e6e7432892d7fa0916",
				stagedName: "win-sshproxy.exe",
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported Windows architecture %s", runtime.GOARCH)
	}
}

func copyRegularFile(src, dest string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp, err := os.OpenFile(dest+".tmp", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), dest)
}
