//go:build !windows

package atcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/tool"
)

func TestAtSubmissionContextSurvivesDaemonExecution(t *testing.T) {
	state := setupATState(t)
	t.Setenv("DAEMON_ONLY", "must-not-leak")
	dir := t.TempDir()
	program := `printf '%s\n' 'hello world' |
sed 's/world/mesh/' > result
printf '%s\n' "$MBI_ENV" >> result
pwd > cwd
umask > mask
env | sort > environment
: > created
`
	env := []string{
		"PATH=/usr/bin:/bin",
		"SHELL=sh",
		"MBI_ENV=garp",
		"PWD=" + dir,
		"BASHY_SCHEDULE_STATE=" + state,
	}
	var stdout, stderr bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir, Env: env,
		Umask: 0o027, UmaskSet: true,
		Stdio: tool.Stdio{In: strings.NewReader(program), Out: &stdout, Err: &stderr},
	}
	if code := cmd.Run(rc, []string{"now"}); code != 0 {
		t.Fatalf("at now: code=%d stderr=%q", code, stderr.String())
	}

	jobs, err := schedule.LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("load submitted job: jobs=%v err=%v", jobs, err)
	}
	// Make the job immediately due; the daemon loop below is the same path used
	// in production, without adding a one-second wall-clock delay to this test.
	jobs[0].NextRun = time.Now().Add(-time.Second)
	if err := schedule.SaveJobs(jobs); err != nil {
		t.Fatal(err)
	}

	daemonCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	daemon := schedule.NewScheduleCmd()
	daemon.SetContext(daemonCtx)
	daemon.SetArgs([]string{"daemon", "--interval", "5ms"})
	daemon.SetOut(&bytes.Buffer{})
	daemon.SetErr(&bytes.Buffer{})
	done := make(chan error, 1)
	go func() { done <- daemon.Execute() }()

	completionPath := filepath.Join(dir, "created")
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(completionPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatal("daemon did not execute due at job")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("daemon: %v", err)
	}

	assertFile := func(name, want string) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s=%q err=%v, want %q", name, got, err, want)
		}
	}
	assertFile("result", "hello mesh\ngarp\n")
	assertFile("cwd", dir+"\n")
	mask, err := os.ReadFile(filepath.Join(dir, "mask"))
	if err != nil || strings.TrimLeft(strings.TrimSpace(string(mask)), "0") != "27" {
		t.Fatalf("mask=%q err=%v, want 0027", mask, err)
	}
	info, err := os.Stat(filepath.Join(dir, "created"))
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("created mode=%v err=%v, want 0640", modeOrZero(info), err)
	}

	envBytes, err := os.ReadFile(filepath.Join(dir, "environment"))
	if err != nil {
		t.Fatal(err)
	}
	var stable []string
	for _, line := range strings.Split(strings.TrimSpace(string(envBytes)), "\n") {
		if strings.HasPrefix(line, "SHLVL=") || strings.HasPrefix(line, "_=") {
			continue
		}
		stable = append(stable, line)
	}
	wantEnv := append([]string(nil), env...)
	sort.Strings(wantEnv)
	if !reflect.DeepEqual(stable, wantEnv) {
		t.Fatalf("executed env=%s, want %s", strings.Join(stable, "\n"), strings.Join(wantEnv, "\n"))
	}
}

func modeOrZero(info os.FileInfo) os.FileMode {
	if info == nil {
		return 0
	}
	return info.Mode().Perm()
}
