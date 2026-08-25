//go:build unix

package mesgcmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/creack/pty/v2"
	"github.com/qiangli/coreutils/tool"
)

func TestDefaultTTYNameUsesRunContextStreamsInOrder(t *testing.T) {
	ptm, pts, err := pty.Open()
	if err != nil {
		t.Skipf("pty.Open failed: %v", err)
	}
	defer ptm.Close()
	defer pts.Close()

	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()

	rc := &tool.RunContext{Stdio: tool.Stdio{In: null, Out: pts, Err: &bytes.Buffer{}}}
	got, err := defaultTTYName(rc)
	if err != nil {
		t.Fatalf("stdout terminal was not found after non-terminal stdin: %v", err)
	}
	gotInfo, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat resolved terminal %q: %v", got, err)
	}
	wantInfo, err := pts.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("resolved %q, not stdout terminal %q", got, pts.Name())
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
