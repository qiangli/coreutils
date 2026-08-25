//go:build unix

package nicecmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

const priorityProbeEnv = "COREUTILS_NICE_PRIORITY_PROBE"
const ioProbeEnv = "COREUTILS_NICE_IO_PROBE"
const environmentProbeEnv = "COREUTILS_NICE_ENVIRONMENT_PROBE"
const statusProbeEnv = "COREUTILS_NICE_STATUS_PROBE"
const formerHelperEnv = "COREUTILS_NICE_PRIORITY_HELPER"

func init() {
	// The first re-exec is the internal priority helper and must not be
	// mistaken for the target: production init handles and replaces it before
	// test init runs. The eventual target arrives here after the overlay.
	if os.Getenv(priorityProbeEnv) == "1" {
		fmt.Fprintln(os.Stdout, currentPriority())
		os.Exit(0)
	}
	if os.Getenv(ioProbeEnv) == "1" {
		input, _ := io.ReadAll(os.Stdin)
		fmt.Fprintf(os.Stdout, "stdin=%s\nargs=%q\n", input, os.Args[1:])
		fmt.Fprintln(os.Stderr, "utility-stderr")
		os.Exit(0)
	}
	if os.Getenv(environmentProbeEnv) == "1" {
		for _, entry := range os.Environ() {
			if strings.HasPrefix(entry, formerHelperEnv+"=") {
				fmt.Fprintln(os.Stdout, entry)
			}
		}
		os.Exit(0)
	}
	if value := os.Getenv(statusProbeEnv); value != "" {
		code, _ := strconv.Atoi(value)
		os.Exit(code)
	}
}

func TestPriorityHelperWaitsForBarrierBeforeExec(t *testing.T) {
	oldExec := helperExec
	t.Cleanup(func() { helperExec = oldExec })

	var gotPath string
	var gotArgv, gotEnv []string
	helperExec = func(path string, argv, envv []string) error {
		gotPath = path
		gotArgv = append([]string(nil), argv...)
		gotEnv = append([]string(nil), envv...)
		return nil // a successful real exec does not return
	}

	var stderr bytes.Buffer
	code := runPriorityHelper(
		[]string{"nice", "/resolved/utility", "left", "-n"},
		[]string{"KEEP=yes", formerHelperEnv + "=user-value"},
		&stderr,
		strings.NewReader(priorityBarrier),
	)
	if code != 0 {
		t.Fatalf("helper code=%d stderr=%q", code, stderr.String())
	}
	if gotPath != "/resolved/utility" || strings.Join(gotArgv, "|") != "/resolved/utility|left|-n" {
		t.Fatalf("exec path=%q argv=%q", gotPath, gotArgv)
	}
	if strings.Join(gotEnv, "|") != "KEEP=yes|"+formerHelperEnv+"=user-value" {
		t.Fatalf("target environment=%q, invocation environment changed", gotEnv)
	}

	gotPath = ""
	stderr.Reset()
	if code := runPriorityHelper([]string{"nice", "/resolved/utility"}, nil, &stderr, bytes.NewReader(nil)); code != 125 {
		t.Fatalf("closed barrier code=%d stderr=%q", code, stderr.String())
	}
	if gotPath != "" {
		t.Fatalf("utility exec attempted before barrier release: %q", gotPath)
	}
}

func TestNicePreservesEnvironmentThatCollidesWithFormerHelperMarker(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: t.TempDir(),
		Env: []string{
			environmentProbeEnv + "=1",
			formerHelperEnv + "=user-supplied-exact-value",
		},
		Stdio: tool.Stdio{Out: &out, Err: &errb},
	}
	utility, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if code := run(rc, []string{"-n", "1", utility}); code != 0 {
		t.Fatalf("nice environment probe: code=%d stderr=%q", code, errb.String())
	}
	if got, want := out.String(), formerHelperEnv+"=user-supplied-exact-value\n"; got != want {
		t.Fatalf("utility environment=%q, want %q", got, want)
	}
}

func TestPriorityHelperPreservesENOEXECScriptFallback(t *testing.T) {
	oldExec := helperExec
	t.Cleanup(func() { helperExec = oldExec })
	var calls [][]string
	helperExec = func(path string, argv, _ []string) error {
		calls = append(calls, append([]string{path}, argv...))
		if len(calls) == 1 {
			return syscall.ENOEXEC
		}
		return nil
	}
	if code := runPriorityHelper([]string{"nice", "/tmp/script", "arg"}, nil, &bytes.Buffer{}, strings.NewReader(priorityBarrier)); code != 0 {
		t.Fatalf("helper code=%d", code)
	}
	if len(calls) != 2 || strings.Join(calls[1], "|") != "/bin/sh|/bin/sh|/tmp/script|arg" {
		t.Fatalf("exec calls=%q, want /bin/sh script fallback", calls)
	}
}

