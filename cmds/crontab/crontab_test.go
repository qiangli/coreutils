//go:build !windows

package crontabcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/tool"
)

func runTool(tb testing.TB, ctx context.Context, stdin string, args ...string) (string, string, int) {
	tb.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   tb.TempDir(),
		Env:   []string{"BASHY_SCHEDULE_STATE=" + os.Getenv("BASHY_SCHEDULE_STATE")},
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	cfg := runConfig{
		currentUser: func() (cronIdentity, error) { return cronIdentity{name: "cron-user", home: rc.Dir}, nil },
		euid:        func() int { return 0 },
		runEditor:   defaultRunConfig().runEditor,
	}
	code := runWithConfig(rc, args, cfg)
	return out.String(), errb.String(), code
}

func runCron(t *testing.T, ctx context.Context, stdin string, args ...string) (string, string, int) {
	return runTool(t, ctx, stdin, args...)
}

func runCronNoStdin(t *testing.T, ctx context.Context, args ...string) (string, string, int) {
	return runCron(t, ctx, "", args...)
}

func setupCronState(t *testing.T) string {
	t.Helper()
	p := t.TempDir() + "/schedule.json"
	t.Setenv("BASHY_SCHEDULE_STATE", p)
	return p
}

func TestCrontabHelp(t *testing.T) {
	out, _, code := runCronNoStdin(t, context.Background(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: crontab") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
}

func TestCrontabRelativeOperandFailsBeforeProcessCWDLookup(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Env: []string{"BASHY_SCHEDULE_STATE=" + filepath.Join(t.TempDir(), "schedule.json")},
		Stdio: tool.Stdio{
			In: strings.NewReader(""), Out: &out, Err: &errb,
		},
	}
	cfg := runConfig{
		currentUser: func() (cronIdentity, error) {
			return cronIdentity{name: "cron-user", home: t.TempDir()}, nil
		},
		euid:      func() int { return 0 },
		runEditor: defaultRunConfig().runEditor,
	}
	code := runWithConfig(rc, []string{"relative-table"}, cfg)
	if code != 2 || !strings.Contains(errb.String(), "invocation working directory") {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
	if strings.Contains(errb.String(), "cannot read") {
		t.Fatalf("relative operand consulted process cwd: %q", errb.String())
	}
}

func TestCrontabListEmpty(t *testing.T) {
	setupCronState(t)
	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("crontab -l: code=%d", code)
	}
	if out != "" {
		t.Errorf("expected empty output, got %q", out)
	}
}

func TestCrontabInstallListRemove(t *testing.T) {
	setupCronState(t)
	cronContent := "*/15 * * * * echo hello\n0 9 * * * date\n"

	// POSIX crontab is silent on success: stdout must be empty.
	out, _, code := runCron(t, context.Background(), cronContent)
	if code != 0 {
		t.Fatalf("crontab - (install from stdin): code=%d", code)
	}
	if out != "" {
		t.Errorf("stdout must be empty on successful install, got %q", out)
	}

	out, _, code = runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("crontab -l: code=%d", code)
	}
	if !strings.Contains(out, "echo hello") {
		t.Errorf("expected 'echo hello' in list, got %q", out)
	}
	if !strings.Contains(out, "date") {
		t.Errorf("expected 'date' in list, got %q", out)
	}

	_, _, code = runCronNoStdin(t, context.Background(), "-r")
	if code != 0 {
		t.Fatalf("crontab -r: code=%d", code)
	}

	out, _, code = runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("crontab -l after -r: code=%d", code)
	}
	if out != "" {
		t.Errorf("expected empty after -r, got %q", out)
	}
}

func TestCrontabRoundTrip(t *testing.T) {
	setupCronState(t)
	cronContent := "0 9 * * * echo hello world\n30 18 * * 1 date\n"

	_, _, code := runCron(t, context.Background(), cronContent)
	if code != 0 {
		t.Fatalf("install: code=%d", code)
	}

	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("list: code=%d", code)
	}

	_, _, code = runCronNoStdin(t, context.Background(), "-r")
	if code != 0 {
		t.Fatalf("remove: code=%d", code)
	}

	_, _, code = runCron(t, context.Background(), out)
	if code != 0 {
		t.Fatalf("reinstall from -l output: code=%d", code)
	}

	out2, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("list after round-trip: code=%d", code)
	}

	if out != out2 {
		t.Errorf("round-trip mismatch:\ninstall: %q\nlist after round-trip: %q", out, out2)
	}
}

