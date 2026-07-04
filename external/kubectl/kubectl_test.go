package kubectl

import (
	"strings"
	"testing"
)

func TestSpec(t *testing.T) {
	s := Spec("v1.31.0")
	if s.Name != "kubectl" || s.Version != "v1.31.0" {
		t.Fatalf("unexpected spec: %+v", s)
	}
	if !strings.Contains(s.URLTemplate, "dl.k8s.io/release/{version}/bin/{goos}/{goarch}/kubectl{ext}") {
		t.Errorf("URL template wrong: %s", s.URLTemplate)
	}
	// kubectl ships a bare-digest .sha256 sidecar → binmgr's default (empty
	// ChecksumURLTemplate → "<url>.sha256"). Member empty (raw binary).
	if s.ChecksumURLTemplate != "" {
		t.Errorf("ChecksumURLTemplate should be empty (default .sha256), got %q", s.ChecksumURLTemplate)
	}
	if s.Member != "" {
		t.Errorf("Member should be empty (raw binary), got %q", s.Member)
	}
}
