//go:build unix

package newgrpcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

const umaskProbeEnv = "COREUTILS_NEWGRP_UMASK_PROBE"
const shellIOProbeEnv = "COREUTILS_NEWGRP_SHELL_IO_PROBE"

func init() {
	// The internal helper is handled by production init before this test init.
	// After it overlays itself with the requested test-binary shell, this probe
	// reports the shell's initial mask without involving a host utility.
	if os.Getenv(umaskProbeEnv) == "1" {
		mask := syscall.Umask(0)
		syscall.Umask(mask)
		fmt.Fprintf(os.Stdout, "%03o\n", mask)
		os.Exit(0)
	}
	if os.Getenv(shellIOProbeEnv) == "1" {
		input, _ := io.ReadAll(os.Stdin)
		cwd, _ := os.Getwd()
		fmt.Fprintf(os.Stdout, "argv0=%s\ncwd=%s\ninput=%s\nenv=%s\n", os.Args[0], cwd, input, os.Getenv("KEEP"))
		fmt.Fprintln(os.Stderr, "shell-stderr")
		os.Exit(0)
	}
}

type stubShellProcess struct{ waitErr error }

func (p stubShellProcess) Wait() error { return p.waitErr }

func TestSyscallCredentialImplementsThePlan(t *testing.T) {
	plan := &credentialPlan{
		RealGID:       "20",
		EffectiveGID:  "20",
		Supplementary: []string{"12", "50"},
	}
	got, err := syscallCredential("1000", plan)
	if err != nil {
		t.Fatal(err)
	}
	if got.Uid != 1000 || got.Gid != 20 || got.NoSetGroups {
		t.Errorf("credential = %+v, want uid 1000, gid 20, and setgroups enabled", got)
	}
	if !slices.Equal(got.Groups, []uint32{12, 50}) {
		t.Errorf("groups = %v, want [12 50]", got.Groups)
	}
}

func TestSyscallCredentialRejectsAnUnrepresentableOrInvalidPlan(t *testing.T) {
	for _, tc := range []struct {
		name string
		uid  string
		plan credentialPlan
	}{
		{
			name: "distinct real and effective gids",
			uid:  "1000",
			plan: credentialPlan{RealGID: "20", EffectiveGID: "21"},
		},
		{
			name: "invalid uid",
			uid:  "not-a-uid",
			plan: credentialPlan{RealGID: "20", EffectiveGID: "20"},
		},
		{
			name: "invalid supplementary gid",
			uid:  "1000",
			plan: credentialPlan{RealGID: "20", EffectiveGID: "20", Supplementary: []string{"bad"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := syscallCredential(tc.uid, &tc.plan); err == nil {
				t.Fatal("invalid credential plan was accepted")
			}
		})
	}
}

func TestCredentialFailureClassificationDoesNotHideShellErrors(t *testing.T) {
	if !isCredentialStartFailure(&os.PathError{Op: "fork/exec", Path: "/bin/sh", Err: syscall.EPERM}) {
		t.Error("EPERM must be treated as a refused credential assignment")
	}
	for _, err := range []error{syscall.ENOENT, syscall.ENOEXEC, syscall.EACCES} {
		if isCredentialStartFailure(&os.PathError{Op: "fork/exec", Path: "/bin/sh", Err: err}) {
			t.Errorf("%v is a shell start error, not a credential failure", err)
		}
	}
}

func TestSupplementaryCapacityFallbackRetainsMandatoryGIDPlan(t *testing.T) {
	plan := &credentialPlan{
		RealGID:                        "20",
		EffectiveGID:                   "20",
		Supplementary:                  []string{"1000", "50", "20"},
		HasOptionalSupplementaryAppend: true,
	}
	var attempts []*syscall.Credential
	start := func(_ context.Context, _ *tool.RunContext, _ shellSpec, credential *syscall.Credential) (shellProcess, error) {
		copy := *credential
		copy.Groups = slices.Clone(credential.Groups)
		attempts = append(attempts, &copy)
		if len(attempts) == 1 {
			return nil, &os.PathError{Op: "fork/exec", Path: "/bin/sh", Err: syscall.EINVAL}
		}
		return stubShellProcess{}, nil
	}

	status, err := spawnShellWithStarter(&tool.RunContext{}, shellSpec{
		Path: "/bin/sh", Argv0: "sh", UID: "1000", Credential: plan,
	}, start)
	if err != nil || status != 0 {
		t.Fatalf("status = %d, error = %v; want successful capacity fallback", status, err)
	}
	if len(attempts) != 2 {
		t.Fatalf("attempts = %d, want initial launch and one fallback", len(attempts))
	}
	if attempts[0].Uid != 1000 || attempts[0].Gid != 20 ||
		!slices.Equal(attempts[0].Groups, []uint32{1000, 50, 20}) {
		t.Errorf("initial credential = %+v", attempts[0])
	}
	if attempts[1].Uid != 1000 || attempts[1].Gid != 20 || attempts[1].NoSetGroups ||
		!slices.Equal(attempts[1].Groups, []uint32{1000, 50}) {
		t.Errorf("fallback credential = %+v; want the same mandatory gid with only the optional append omitted", attempts[1])
	}
	if !slices.Equal(plan.Supplementary, []string{"1000", "50", "20"}) {
		t.Errorf("fallback mutated the plan: %v", plan.Supplementary)
	}
}

func TestSupplementaryCapacityFallbackIsNarrow(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		optional bool
		wantRuns int
	}{
		{name: "capacity error with optional append", err: syscall.EINVAL, optional: true, wantRuns: 2},
		{name: "permission error", err: syscall.EPERM, optional: true, wantRuns: 1},
		{name: "capacity error without optional append", err: syscall.EINVAL, wantRuns: 1},
		{name: "shell error", err: syscall.ENOENT, optional: true, wantRuns: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := &credentialPlan{
				RealGID:                        "20",
				EffectiveGID:                   "20",
				Supplementary:                  []string{"1000", "20"},
				HasOptionalSupplementaryAppend: tc.optional,
			}
			runs := 0
			start := func(_ context.Context, _ *tool.RunContext, _ shellSpec, _ *syscall.Credential) (shellProcess, error) {
				runs++
				return nil, &os.PathError{Op: "fork/exec", Path: "/bin/sh", Err: tc.err}
			}
			_, _ = spawnShellWithStarter(&tool.RunContext{}, shellSpec{
				Path: "/bin/sh", Argv0: "sh", UID: "1000", Credential: plan,
			}, start)
			if runs != tc.wantRuns {
				t.Errorf("launch attempts = %d, want %d", runs, tc.wantRuns)
			}
		})
	}
}