func TestCrontabPersistsShellProgramAndContext(t *testing.T) {
	setupCronState(t)
	_, _, code := runCron(t, context.Background(), "* * * * * echo ok > result &\n")
	if code != 0 {
		t.Fatalf("install: code=%d", code)
	}
	jobs, err := schedule.LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("LoadJobs: jobs=%d err=%v", len(jobs), err)
	}
	if got, want := jobs[0].Command, []string{"/bin/sh", "-c", "echo ok > result &"}; !slices.Equal(got, want) {
		t.Errorf("job command=%q, want shell program %q", got, want)
	}
	if !jobs[0].EnvSet || !jobs[0].UmaskSet {
		t.Errorf("execution defaults absent: env_set=%v umask_set=%v", jobs[0].EnvSet, jobs[0].UmaskSet)
	}
	if jobs[0].Umask != 0o022 || !slices.Contains(jobs[0].Env, "PATH=/usr/bin:/bin") || !slices.Contains(jobs[0].Env, "SHELL=/bin/sh") {
		t.Errorf("execution defaults incorrect: env=%q umask=%04o", jobs[0].Env, jobs[0].Umask)
	}
}

func TestCrontabParseSkipsComments(t *testing.T) {
	setupCronState(t)
	cronContent := "# This is a comment\n0 9 * * * echo working\n# Another comment\n"

	_, _, code := runCron(t, context.Background(), cronContent)
	if code != 0 {
		t.Fatalf("install with comments: code=%d", code)
	}

	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("list: code=%d", code)
	}

	if out != cronContent {
		t.Errorf("comments were not preserved: got %q, want %q", out, cronContent)
	}
}

func TestCrontabReinstallReplaces(t *testing.T) {
	setupCronState(t)
	_, _, code := runCron(t, context.Background(), "0 9 * * * first\n")
	if code != 0 {
		t.Fatalf("first install: code=%d", code)
	}

	_, _, code = runCron(t, context.Background(), "30 18 * * * second\n")
	if code != 0 {
		t.Fatalf("second install: code=%d", code)
	}

	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("list: code=%d", code)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line after replace, got %d: %q", len(lines), out)
	}
	if !strings.Contains(out, "second") {
		t.Errorf("expected 'second', got %q", out)
	}
	if strings.Contains(out, "first") {
		t.Errorf("'first' should have been replaced, got %q", out)
	}
}

func TestCrontabBadLine(t *testing.T) {
	setupCronState(t)
	_, _, code := runCron(t, context.Background(), "0 9 * * * echo original\n")
	if code != 0 {
		t.Fatalf("seed install: code=%d", code)
	}
	cronContent := "0 9 * * * echo replacement\njust three fields\n"

	_, errb, code := runCron(t, context.Background(), cronContent)
	if code != 1 {
		t.Fatalf("install with bad line: code=%d, want 1", code)
	}
	if !strings.Contains(errb, "not enough fields") {
		t.Errorf("expected error about not enough fields, got %q", errb)
	}

	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("list: code=%d", code)
	}
	if out != "0 9 * * * echo original\n" {
		t.Errorf("invalid replacement changed existing table: %q", out)
	}
}

func TestCrontabInvalidScheduleIsAtomic(t *testing.T) {
	setupCronState(t)
	if _, _, code := runCron(t, context.Background(), "0 9 * * * echo original\n"); code != 0 {
		t.Fatalf("seed install: code=%d", code)
	}
	_, errb, code := runCron(t, context.Background(), "not-a-minute * * * * echo bad\n")
	if code != 1 || !strings.Contains(errb, "cannot compute next run") {
		t.Fatalf("invalid schedule = (stderr %q, code %d), want diagnostic and 1", errb, code)
	}
	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 || out != "0 9 * * * echo original\n" {
		t.Fatalf("invalid schedule changed table: out=%q code=%d", out, code)
	}
}

func TestCrontabRejectsConflictingModesAndExtraOperands(t *testing.T) {
	setupCronState(t)
	for _, tc := range []struct {
		stdin string
		args  []string
	}{
		{args: []string{"-l", "-r"}},
		{args: []string{"-l", "file"}},
		{args: []string{"-r", "file"}},
		{stdin: "0 9 * * * echo must-not-install\n", args: []string{"-", "extra"}},
	} {
		_, errb, code := runCron(t, context.Background(), tc.stdin, tc.args...)
		if code != 2 || errb == "" {
			t.Errorf("crontab %q = (stderr %q, code %d), want usage error", tc.args, errb, code)
		}
	}
	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 || out != "" {
		t.Fatalf("invalid invocation changed table: out=%q code=%d", out, code)
	}
}

