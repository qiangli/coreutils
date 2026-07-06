// Package java provisions a Temurin (Eclipse Adoptium) JDK on demand so `bashy
// java`/`javac` — and `bashy mvn` (Apache Maven) — work on a bare node with no
// system Java: the Adoptium API supplies the exact per-platform archive + its
// sha256 + the (variable) extracted dir name; binmgr tree-mode downloads →
// verifies → extracts → caches. Maven comes from the official Apache dist,
// sha512-verified. JAVA_HOME is wired for mvn. No embedding. Both permissive.
package java

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/binmgr"
)

// DefaultFeature is the JDK feature version installed by default (current LTS).
// Override via $BASHY_JAVA_VERSION (e.g. "17", "21", "23").
const DefaultFeature = "21"

// DefaultMavenVersion is the Apache Maven provisioned for `bashy mvn`.
const DefaultMavenVersion = "3.9.9"

func adoptiumOSArch() (os_, arch string, err error) {
	switch runtime.GOOS {
	case "linux":
		os_ = "linux"
	case "darwin":
		os_ = "mac"
	case "windows":
		os_ = "windows"
	default:
		return "", "", fmt.Errorf("java: unsupported OS %q", runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "aarch64"
	default:
		return "", "", fmt.Errorf("java: unsupported arch %q", runtime.GOARCH)
	}
	return os_, arch, nil
}

type adoptiumAsset struct {
	Binary struct {
		Package struct {
			Link     string `json:"link"`
			Name     string `json:"name"`
			Checksum string `json:"checksum"` // sha256
		} `json:"package"`
	} `json:"binary"`
	ReleaseName string `json:"release_name"` // e.g. jdk-21.0.5+11 — the extracted top dir
}

// EnsureJDK provisions the Temurin JDK for the requested feature version and
// returns (javaBin, javaHome). Idempotent via binmgr's cache.
func EnsureJDK(ctx context.Context, feature string) (javaBin, javaHome string, err error) {
	feature = strings.TrimSpace(feature)
	if feature == "" {
		feature = DefaultFeature
	}
	os_, arch, err := adoptiumOSArch()
	if err != nil {
		return "", "", err
	}
	api := fmt.Sprintf("https://api.adoptium.net/v3/assets/latest/%s/hotspot?architecture=%s&image_type=jdk&os=%s&vendor=eclipse", feature, arch, os_)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", "", fmt.Errorf("java: adoptium API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("java: adoptium API HTTP %d for JDK %s %s/%s", resp.StatusCode, feature, os_, arch)
	}
	body, _ := io.ReadAll(resp.Body)
	var assets []adoptiumAsset
	if err := json.Unmarshal(body, &assets); err != nil || len(assets) == 0 {
		return "", "", fmt.Errorf("java: no Temurin JDK %s for %s/%s", feature, os_, arch)
	}
	a := assets[0]
	if a.Binary.Package.Checksum == "" {
		return "", "", fmt.Errorf("java: adoptium returned no checksum for %s (refusing unverified)", a.Binary.Package.Name)
	}
	// Extracted layout: <release_name>/... ; on macOS the JDK is under Contents/Home.
	home := a.ReleaseName
	if runtime.GOOS == "darwin" {
		home = a.ReleaseName + "/Contents/Home"
	}
	entry := home + "/bin/java"
	if runtime.GOOS == "windows" {
		entry = home + "/bin/java.exe"
	}
	tool := binmgr.Tool{
		Name: "temurin-jdk-" + feature, Version: a.ReleaseName,
		Assets: map[string]binmgr.Asset{
			binmgr.Platform(): {URL: a.Binary.Package.Link, SHA256: a.Binary.Package.Checksum, Tree: true, Entrypoint: entry},
		},
	}
	javaBin, err = binmgr.Ensure(ctx, tool)
	if err != nil {
		return "", "", err
	}
	// javaBin = <cache>/.../<home>/bin/java → JAVA_HOME = <home>.
	return javaBin, filepath.Dir(filepath.Dir(javaBin)), nil
}

