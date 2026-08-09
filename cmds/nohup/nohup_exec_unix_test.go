//go:build unix

package nohupcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestNohupExecutableTextFallbackPreservesArgsAndEmptyEnv(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "plain=text-command")
	body := "printf 'zero=%s;argc=%s;' \"$0\" \"$#\"\n" +
		"for arg do printf '<%s>' \"$arg\"; done\n" +
		"printf ';leak=%s' \"${NOHUP_HOST_LEAK-unset}\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOHUP_HOST_LEAK", "host-value")

	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   nil,
		Stdio: tool.Stdio{Out: &out, Err: &errOut},
	}
	if code := run(rc, []string{script, "two words", ""}); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
	want := "zero=" + script + ";argc=2;<two words><>;leak=unset"
	if out.String() != want {
		t.Fatalf("stdout=%q, want %q", out.String(), want)
	}
}