func TestCrontabUserOptionFailsClosed(t *testing.T) {
	setupCronState(t)
	for _, args := range [][]string{
		{"-u", "someone-else", "-l"},
		{"--user=", "-l"},
	} {
		_, errb, code := runCronNoStdin(t, context.Background(), args...)
		if code != 2 || !strings.Contains(errb, "not supported") || !strings.Contains(errb, "-u") {
			t.Errorf("crontab %q = (stderr %q, code %d), want fail-closed contract error", args, errb, code)
		}
	}
}

// TestCrontabPreservesCommandInternalWhitespace proves the round-trip
// invariant documented at the top of crontab.go: what -l prints must be
// reinstallable byte-for-byte. A command field's internal whitespace runs
// (e.g. inside a quoted argument) are significant to the shell that later
// runs the command and must not be collapsed during parsing.
func TestCrontabPreservesCommandInternalWhitespace(t *testing.T) {
	setupCronState(t)
	cronContent := "0 9 * * * echo \"a   b\"\n"

	_, _, code := runCron(t, context.Background(), cronContent)
	if code != 0 {
		t.Fatalf("install: code=%d", code)
	}

	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("list: code=%d", code)
	}
	if out != cronContent {
		t.Errorf("command internal whitespace not preserved:\ninstalled: %q\nlisted:    %q", cronContent, out)
	}
}

func TestCrontabStdinReplace(t *testing.T) {
	setupCronState(t)
	cronContent := "0 9 * * * echo from stdin\n"

	_, _, code := runCron(t, context.Background(), cronContent)
	if code != 0 {
		t.Fatalf("install from stdin (no operands): code=%d", code)
	}

	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 {
		t.Fatalf("list: code=%d", code)
	}
	if !strings.Contains(out, "echo from stdin") {
		t.Errorf("expected 'echo from stdin', got %q", out)
	}
}

