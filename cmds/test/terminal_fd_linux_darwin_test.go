//go:build linux || darwin

package testcmd

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/creack/pty/v2"

	"github.com/qiangli/coreutils/tool"
)

const terminalFDHelper = "COREUTILS_TEST_TERMINAL_FD_HELPER"

// TestTerminalInheritedDescriptor uses a real process boundary and places a
// pty on descriptor 3, matching the POSIX VSC assertion's
// `test -t 3 3>/dev/tty` process shape. Both registered spellings share the
// predicate but are exercised independently to pin their argument framing.
func TestTerminalInheritedDescriptor(t *testing.T) {
	if spelling := os.Getenv(terminalFDHelper); spelling != "" {
		rc := &tool.RunContext{
			Ctx: context.Background(),
			Stdio: tool.Stdio{
				In:  os.Stdin,
				Out: os.Stdout,
				Err: os.Stderr,
			},
		}
		if spelling == "bracket" {
			os.Exit(bracketCmd.Run(rc, []string{"-t", "3", "]"}))
		}
		os.Exit(cmd.Run(rc, []string{"-t", "3"}))
	}

	ptm, tty, err := pty.Open()
	if err != nil {
		t.Skipf("cannot open pty: %v", err)
	}
	defer ptm.Close()
	defer tty.Close()

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, spelling := range []string{"test", "bracket"} {
		t.Run(spelling, func(t *testing.T) {
			child := exec.Command(exe, "-test.run=^TestTerminalInheritedDescriptor$")
			child.Env = append(os.Environ(), terminalFDHelper+"="+spelling)
			child.ExtraFiles = []*os.File{tty} // first ExtraFile is fd 3
			if output, err := child.CombinedOutput(); err != nil {
				t.Fatalf("%s -t 3 with inherited pty: %v\n%s", spelling, err, output)
			}
		})
	}
}
