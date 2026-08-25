package iconvcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func invokeDir(t *testing.T, dir, input string, args ...string) (int, []byte, string) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir,
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &errb},
	}
	return run(rc, args), out.Bytes(), errb.String()
}

// A discard recorded for one operand must survive the later operands:
// the exit status covers the whole invocation, so a trailing empty file
// must not launder an earlier operand's discarded characters back to 0.
func TestDiscardStatusSurvivesLaterEmptyOperand(t *testing.T) {
	dir := t.TempDir()
	// A single stray byte cannot be UTF-16; -c discards it.
	if err := os.WriteFile(filepath.Join(dir, "bad"), []byte{'A'}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "empty"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := invokeDir(t, dir, "", "-c", "-f", "UTF-16", "-t", "UTF-8", "bad", "empty")
	if code == 0 {
		t.Fatalf("discarded characters in the first operand must keep the failure status; got 0 (out=%q)", out)
	}
	if len(out) != 0 {
		t.Errorf("out=%q, want empty", out)
	}
}

// A GB18030 four-byte sequence truncated at end of input is a discarded
// character under -c and must set the failure status, like every other
// discard path.
func TestDiscardTruncatedGB18030FourByteTailFails(t *testing.T) {
	code, out, _ := invoke(t, "\x81\x30", "-c", "-f", "GB18030", "-t", "UTF-8")
	if code == 0 {
		t.Fatalf("truncated four-byte GB18030 tail discarded silently: code=0 out=%q", out)
	}
	// The invalid lead byte is dropped; the trailing ASCII digit survives.
	if string(out) != "0" {
		t.Errorf("out=%q, want %q", out, "0")
	}
}
