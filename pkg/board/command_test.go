package board

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func emptySources() []Source { return []Source{} }

func runCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewStewardCommand(func(w io.Writer) error {
		_, err := io.WriteString(w, "existing steward skill\n")
		return err
	}, emptySources())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestStewardNamespaceKeepsBareAndSkill(t *testing.T) {
	for _, args := range [][]string{nil, {"skill"}} {
		out, err := runCommand(t, args...)
		if err != nil || out != "existing steward skill\n" {
			t.Fatalf("steward %v: out=%q err=%v", args, out, err)
		}
	}
}

func TestDashboardFormatsAndOut(t *testing.T) {
	out, err := runCommand(t, "dashboard", "--json")
	if err != nil || !strings.Contains(out, `"schema_version": "bashy-board-v1"`) {
		t.Fatalf("dashboard --json: %v\n%s", err, out)
	}
	out, err = runCommand(t, "dashboard", "--html")
	if err != nil || !strings.HasPrefix(out, "<!doctype html>") {
		t.Fatalf("dashboard --html: %v\n%s", err, out)
	}
	path := filepath.Join(t.TempDir(), "board.html")
	out, err = runCommand(t, "dashboard", "--html", "--out", path)
	if err != nil || out != "" {
		t.Fatalf("dashboard --out: out=%q err=%v", out, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(raw, []byte("<!doctype html>")) {
		t.Fatalf("dashboard output file: %v", err)
	}
}

func TestDashboardRejectsUnknownPanel(t *testing.T) {
	_, err := runCommand(t, "dashboard", "--expand", "future")
	if err == nil || !strings.Contains(err.Error(), "unknown panel") {
		t.Fatalf("error = %v", err)
	}
}
