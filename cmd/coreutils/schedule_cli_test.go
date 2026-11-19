package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestScheduleCLIProcess(t *testing.T) {
	if os.Getenv("COREUTILS_SCHEDULE_CLI_TEST") != "1" {
		return
	}
	args := strings.Split(os.Getenv("COREUTILS_SCHEDULE_CLI_ARGS"), "\n")
	argv0 := os.Getenv("COREUTILS_SCHEDULE_CLI_ARGV0")
	if argv0 == "" {
		argv0 = "coreutils"
	}
	os.Args = append([]string{argv0}, args...)
	main()
	os.Exit(0)
}

func TestScheduleCLI(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
		code int
	}{
		{"help", []string{"schedule", "--help"}, "daemon", 0},
		{"daemon-help", []string{"schedule", "daemon", "--help"}, "--interval", 0},
		{"unknown-command", []string{"schedule", "not-a-command"}, "unknown command", 1},
		{"unknown-flag", []string{"schedule", "daemon", "--not-a-flag"}, "unknown flag", 1},
		{"invalid-interval", []string{"schedule", "daemon", "--interval", "invalid"}, "invalid", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := exec.Command(os.Args[0], "-test.run=^TestScheduleCLIProcess$")
			c.Env = append(os.Environ(), "COREUTILS_SCHEDULE_CLI_TEST=1", "COREUTILS_SCHEDULE_CLI_ARGS="+strings.Join(tc.args, "\n"), "BASHY_SCHEDULE_STATE="+t.TempDir()+"/state.json")
			out, err := c.CombinedOutput()
			code := 0
			if err != nil {
				if exit, ok := err.(*exec.ExitError); ok {
					code = exit.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			if code != tc.code || !strings.Contains(string(out), tc.want) {
				t.Fatalf("exit=%d want=%d output=%q want substring=%q", code, tc.code, out, tc.want)
			}
		})
	}
}

func TestScheduleOperandDoesNotHijackApplet(t *testing.T) {
	c := exec.Command(os.Args[0], "-test.run=^TestScheduleCLIProcess$")
	c.Env = append(os.Environ(), "COREUTILS_SCHEDULE_CLI_TEST=1", "COREUTILS_SCHEDULE_CLI_ARGV0=printf", "COREUTILS_SCHEDULE_CLI_ARGS=schedule")
	out, err := c.CombinedOutput()
	if err != nil || string(out) != "schedule" {
		t.Fatalf("printf schedule: error=%v output=%q", err, out)
	}
}
