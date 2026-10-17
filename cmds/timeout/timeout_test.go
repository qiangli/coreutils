package timeoutcmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"5":    5 * time.Second,
		"5s":   5 * time.Second,
		"2m":   2 * time.Minute,
		"1h":   time.Hour,
		"1d":   24 * time.Hour,
		"0.5":  500 * time.Millisecond,
		"1.5s": 1500 * time.Millisecond,
		"0":    0,
	}
	for in, want := range cases {
		got, err := parseDuration(in)
		if err != nil || got != want {
			t.Errorf("parseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "5x", "s"} {
		if _, err := parseDuration(bad); err == nil {
			t.Errorf("parseDuration(%q) should error", bad)
		}
	}
}

func TestSignalByName(t *testing.T) {
	for _, name := range []string{"TERM", "SIGKILL", "int", "9"} {
		if signalByName(name) == nil {
			t.Errorf("signalByName(%q) = nil, want a signal", name)
		}
	}
	if signalByName("NOSUCH") != nil {
		t.Error("signalByName(NOSUCH) should be nil")
	}
}

func TestExitStatus(t *testing.T) {
	if got := exitStatus(nil, true, false); got != 124 {
		t.Errorf("timed-out exit = %d, want 124", got)
	}
	if got := exitStatus(nil, false, false); got != 0 {
		t.Errorf("clean exit = %d, want 0", got)
	}
}

func TestTimeoutHelpAndVersion(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}

	code := cmd.Run(rc, []string{"--help"})
	if code != 0 || !strings.Contains(out.String(), "Usage: timeout") {
		t.Errorf("--help: code=%d out=%q", code, out.String())
	}
	for _, want := range []string{"-f, --foreground", "-k, --kill-after", "-p, --preserve-status", "-s, --signal", "-v, --verbose", "-h, --help", "-V, --version"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--help missing %q in %q", want, out.String())
		}
	}

	out.Reset()
	code = cmd.Run(rc, []string{"--version"})
	if code != 0 || !strings.Contains(out.String(), "timeout") {
		t.Errorf("--version: code=%d out=%q", code, out.String())
	}

	out.Reset()
	code = cmd.Run(rc, []string{"-h"})
	if code != 0 || !strings.Contains(out.String(), "Usage: timeout") {
		t.Errorf("-h: code=%d out=%q", code, out.String())
	}

	out.Reset()
	code = cmd.Run(rc, []string{"-V"})
	if code != 0 || !strings.Contains(out.String(), "timeout") {
		t.Errorf("-V: code=%d out=%q", code, out.String())
	}
}

