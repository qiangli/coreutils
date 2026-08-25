//go:build unix

package schedule

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
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
