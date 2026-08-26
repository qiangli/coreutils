//go:build unix

package mesgcmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/creack/pty/v2"
	"github.com/qiangli/coreutils/tool"
)

func TestDefaultTTYNameUsesRunContextStreamsInOrder(t *testing.T) {
	type terminal struct {
		master *os.File
		slave  *os.File
	}
	openTerminal := func() terminal {
		ptm, pts, err := pty.Open()
		if err != nil {
			t.Skipf("pty.Open failed: %v", err)
		}
		t.Cleanup(func() { _ = ptm.Close(); _ = pts.Close() })
		return terminal{master: ptm, slave: pts}
	}
	stdinTTY, stdoutTTY, stderrTTY := openTerminal(), openTerminal(), openTerminal()

	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()

	for _, tc := range []struct {
		name string
		in   io.Reader
		out  io.Writer
		err  io.Writer
		want *os.File
	}{
		{"stdin first", stdinTTY.slave, stdoutTTY.slave, stderrTTY.slave, stdinTTY.slave},
		{"stdout fallback", null, stdoutTTY.slave, stderrTTY.slave, stdoutTTY.slave},
		{"stderr fallback", null, &bytes.Buffer{}, stderrTTY.slave, stderrTTY.slave},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rc := &tool.RunContext{Stdio: tool.Stdio{In: tc.in, Out: tc.out, Err: tc.err}}
			got, err := defaultTTYName(rc)
			if err != nil {
				t.Fatalf("terminal was not found: %v", err)
			}
			gotInfo, err := os.Stat(got)
			if err != nil {
				t.Fatalf("stat resolved terminal %q: %v", got, err)
			}
			wantInfo, err := tc.want.Stat()
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(gotInfo, wantInfo) {
				t.Fatalf("resolved %q, not selected terminal %q", got, tc.want.Name())
			}
		})
	}
}

func TestMesgChangesRealPTYPermissionAndStatus(t *testing.T) {
	ptm, pts, err := pty.Open()
	if err != nil {
		t.Skipf("pty.Open failed: %v", err)
	}
	defer ptm.Close()
	defer pts.Close()

	original, err := pts.Stat()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(pts.Name(), original.Mode().Perm()) })
	if err := os.Chmod(pts.Name(), original.Mode().Perm()&^0o020); err != nil {
		t.Skipf("cannot change PTY permissions: %v", err)
	}

	runPTY := func(args ...string) (string, string, int) {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{Stdio: tool.Stdio{In: pts, Out: &out, Err: &errb}}
		code := run(rc, args)
		return out.String(), errb.String(), code
	}

	_, _, code := runPTY()
	if code != 1 {
		t.Fatalf("real PTY denied query exit=%d, want 1", code)
	}
	_, errb, code := runPTY("y")
	if code != 0 || errb != "" {
		t.Fatalf("mesg y on real PTY = (%q, %d)", errb, code)
	}
	fi, err := pts.Stat()
	if err != nil || fi.Mode().Perm()&0o020 == 0 {
		t.Fatalf("mesg y did not set PTY g+w: mode=%v err=%v", fi, err)
	}
	_, errb, code = runPTY("n")
	if code != 1 || errb != "" {
		t.Fatalf("mesg n on real PTY = (%q, %d)", errb, code)
	}
	fi, err = pts.Stat()
	if err != nil || fi.Mode().Perm()&0o020 != 0 {
		t.Fatalf("mesg n did not clear PTY g+w: mode=%v err=%v", fi, err)
	}
}

func TestDefaultTTYNameRejectsCharacterDeviceThatIsNotTerminal(t *testing.T) {
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()

	var out, errb bytes.Buffer
	rc := &tool.RunContext{Stdio: tool.Stdio{In: null, Out: &out, Err: &errb}}
	if got, err := defaultTTYName(rc); err == nil {
		t.Fatalf("/dev/null resolved as terminal %q", got)
	}
}
