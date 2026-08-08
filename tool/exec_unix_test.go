//go:build !windows

package tool

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartCommandFallsBackToShellWithoutHostEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain-script")
	const hostOnly = "COREUTILS_START_COMMAND_HOST_ONLY"
	t.Setenv(hostOnly, "must-not-leak")
	content := "printf '%s|%s|%s\\n' \"${" + hostOnly + "-missing}\" \"$1\" \"$2\"\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	rc := &RunContext{Ctx: context.Background(), Dir: dir, Env: nil}
	c, err := rc.StartCommand(path, []string{"left", "right"}, nil, &out, &out)
	if err != nil {
		t.Fatalf("StartCommand: %v", err)
	}
	if err := c.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "missing|left|right" {
		t.Fatalf("output=%q, want nil Env isolated and arguments preserved", got)
	}
}