func TestTimeoutShortOptionSurface(t *testing.T) {
	// A no-op COMMAND that exists per platform: /bin/true has no windows
	// counterpart, so use cmd.exe there.
	env := []string{"PATH=/bin:/usr/bin"}
	noop := []string{"true"}
	if runtime.GOOS == "windows" {
		env = []string{"PATH=" + os.Getenv("PATH")}
		noop = []string{"cmd", "/c", "exit 0"}
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: t.TempDir(),
		Env: env,
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: &out,
			Err: &errb,
		},
	}
	code := cmd.Run(rc, append([]string{"-f", "-p", "-s", "TERM", "-k", "1s", "1s"}, noop...))
	if code != 0 || errb.String() != "" {
		t.Fatalf("short options: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

// An explicit extensionless path to a Windows executable must resolve through
// PATHEXT (as CreateProcess and env do), not fail with "No such file".
func TestTimeoutExplicitPathUsesPATHEXT(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PATHEXT resolution is Windows-only")
	}
	src, err := os.ReadFile(os.Getenv("SystemRoot") + `\System32\cmd.exe`)
	if err != nil {
		t.Skipf("no cmd.exe to copy: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(dir+`\prog.exe`, src, 0o755); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: dir,
		Env: []string{"PATH=" + os.Getenv("PATH"), "PATHEXT=.COM;.EXE;.BAT;.CMD"},
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: &out,
			Err: &errb,
		},
	}
	code := cmd.Run(rc, []string{"10", dir + `\prog`, "/c", "exit 0"})
	if code != 0 || errb.String() != "" {
		t.Fatalf("extensionless explicit path: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func waitForProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isPidAlive(pid) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return !isPidAlive(pid)
}

func TestHelperChildGrandchild(t *testing.T) {
	role := os.Getenv("TIMEOUT_TEST_HELPER_ROLE")
	if role == "" {
		return
	}
	switch role {
	case "child":
		childPidFile := os.Getenv("TIMEOUT_CHILD_PID_FILE")
		if childPidFile != "" {
			_ = os.WriteFile(childPidFile, []byte(strconv.Itoa(os.Getpid())), 0o644)
		}
		grandchildPidFile := os.Getenv("TIMEOUT_GRANDCHILD_PID_FILE")
		exe, err := os.Executable()
		if err != nil {
			exe = os.Args[0]
		}
		var childEnv []string
		for _, e := range os.Environ() {
			if !strings.HasPrefix(e, "TIMEOUT_TEST_HELPER_ROLE=") {
				childEnv = append(childEnv, e)
			}
		}
		cmd := exec.Command(exe, "-test.run=^TestHelperChildGrandchild$", "--")
		cmd.Env = append(childEnv, "TIMEOUT_TEST_HELPER_ROLE=grandchild")
		if err := cmd.Start(); err != nil {
			os.Exit(2)
		}
		// Wait until grandchild has written its PID file
		if grandchildPidFile != "" {
			for i := 0; i < 150; i++ {
				data, err := os.ReadFile(grandchildPidFile)
				if err == nil && len(strings.TrimSpace(string(data))) > 0 {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
		}
		time.Sleep(30 * time.Second)
		os.Exit(0)

	case "grandchild":
		grandchildPidFile := os.Getenv("TIMEOUT_GRANDCHILD_PID_FILE")
		if grandchildPidFile != "" {
			_ = os.WriteFile(grandchildPidFile, []byte(strconv.Itoa(os.Getpid())), 0o644)
		}
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
}

func TestTimeoutWindowsKillsChildTree(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only child tree kill test")
	}

	tempDir := t.TempDir()
	childPidFile := filepath.Join(tempDir, "child.pid")
	grandchildPidFile := filepath.Join(tempDir, "grandchild.pid")

	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}

	var out, errb bytes.Buffer
	env := append(os.Environ(),
		"TIMEOUT_TEST_HELPER_ROLE=child",
		"TIMEOUT_CHILD_PID_FILE="+childPidFile,
		"TIMEOUT_GRANDCHILD_PID_FILE="+grandchildPidFile,
	)
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   tempDir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}

	code := cmd.Run(rc, []string{"3s", exe, "-test.run=^TestHelperChildGrandchild$", "--"})
	if code != 124 {
		t.Fatalf("timeout exit code = %d, want 124; out=%q, err=%q", code, out.String(), errb.String())
	}

	childData, err := os.ReadFile(childPidFile)
	if err != nil {
		t.Fatalf("failed to read child PID file: %v", err)
	}
	childPid, err := strconv.Atoi(strings.TrimSpace(string(childData)))
	if err != nil {
		t.Fatalf("invalid child PID %q: %v", string(childData), err)
	}

	grandchildData, err := os.ReadFile(grandchildPidFile)
	if err != nil {
		t.Fatalf("failed to read grandchild PID file: %v", err)
	}
	grandchildPid, err := strconv.Atoi(strings.TrimSpace(string(grandchildData)))
	if err != nil {
		t.Fatalf("invalid grandchild PID %q: %v", string(grandchildData), err)
	}

	t.Cleanup(func() {
		killPid(childPid)
		killPid(grandchildPid)
	})

	if !waitForProcessExit(childPid, 5*time.Second) {
		t.Errorf("child process %d is still alive after timeout", childPid)
	}
	if !waitForProcessExit(grandchildPid, 5*time.Second) {
		t.Errorf("grandchild process %d is still alive after timeout", grandchildPid)
	}
}