func TestPasswordPromptUsesStderrAndReadsControllingTTY(t *testing.T) {
	ttyPath := filepath.Join(t.TempDir(), "tty")
	if err := os.WriteFile(ttyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	oldOpen, oldRead := openPasswordTTY, readPasswordSecret
	readCalled := false
	openPasswordTTY = func() (*os.File, error) { return os.Open(ttyPath) }
	readPasswordSecret = func(fd int) ([]byte, error) {
		readCalled = true
		if fd < 0 {
			t.Fatalf("invalid tty fd %d", fd)
		}
		return []byte("s3cret"), nil
	}
	t.Cleanup(func() { openPasswordTTY, readPasswordSecret = oldOpen, oldRead })

	var stderr bytes.Buffer
	rc := &tool.RunContext{Stdio: tool.Stdio{Err: &stderr}}
	password, err := readPassword(rc, "Password: ")
	if err != nil {
		t.Fatal(err)
	}
	if password != "s3cret" || !readCalled {
		t.Errorf("password = %q, readCalled = %v", password, readCalled)
	}
	if stderr.String() != "Password: \n" {
		t.Errorf("stderr = %q, want prompt and trailing newline", stderr.String())
	}
	content, err := os.ReadFile(ttyPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 0 {
		t.Errorf("tty contains %q: prompt must be routed to stderr", content)
	}
}

func TestPasswordPromptDoesNotClaimATerminalOnOpenFailure(t *testing.T) {
	oldOpen := openPasswordTTY
	openPasswordTTY = func() (*os.File, error) { return nil, errors.New("no tty") }
	t.Cleanup(func() { openPasswordTTY = oldOpen })

	var stderr bytes.Buffer
	_, err := readPassword(&tool.RunContext{Stdio: tool.Stdio{Err: &stderr}}, "Password: ")
	if err == nil || !strings.Contains(err.Error(), "no terminal") {
		t.Fatalf("error = %v, want no-terminal failure", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, prompt must not be emitted before a tty is secured", stderr.String())
	}
}

func TestDefaultSpawnShellPropagatesExitStatus(t *testing.T) {
	path := writeExecutableScript(t, "exit 23\n")
	rc := &tool.RunContext{Stdio: tool.Stdio{In: strings.NewReader("")}}
	status, err := defaultSpawnShell(rc, shellSpec{Path: path, Argv0: "status-test"})
	if err != nil {
		t.Fatal(err)
	}
	if status != 23 {
		t.Errorf("status = %d, want 23", status)
	}
}

func TestDefaultSpawnShellPreservesArgumentsDirectoryEnvironmentAndIO(t *testing.T) {
	path, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Env: []string{shellIOProbeEnv + "=1", "KEEP=exported"},
		Stdio: tool.Stdio{
			In:  strings.NewReader("payload"),
			Out: &out,
			Err: &errb,
		},
	}
	status, err := defaultSpawnShell(rc, shellSpec{Path: path, Argv0: "chosen-argv0", Dir: dir})
	if err != nil || status != 0 {
		t.Fatalf("status = %d, error = %v, stderr = %q", status, err, errb.String())
	}
	physicalDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("argv0=chosen-argv0\ncwd=%s\ninput=payload\nenv=exported\n", physicalDir)
	if out.String() != want {
		t.Fatalf("shell stdout = %q, want %q", out.String(), want)
	}
	if errb.String() != "shell-stderr\n" {
		t.Fatalf("shell stderr = %q", errb.String())
	}
}

func TestDefaultSpawnShellAppliesRunContextVirtualUmask(t *testing.T) {
	parentMask := syscall.Umask(0)
	syscall.Umask(parentMask)
	want := 0o077
	if parentMask == want {
		want = 0o022
	}

	path, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Env:      []string{umaskProbeEnv + "=1"},
		UmaskSet: true,
		Umask:    os.FileMode(want),
		Stdio:    tool.Stdio{Out: &out, Err: &errb},
	}
	status, err := defaultSpawnShell(rc, shellSpec{Path: path, Argv0: "umask-probe"})
	if err != nil || status != 0 {
		t.Fatalf("status = %d, error = %v, stderr = %q", status, err, errb.String())
	}
	if got, expected := strings.TrimSpace(out.String()), fmt.Sprintf("%03o", want); got != expected {
		t.Fatalf("child umask = %q, want virtual mask %q (parent mask %03o)", got, expected, parentMask)
	}
	after := syscall.Umask(parentMask)
	syscall.Umask(after)
	if after != parentMask {
		t.Fatalf("embedding process umask changed: before %03o after %03o", parentMask, after)
	}
}

func TestRunUmaskHelperRequiresCompleteControlAndPreservesExecInputs(t *testing.T) {
	oldExec := umaskHelperExec
	oldMask := syscall.Umask(0)
	syscall.Umask(oldMask)
	t.Cleanup(func() {
		umaskHelperExec = oldExec
		syscall.Umask(oldMask)
	})

	var gotPath string
	var gotArgv, gotEnv []string
	umaskHelperExec = func(path string, argv, envv []string) error {
		gotPath = path
		gotArgv = slices.Clone(argv)
		gotEnv = slices.Clone(envv)
		return nil // a successful real exec does not return
	}

	env := []string{"KEEP=exact", "DUP=first", "DUP=last"}
	args := []string{"077", "/bin/chosen-shell", "-chosen-shell"}
	if code := runUmaskHelper(args, env, &bytes.Buffer{}, strings.NewReader(umaskHelperToken[:len(umaskHelperToken)-1])); code == 0 {
		t.Fatal("an incomplete control token was accepted")
	}
	if gotPath != "" {
		t.Fatalf("exec occurred before a complete valid token: %q", gotPath)
	}

	code := runUmaskHelper(args, env, &bytes.Buffer{}, strings.NewReader(umaskHelperToken))
	syscall.Umask(oldMask)
	if code != 0 {
		t.Fatalf("valid helper invocation returned %d", code)
	}
	if gotPath != "/bin/chosen-shell" || !slices.Equal(gotArgv, []string{"-chosen-shell"}) {
		t.Fatalf("exec path = %q argv = %q", gotPath, gotArgv)
	}
	if !slices.Equal(gotEnv, env) {
		t.Fatalf("exec environment = %q, want exact %q", gotEnv, env)
	}
}

func TestRunUmaskHelperRejectsInvalidControlOrMaskWithoutExec(t *testing.T) {
	oldExec := umaskHelperExec
	t.Cleanup(func() { umaskHelperExec = oldExec })
	for _, tc := range []struct {
		name    string
		args    []string
		control string
	}{
		{name: "wrong control", args: []string{"077", "/bin/sh", "sh"}, control: strings.Repeat("x", len(umaskHelperToken))},
		{name: "non-octal mask", args: []string{"xyz", "/bin/sh", "sh"}, control: umaskHelperToken},
		{name: "out-of-range mask", args: []string{"1000", "/bin/sh", "sh"}, control: umaskHelperToken},
		{name: "missing argument", args: []string{"077", "/bin/sh"}, control: umaskHelperToken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			execCalled := false
			umaskHelperExec = func(string, []string, []string) error {
				execCalled = true
				return nil
			}
			if code := runUmaskHelper(tc.args, nil, &bytes.Buffer{}, strings.NewReader(tc.control)); code == 0 {
				t.Fatal("invalid helper invocation returned success")
			}
			if execCalled {
				t.Fatal("invalid helper invocation reached exec")
			}
		})
	}
}

func TestDefaultSpawnShellPropagatesSignalStatus(t *testing.T) {
	path := writeExecutableScript(t, "kill -TERM $$\n")
	rc := &tool.RunContext{Stdio: tool.Stdio{In: strings.NewReader("")}}
	status, err := defaultSpawnShell(rc, shellSpec{Path: path, Argv0: "signal-test"})
	if err != nil {
		t.Fatal(err)
	}
	if status != 128+int(syscall.SIGTERM) || rc.ExitSignal != int(syscall.SIGTERM) {
		t.Errorf("status = %d, signal = %d; want %d and %d", status, rc.ExitSignal, 128+int(syscall.SIGTERM), syscall.SIGTERM)
	}
}

func writeExecutableScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "script")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
