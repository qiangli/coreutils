package actrunner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpec(t *testing.T) {
	s := Spec("")
	if s.Name != "act_runner" || s.Version != DefaultVersion {
		t.Fatalf("default spec = %+v", s)
	}
	if !strings.Contains(s.URLTemplate, "dl.gitea.com/act_runner") {
		t.Fatalf("url template not dl.gitea.com: %s", s.URLTemplate)
	}
	if !strings.Contains(s.URLTemplate, "{version}") || !strings.Contains(s.URLTemplate, "{goos}") {
		t.Fatalf("url template missing tokens: %s", s.URLTemplate)
	}
	if Spec("0.2.99").Version != "0.2.99" {
		t.Fatal("version override not honored")
	}
}

func TestRegisteredAndDataDir(t *testing.T) {
	dir := t.TempDir()
	if Registered(dir) {
		t.Fatal("empty dir should not be registered")
	}
	if err := os.WriteFile(filepath.Join(dir, ".runner"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Registered(dir) {
		t.Fatal(".runner present should be registered")
	}
	t.Setenv("ACT_RUNNER_DIR", dir)
	if DefaultDataDir() != dir {
		t.Fatalf("DefaultDataDir = %s, want %s", DefaultDataDir(), dir)
	}
}