func TestCrontabDashOperandIsLiteralPathname(t *testing.T) {
	setupCronState(t)
	dir := t.TempDir()
	source := "0 9 * * * echo from-literal-dash\n"
	if err := os.WriteFile(filepath.Join(dir, "-"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	rc, out, errOut := cronTestContext(t, "0 9 * * * echo from-stdin\n")
	rc.Dir = dir
	if code := runWithConfig(rc, []string{"-"}, allowedConfig(dir)); code != 0 {
		t.Fatalf("install literal '-': code=%d stderr=%q", code, errOut)
	}
	if out.Len() != 0 {
		t.Fatalf("install stdout=%q, want empty", out)
	}
	listRC, listed, listErr := cronTestContext(t, "")
	if code := runWithConfig(listRC, []string{"-l"}, allowedConfig(listRC.Dir)); code != 0 || listed.String() != source {
		t.Fatalf("list code=%d stderr=%q source=%q, want %q", code, listErr, listed, source)
	}
}

// TestCrontabInstallSilent proves POSIX crontab is silent on success:
// neither stdin-replace, file-install, nor reinstall produce any stdout.
func TestCrontabInstallSilent(t *testing.T) {
	setupCronState(t)

	// stdin replace (crontab -)
	out, _, code := runCron(t, context.Background(), "0 9 * * * echo one\n")
	if code != 0 {
		t.Fatalf("crontab -: code=%d", code)
	}
	if out != "" {
		t.Errorf("crontab - stdout must be empty, got %q", out)
	}

	// stdin replace (no operands, reads stdin)
	out, _, code = runCron(t, context.Background(), "0 9 * * * echo two\n")
	if code != 0 {
		t.Fatalf("crontab (stdin): code=%d", code)
	}
	if out != "" {
		t.Errorf("crontab (stdin) stdout must be empty, got %q", out)
	}

	// reinstall (replacement) also silent
	out, _, code = runCron(t, context.Background(), "30 18 * * * echo three\n")
	if code != 0 {
		t.Fatalf("crontab reinstall: code=%d", code)
	}
	if out != "" {
		t.Errorf("crontab reinstall stdout must be empty, got %q", out)
	}
}

func TestAccessPolicyRequiresExactlyOneUsernamePerLine(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		want          bool
		wantErr       bool
	}{
		{"exact", "cron-user\nother\n", true, false},
		{"no final newline", "cron-user", true, false},
		{"comment suffix", "cron-user # comment\n", false, true},
		{"leading blank", " cron-user\n", false, true},
		{"trailing blank", "cron-user \n", false, true},
		{"two names", "cron-user other\n", false, true},
		{"comment line", "# cron-user\n", false, true},
		{"blank line", "cron-user\n\nother\n", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cron.allow")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := accessFileContains(path, "cron-user")
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("contains=(%v,%v), want (%v, err=%v)", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestMalformedAccessPolicyFailsClosedWithoutMutation(t *testing.T) {
	setupCronState(t)
	if _, _, code := runCron(t, context.Background(), "0 9 * * * original\n"); code != 0 {
		t.Fatal("seed failed")
	}
	accessDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(accessDir, "cron.allow"), []byte("cron-user # not a policy comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rc, _, errOut := cronTestContext(t, "0 9 * * * replacement\n")
	cfg := allowedConfig(rc.Dir)
	cfg.accessDirs = []string{accessDir}
	if code := runWithConfig(rc, nil, cfg); code != 1 || !strings.Contains(errOut.String(), "not authorized") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	out, _, code := runCronNoStdin(t, context.Background(), "-l")
	if code != 0 || out != "0 9 * * * original\n" {
		t.Fatalf("policy failure mutated table: %q", out)
	}
}

func TestCrontabPreservesWholeSourceAndEditorInputByteForByte(t *testing.T) {
	setupCronState(t)
	source := []byte("  # indented comment\n\n\t\n  0\t9  * * *   echo hi  %input  \n# final comment without newline")
	rc, _, errOut := cronTestContext(t, string(source))
	cfg := allowedConfig(rc.Dir)
	if code := runWithConfig(rc, nil, cfg); code != 0 {
		t.Fatalf("install code=%d stderr=%q", code, errOut)
	}
	listRC, listOut, listErr := cronTestContext(t, "")
	if code := runWithConfig(listRC, []string{"-l"}, allowedConfig(listRC.Dir)); code != 0 || !bytes.Equal(listOut.Bytes(), source) {
		t.Fatalf("list code=%d stderr=%q got=%q want=%q", code, listErr, listOut.Bytes(), source)
	}
	editRC, _, editErr := cronTestContext(t, "")
	editCfg := allowedConfig(editRC.Dir)
	editCfg.runEditor = func(_ *tool.RunContext, _ string, path string) error {
		got, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, source) {
			return fmt.Errorf("editor input=%q, want %q", got, source)
		}
		return nil
	}
	if code := runWithConfig(editRC, []string{"-e"}, editCfg); code != 0 {
		t.Fatalf("edit code=%d stderr=%q", code, editErr)
	}
}

func TestPercentCompilationAndExplicitShell(t *testing.T) {
	setupCronState(t)
	dir := t.TempDir()
	shell := filepath.Join(dir, "explicit-shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nexec /bin/sh \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	source := "SHELL=" + shell + "\n0 1 * * * cat - && printf \\%s done%alpha%beta\\%tail  \n"
	rc, _, errOut := cronTestContext(t, source)
	if code := runWithConfig(rc, nil, allowedConfig(rc.Dir)); code != 0 {
		t.Fatalf("install code=%d stderr=%q", code, errOut)
	}
	jobs, err := schedule.NewStore(os.Getenv("BASHY_SCHEDULE_STATE")).LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	j := jobs[0]
	if j.Command[0] != shell || j.Command[2] != "cat - && printf %s done" || j.Stdin != "alpha\nbeta%tail  \n" {
		t.Fatalf("compiled job=%+v", j)
	}
	if !slices.Contains(j.Env, "SHELL="+shell) {
		t.Fatalf("explicit SHELL absent from env: %q", j.Env)
	}
	t.Setenv("PATH", filepath.Join(dir, "deliberately-invalid"))
	var delivered []byte
	if err := schedule.FireJob(j, io.Discard, func(_ string, body []byte) error {
		delivered = append([]byte(nil), body...)
		return nil
	}); err != nil {
		t.Fatalf("execute with explicit SHELL: %v", err)
	}
	if got, want := string(delivered), "alpha\nbeta%tail  \ndone"; got != want {
		t.Fatalf("explicit SHELL output=%q, want %q", got, want)
	}
}

func TestPercentInputReachesScheduledCommand(t *testing.T) {
	setupCronState(t)
	dir := t.TempDir()
	helper := filepath.Join(dir, "consume-input")
	result := filepath.Join(dir, "result")
	program := "#!/bin/sh\nIFS= read -r first || exit 1\nIFS= read -r second || exit 1\nprintf '<%s>|<%s>\\n' \"$first\" \"$second\"\n"
	if err := os.WriteFile(helper, []byte(program), 0o700); err != nil {
		t.Fatal(err)
	}

	source := fmt.Sprintf("SHELL=/bin/sh\n0 1 * * * %s > %s%%left%%right\\%%tail\n", helper, result)
	rc, _, errOut := cronTestContext(t, source)
	if code := runWithConfig(rc, nil, allowedConfig(rc.Dir)); code != 0 {
		t.Fatalf("install code=%d stderr=%q", code, errOut)
	}
	jobs, err := schedule.NewStore(os.Getenv("BASHY_SCHEDULE_STATE")).LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
	if err := schedule.FireJob(jobs[0], io.Discard, nil); err != nil {
		t.Fatalf("execute scheduled command: %v", err)
	}
	got, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	if want := "<left>|<right%tail>\n"; string(got) != want {
		t.Fatalf("scheduled stdin output=%q, want %q", got, want)
	}
}

func TestCronExecutionUsesAbsoluteShellAndDefaultPATH(t *testing.T) {
	setupCronState(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "made", "result")
	rc, _, errOut := cronTestContext(t, "* * * * * mkdir -p made && printf ok > made/result\n")
	rc.Dir = dir
	if code := runWithConfig(rc, nil, allowedConfig(dir)); code != 0 {
		t.Fatalf("install code=%d stderr=%q", code, errOut)
	}
	jobs, err := schedule.StoreFor(rc.Dir, rc.Env).LoadJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("load installed job: jobs=%v err=%v", jobs, err)
	}
	j := jobs[0]
	if got, want := j.Command, []string{"/bin/sh", "-c", "mkdir -p made && printf ok > made/result"}; !slices.Equal(got, want) {
		t.Fatalf("installed command=%q, want %q", got, want)
	}
	if !slices.Contains(j.Env, "PATH=/usr/bin:/bin") || !slices.Contains(j.Env, "SHELL=/bin/sh") {
		t.Fatalf("installed defaults=%q", j.Env)
	}
	t.Setenv("PATH", filepath.Join(dir, "deliberately-invalid"))
	var mail []byte
	if err := schedule.FireJob(j, io.Discard, func(_ string, body []byte) error { mail = append([]byte(nil), body...); return nil }); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "ok" {
		t.Fatalf("required PATH utility did not run: content=%q err=%v mail=%q", got, err, mail)
	}
}

func TestRunContextSelectsIsolatedStoresAndPreservesAtJobs(t *testing.T) {
	root := t.TempDir()
	stateA, stateB := filepath.Join(root, "a.json"), filepath.Join(root, "b.json")
	atJob := &schedule.Job{ID: "at-keep", Kind: "at", Enabled: true}
	if err := schedule.NewStore(stateA).SaveJobs([]*schedule.Job{atJob}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ state, source string }{{stateA, "0 1 * * * one\n"}, {stateB, "0 2 * * * two\n"}} {
		rc, _, errOut := cronTestContext(t, tc.source)
		rc.Env = []string{"BASHY_SCHEDULE_STATE=" + tc.state}
		if code := runWithConfig(rc, nil, allowedConfig(rc.Dir)); code != 0 {
			t.Fatalf("install %s: code=%d stderr=%q", tc.state, code, errOut)
		}
	}
	jobsA, _ := schedule.NewStore(stateA).LoadJobs()
	jobsB, _ := schedule.NewStore(stateB).LoadJobs()
	if len(jobsA) != 2 || jobsA[0].ID != "at-keep" || len(jobsB) != 1 || jobsB[0].Command[2] != "two" {
		t.Fatalf("isolated jobs: A=%+v B=%+v", jobsA, jobsB)
	}
	storeA := schedule.NewStore(stateA)
	if err := storeA.RemoveCron(); err != nil {
		t.Fatal(err)
	}
	remaining, _ := storeA.LoadJobs()
	if len(remaining) != 1 || remaining[0].ID != "at-keep" {
		t.Fatalf("cron removal changed at jobs: %+v", remaining)
	}
}

func cronTestContext(t *testing.T, stdin string) (*tool.RunContext, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Env: []string{"BASHY_SCHEDULE_STATE=" + os.Getenv("BASHY_SCHEDULE_STATE"), "EDITOR=unused"}, Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errOut}}
	return rc, &out, &errOut
}

func allowedConfig(home string) runConfig {
	return runConfig{currentUser: func() (cronIdentity, error) { return cronIdentity{name: "cron-user", home: home}, nil }, euid: func() int { return 0 }, runEditor: defaultRunConfig().runEditor}
}
