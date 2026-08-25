//go:build unix

package paxcmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	osexec "os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/creack/pty/v2"
	"github.com/qiangli/coreutils/tool"
)

// TestInteractiveRenameUsesTheRealControllingPTY runs its helper as the
// foreground process of a fresh PTY. The helper's archive/stdout are ordinary
// streams, so success proves -i opened /dev/tty itself for both the prompt and
// response instead of accidentally consuming standard input or using stderr.
func TestInteractiveRenameUsesTheRealControllingPTY(t *testing.T) {
	if os.Getenv("PAX_REAL_PTY_HELPER") == "1" {
		var out, errOut bytes.Buffer
		rc := &tool.RunContext{
			Dir:   os.Getenv("PAX_REAL_PTY_DIR"),
			Stdio: tool.Stdio{In: strings.NewReader("not a tty response\n"), Out: &out, Err: &errOut},
		}
		code := run(rc, []string{"-i", "-f", os.Getenv("PAX_REAL_PTY_ARCHIVE")})
		if code != 0 || out.String() != "item\n" || errOut.String() != "" {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
		}
		return
	}

	d := t.TempDir()
	if err := os.WriteFile(d+"/item", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	arc := writeArchive(t, d, "item")
	cmd := osexec.Command(os.Args[0], "-test.run=^TestInteractiveRenameUsesTheRealControllingPTY$")
	cmd.Env = append(os.Environ(), "PAX_REAL_PTY_HELPER=1", "PAX_REAL_PTY_DIR="+d, "PAX_REAL_PTY_ARCHIVE="+arc)
	ptm, err := pty.Start(cmd)
	if err != nil {
		t.Skipf("pty.Start failed: %v", err)
	}
	if _, err := ptm.Write([]byte(".\n")); err != nil {
		ptm.Close()
		t.Fatal(err)
	}
	transcript, readErr := io.ReadAll(ptm)
	waitErr := cmd.Wait()
	_ = ptm.Close()
	if readErr != nil && !errors.Is(readErr, syscall.EIO) {
		t.Fatalf("read PTY: %v", readErr)
	}
	if waitErr != nil {
		t.Fatalf("helper: %v transcript=%q", waitErr, transcript)
	}
	if !strings.Contains(string(transcript), "pax: rename item?") {
		t.Fatalf("real PTY transcript=%q", transcript)
	}
}
