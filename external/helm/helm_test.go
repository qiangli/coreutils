package helm

import (
	"runtime"
	"strings"
	"testing"
)

func TestSpec(t *testing.T) {
	s := Spec("v3.16.3")
	if s.Name != "helm" || s.Version != "v3.16.3" {
		t.Fatalf("unexpected spec: %+v", s)
	}
	if !strings.Contains(s.URLTemplate, "get.helm.sh/helm-{version}-{goos}-{goarch}.tar.gz") {
		t.Errorf("URL template wrong: %s", s.URLTemplate)
	}
	if !strings.HasSuffix(s.ChecksumURLTemplate, ".tar.gz.sha256sum") {
		t.Errorf("checksum template wrong: %s", s.ChecksumURLTemplate)
	}
	// Member points at the binary inside the <goos>-<goarch>/ tree, .exe on Windows.
	wantExe := "helm"
	if runtime.GOOS == "windows" {
		wantExe = "helm.exe"
	}
	want := runtime.GOOS + "-" + runtime.GOARCH + "/" + wantExe
	if s.Member != want {
		t.Errorf("Member = %q, want %q", s.Member, want)
	}
}