func TestNiceUtilityStartsAtAdjustedPriority(t *testing.T) {
	before := currentPriority()
	want := before + 5
	if want > nzero-1 {
		want = nzero - 1
	}

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   []string{priorityProbeEnv + "=1"},
		Stdio: tool.Stdio{Out: &out, Err: &errb},
	}
	utility, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if code := run(rc, []string{"-n", "5", utility}); code != 0 {
		t.Fatalf("nice priority probe: code=%d stderr=%q", code, errb.String())
	}
	got, err := strconv.Atoi(strings.TrimSpace(out.String()))
	if err != nil {
		t.Fatalf("priority probe output=%q: %v", out.String(), err)
	}
	if got != want {
		t.Fatalf("utility initial priority=%d, want %d (parent=%d)", got, want, before)
	}
	if after := currentPriority(); after != before {
		t.Fatalf("embedding process priority changed: before=%d after=%d", before, after)
	}
}

func TestNicePassesUtilityArgumentsAndStandardStreamsUnchanged(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: t.TempDir(),
		Env: []string{ioProbeEnv + "=1"},
		Stdio: tool.Stdio{
			In:  strings.NewReader("input payload"),
			Out: &out,
			Err: &errb,
		},
	}
	utility, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if code := run(rc, []string{"-n", "1", utility, "", "-n", "argument"}); code != 0 {
		t.Fatalf("nice I/O probe: code=%d stderr=%q", code, errb.String())
	}
	if got, want := out.String(), "stdin=input payload\nargs=[\"\" \"-n\" \"argument\"]\n"; got != want {
		t.Fatalf("utility stdout=%q, want %q", got, want)
	}
	if errb.String() != "utility-stderr\n" {
		t.Fatalf("utility stderr=%q", errb.String())
	}
}

func TestNiceAdjustmentFailureStillInvokesUtilityAndUtilityStatusWins(t *testing.T) {
	oldSetter := prioritySetter
	prioritySetter = func(int, int) error { return syscall.EPERM }
	t.Cleanup(func() { prioritySetter = oldSetter })

	before := currentPriority()
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   []string{statusProbeEnv + "=23"},
		Stdio: tool.Stdio{Err: &errb},
	}
	utility, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if code := run(rc, []string{"-n", "5", utility}); code != 23 {
		t.Fatalf("code=%d want utility status 23, stderr=%q", code, errb.String())
	}
	if !strings.Contains(errb.String(), "nice: cannot set niceness") {
		t.Fatalf("missing adjustment warning: %q", errb.String())
	}
	if after := currentPriority(); after != before {
		t.Fatalf("embedding process priority changed: before=%d after=%d", before, after)
	}
}

func TestPriorityHelperExecFailuresUsePOSIXStatuses(t *testing.T) {
	oldExec := helperExec
	t.Cleanup(func() { helperExec = oldExec })

	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"not found", syscall.ENOENT, 127},
		{"not invokable", syscall.EACCES, 126},
		{"wrapped not found", fmt.Errorf("exec: %w", syscall.ENOENT), 127},
		{"other", errors.New("boom"), 126},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helperExec = func(string, []string, []string) error { return tc.err }
			var stderr bytes.Buffer
			if got := runPriorityHelper([]string{"nice", "/utility"}, nil, &stderr, strings.NewReader(priorityBarrier)); got != tc.want {
				t.Fatalf("code=%d want=%d stderr=%q", got, tc.want, stderr.String())
			}
		})
	}
}

func TestNicePriorityHelperStartFailureUsesInternalErrorStatus(t *testing.T) {
	old := helperExecutable
	helperExecutable = func() (string, error) { return "", errors.New("unavailable") }
	t.Cleanup(func() { helperExecutable = old })

	var errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Err: &errb}}
	if code := runCommand(rc, "nice", []string{os.Args[0]}, currentPriority()+1); code != 125 {
		t.Fatalf("code=%d want=125 stderr=%q", code, errb.String())
	}
	if !strings.Contains(errb.String(), "failed to start priority helper") {
		t.Fatalf("stderr=%q", errb.String())
	}
}
