package timecmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func runTime(t *testing.T, env []string, args ...string) (out, errOut string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &o, Err: &e},
	}
	code = cmd.Run(rc, args)
	return o.String(), e.String(), code
}

func TestTimePosixFormat(t *testing.T) {
	_, errOut, code := runTime(t, os.Environ(), "-p", "true")
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	for _, k := range []string{"real ", "user ", "sys "} {
		if !strings.Contains(errOut, k) {
			t.Errorf("-p report missing %q: %q", k, errOut)
		}
	}
}

func TestTimeExitStatusPropagates(t *testing.T) {
	if _, _, code := runTime(t, os.Environ(), "sh", "-c", "exit 7"); code != 7 {
		t.Errorf("exit %d, want 7 (command status propagates)", code)
	}
}

func TestTimeCommandNotFound(t *testing.T) {
	if _, _, code := runTime(t, os.Environ(), "definitely-not-a-real-command-xyz"); code != 127 {
		t.Errorf("not-found exit status = %d, want 127", code)
	}
}

func TestTimeMissingCommand(t *testing.T) {
	if _, _, code := runTime(t, os.Environ(), "-v"); code == 0 {
		t.Error("missing command should be a usage error")
	}
}

func TestTimeFormatSpecifiers(t *testing.T) {
	out, errOut, _ := runTime(t, os.Environ(), "-f", "ELAPSED=%e CMD=%C EXIT=%x", "true")
	_ = out
	if !strings.Contains(errOut, "CMD=true") || !strings.Contains(errOut, "EXIT=0") {
		t.Errorf("-f specifiers not expanded: %q", errOut)
	}
}

func TestTimeNumericLocalePrecedenceAndFormatting(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{"LANG", []string{"LANG=de_DE.iso88591"}, "real 1,25\nuser 0,50\nsys 0,25\n"},
		{"LC_NUMERIC", []string{"LANG=POSIX", "LC_NUMERIC=de_DE.UTF-8"}, "real 1,25\nuser 0,50\nsys 0,25\n"},
		{"LC_ALL", []string{"LANG=POSIX", "LC_NUMERIC=POSIX", "LC_ALL=de_DE.iso88591"}, "real 1,25\nuser 0,50\nsys 0,25\n"},
		{"POSIX override", []string{"LANG=de_DE.iso88591", "LC_NUMERIC=de_DE.iso88591", "LC_ALL=POSIX"}, "real 1.25\nuser 0.50\nsys 0.25\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			radix, err := numericRadix(tc.env)
			if err != nil {
				t.Fatal(err)
			}
			got := report(opts{posix: true}, []string{"true"}, 1250*time.Millisecond, 500*time.Millisecond, 250*time.Millisecond, 0, false, 0, radix)
			if got != tc.want {
				t.Fatalf("report=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestTimeCustomFormatLocalizesOnlyNumericConversions(t *testing.T) {
	radix, err := numericRadix([]string{"LC_NUMERIC=de_DE.iso88591"})
	if err != nil {
		t.Fatal(err)
	}
	got := report(opts{format: "literal. %e %U %S %C"}, []string{"a.b"}, 1250*time.Millisecond, 500*time.Millisecond, 250*time.Millisecond, 0, false, 0, radix)
	if got != "literal. 1,25 0,50 0,25 a.b\n" {
		t.Fatalf("custom report=%q", got)
	}
}

func TestTimeAgenticBudgetTodo(t *testing.T) {
	// A zero budget guarantees "over budget" → the TODO fires.
	env := append(os.Environ(), "BASHY_AGENTIC=1")
	_, errOut, _ := runTime(t, env, "--budget", "0s", "--todo", "split this step", "true")
	if !strings.Contains(errOut, `"kind":"todo"`) || !strings.Contains(errOut, "split this step") {
		t.Errorf("expected an agent-mode TODO line, got %q", errOut)
	}
	// No budget ⇒ no TODO.
	_, errOut2, _ := runTime(t, os.Environ(), "true")
	if strings.Contains(errOut2, "todo") {
		t.Errorf("no budget should produce no TODO: %q", errOut2)
	}
}

// TestTimeOutputFileOpenError: an unopenable -o file is fatal (exit 1, path
// named), never a silent fallback to stderr.
func TestTimeOutputFileOpenError(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "no-such-dir", "out")
	_, errOut, code := runTime(t, os.Environ(), "-o", bad, "true")
	if code != 1 {
		t.Fatalf("time -o unopenable: exit=%d, want 1", code)
	}
	if !strings.Contains(errOut, bad) {
		t.Fatalf("stderr=%q, want the failing -o path named", errOut)
	}
}
