//go:build windows

package schedule

import (
	"io"
	"strings"
	"testing"
)

func TestWindowsCronExecutionFailsClosed(t *testing.T) {
	j := &Job{ID: "cron", POSIXCron: true, Command: []string{"cmd", "/c", "exit 0"}}
	if err := FireJob(j, io.Discard, nil); err == nil || !strings.Contains(err.Error(), "unsupported on Windows") {
		t.Fatalf("error=%v", err)
	}
}
