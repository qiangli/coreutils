//go:build unix

package envcmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

const (
	envLongCWDMode      = "COREUTILS_ENV_LONG_CWD_MODE"
	envLongCWDDepth     = "COREUTILS_ENV_LONG_CWD_DEPTH"
	envLongCWDComponent = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	envLongCWDMarker    = ".env-pathmax-marker"
)

// TestEnvDedicatedProcessExecFromLongCWD covers the standalone env(1)
// boundary used by POSIX sh tests: COMMAND starts with PWD removed while the
// inherited current directory has an absolute spelling longer than PATH_MAX.
// That cwd is valid because it was entered component by component. A dedicated
// env must exec COMMAND in place without trying to resolve the overlong name
// again.
func TestEnvDedicatedProcessExecFromLongCWD(t *testing.T) {
	switch os.Getenv(envLongCWDMode) {
	case "setup":
		runEnvLongCWDSetup()
		return
	case "target":
		runEnvLongCWDTarget()
		return
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	child := exec.Command(exe, "-test.run=^TestEnvDedicatedProcessExecFromLongCWD$")
	child.Dir = root
	child.Env = append(os.Environ(), envLongCWDMode+"=setup")
	out, err := child.CombinedOutput()
	if err != nil {
		t.Fatalf("env exec from cwd beyond PATH_MAX: %v\n%s", err, out)
	}
	if got, want := string(out), "exec-from-long-cwd=ok\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func runEnvLongCWDSetup() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	base, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	logical := base
	depth := 0
	for len(logical) <= unix.PathMax+len(envLongCWDComponent) {
		if err := os.Mkdir(envLongCWDComponent, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if err := os.Chdir(envLongCWDComponent); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		logical = filepath.Join(logical, envLongCWDComponent)
		depth++
	}
	if len(logical) <= unix.PathMax {
		fmt.Fprintf(os.Stderr, "setup cwd length %d <= PATH_MAX %d\n", len(logical), unix.PathMax)
		cleanupEnvLongCWD(depth)
		os.Exit(2)
	}
	if err := os.WriteFile(envLongCWDMarker, []byte("present"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		cleanupEnvLongCWD(depth)
		os.Exit(2)
	}

	rc := &tool.RunContext{
		Ctx:              context.Background(),
		Dir:              logical,
		DirIsProcessCwd:  true,
		DedicatedProcess: true,
		Env:              os.Environ(),
		FS:               tool.NewLocalFS(),
		Stdio: tool.Stdio{
			In:  os.Stdin,
			Out: os.Stdout,
			Err: os.Stderr,
		},
	}
	args := []string{
		"-u", "PWD",
		envLongCWDMode + "=target",
		envLongCWDDepth + "=" + strconv.Itoa(depth),
		exe, "-test.run=^TestEnvDedicatedProcessExecFromLongCWD$",
	}
	code := cmd.Run(rc, args)
	cleanupEnvLongCWD(depth)
	os.Exit(code)
}

func runEnvLongCWDTarget() {
	if _, present := os.LookupEnv("PWD"); present {
		fmt.Fprintln(os.Stderr, "PWD unexpectedly survived env -u")
		os.Exit(3)
	}
	data, err := os.ReadFile(envLongCWDMarker)
	if err != nil || string(data) != "present" {
		fmt.Fprintf(os.Stderr, "COMMAND did not inherit long cwd: data=%q err=%v\n", data, err)
		os.Exit(4)
	}
	depth, err := strconv.Atoi(os.Getenv(envLongCWDDepth))
	if err != nil || depth < 1 {
		fmt.Fprintf(os.Stderr, "invalid cleanup depth: %q\n", os.Getenv(envLongCWDDepth))
		os.Exit(5)
	}
	if err := os.Remove(envLongCWDMarker); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(6)
	}
	cleanupEnvLongCWD(depth)
	fmt.Println("exec-from-long-cwd=ok")
	os.Exit(0)
}

func cleanupEnvLongCWD(depth int) {
	for range depth {
		if err := os.Chdir(".."); err != nil {
			return
		}
		_ = os.Remove(envLongCWDComponent)
	}
}