// EnsureMaven provisions Apache Maven (sha512-verified) and returns the mvn
// launcher path. Maven itself is a JVM app — the caller wires JAVA_HOME.
func EnsureMaven(ctx context.Context, version string) (string, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		version = DefaultMavenVersion
	}
	base := "apache-maven-" + version
	entry := base + "/bin/mvn"
	if runtime.GOOS == "windows" {
		entry = base + "/bin/mvn.cmd"
	}
	// archive.apache.org keeps EVERY released version permanently (dlcdn.apache.org
	// prunes to current only, so a pinned version 404s once it's superseded).
	url := fmt.Sprintf("https://archive.apache.org/dist/maven/maven-3/%s/binaries/%s-bin.tar.gz", version, base)
	sha512, err := resolveSHA512(ctx, url)
	if err != nil {
		return "", err
	}
	tool := binmgr.Tool{
		Name: "maven", Version: version,
		Assets: map[string]binmgr.Asset{
			binmgr.Platform(): {URL: url, SHA512: sha512, Tree: true, Entrypoint: entry},
		},
	}
	return binmgr.Ensure(ctx, tool)
}

// resolveSHA512 fetches Apache's official .sha512 sidecar (Apache dist ships
// sha512, not sha256). Fails — never empty — so binmgr fail-closes.
func resolveSHA512(ctx context.Context, url string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url+".sha512", nil)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("java/maven: fetch sha512 sidecar: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("java/maven: sha512 sidecar HTTP %d for %s", resp.StatusCode, url)
	}
	body, _ := io.ReadAll(resp.Body)
	if f := strings.Fields(string(body)); len(f) > 0 && len(f[0]) == 128 {
		return f[0], nil
	}
	return "", fmt.Errorf("java/maven: malformed sha512 sidecar for %s", url)
}

// NewJavaCmd / NewJavacCmd run java/javac from the provisioned JDK.
func NewJavaCmd() *cobra.Command  { return newJDKCmd("java", "Java runtime", "java") }
func NewJavacCmd() *cobra.Command { return newJDKCmd("javac", "Java compiler", "javac") }

func newJDKCmd(use, short, tool string) *cobra.Command {
	return &cobra.Command{
		Use:                use,
		Short:              "Run the " + short + ", auto-provisioned Temurin JDK (download + verify + cache, no system Java)",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			javaBin, javaHome, err := EnsureJDK(cmd.Context(), os.Getenv("BASHY_JAVA_VERSION"))
			if err != nil {
				return err
			}
			exe := javaBin // java
			if tool == "javac" {
				exe = filepath.Join(filepath.Dir(javaBin), "javac")
				if runtime.GOOS == "windows" {
					exe += ".exe"
				}
			}
			c := exec.CommandContext(cmd.Context(), exe, args...)
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			c.Env = append(os.Environ(), "JAVA_HOME="+javaHome)
			return c.Run()
		},
	}
}

// NewMvnCmd runs Apache Maven, provisioning both the JDK (for JAVA_HOME) and mvn.
func NewMvnCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "mvn",
		Short:              "Run Apache Maven, auto-provisioned with its JDK (download + verify + cache)",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			javaBin, javaHome, err := EnsureJDK(cmd.Context(), os.Getenv("BASHY_JAVA_VERSION"))
			if err != nil {
				return err
			}
			mvn, err := EnsureMaven(cmd.Context(), os.Getenv("BASHY_MAVEN_VERSION"))
			if err != nil {
				return err
			}
			c := exec.CommandContext(cmd.Context(), mvn, args...)
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			c.Env = append(os.Environ(),
				"JAVA_HOME="+javaHome,
				"PATH="+filepath.Dir(javaBin)+string(os.PathListSeparator)+os.Getenv("PATH"))
			return c.Run()
		},
	}
}
