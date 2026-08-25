//go:build unix

package schedule

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/creack/pty/v2"
	"golang.org/x/sys/unix"
)

func TestJobRunsInSeparateProcessGroup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pgid")
	j := &Job{ID: "pg", Kind: "at", Command: []string{"/bin/sh", "-c", "ps -o pgid= -p $$ > " + path}}
	if err := FireJob(j, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pgid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	parentText, parentErr := exec.Command("ps", "-o", "pgid=", "-p", strconv.Itoa(os.Getpid())).Output()
	parentPGID, parseParentErr := strconv.Atoi(strings.TrimSpace(string(parentText)))
	if err != nil || parentErr != nil || parseParentErr != nil || pgid == parentPGID {
		t.Fatalf("job pgid=%d parent pgid=%d errors=(%v,%v,%v)", pgid, parentPGID, err, parentErr, parseParentErr)
	}
}

func TestJobNoControllingTerminalHelper(t *testing.T) {
	if os.Getenv("BASHY_AT_SESSION_HELPER") != "1" {
		return
	}
	// pty.Start gives this helper a controlling terminal. If that setup did
	// not happen, the outer test would otherwise produce a false positive.
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		os.Exit(3)
	}
	_ = tty.Close()
	record := os.Getenv("BASHY_AT_SESSION_RECORD")
	j := &Job{
		ID: "session", Kind: "at",
		Command: []string{os.Args[0], "-test.run=^TestJobSessionChild$"},
		Env: append(os.Environ(),
			"BASHY_AT_SESSION_CHILD=1",
			"BASHY_AT_SESSION_RECORD="+record,
		),
		EnvSet: true,
	}
	if err := FireJob(j, io.Discard, nil); err != nil {
		os.Exit(4)
	}
	os.Exit(0)
}

func TestJobSessionChild(t *testing.T) {
	if os.Getenv("BASHY_AT_SESSION_CHILD") != "1" {
		return
	}
	pid := os.Getpid()
	sid, err := unix.Getsid(0)
	if err != nil {
		os.Exit(5)
	}
	pgid := unix.Getpgrp()
	hasTTY := "no"
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		hasTTY = "yes"
		_ = tty.Close()
	}
	record := fmt.Sprintf("%d %d %d %s\n", pid, sid, pgid, hasTTY)
	if err := os.WriteFile(os.Getenv("BASHY_AT_SESSION_RECORD"), []byte(record), 0o600); err != nil {
		os.Exit(6)
	}
	os.Exit(0)
}

func TestJobStartsNewSessionWithoutControllingTerminal(t *testing.T) {
	record := filepath.Join(t.TempDir(), "session")
	cmd := exec.Command(os.Args[0], "-test.run=^TestJobNoControllingTerminalHelper$")
	cmd.Env = append(os.Environ(), "BASHY_AT_SESSION_HELPER=1", "BASHY_AT_SESSION_RECORD="+record)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Skipf("pty.Start: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		_ = ptmx.Close()
		t.Fatalf("session helper: %v", err)
	}
	_ = ptmx.Close()
	b, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(b))
	if len(fields) != 4 || fields[0] != fields[1] || fields[0] != fields[2] || fields[3] != "no" {
		t.Fatalf("child process/session/terminal record=%q, want pid=sid=pgid and no tty", b)
	}
}
