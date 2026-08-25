//go:build !unix

package schedule

import (
	"io"
	"strings"
	"testing"
)

func TestNonUnixCronExecutionFailsClosed(t *testing.T) {
	j := &Job{ID: "cron", POSIXCron: true, Command: []string{"cmd", "/c", "exit 0"}}
	if err := FireJob(j, io.Discard, nil); err == nil || !strings.Contains(err.Error(), "unsupported on this non-Unix host") {
		t.Fatalf("error=%v", err)
	}
}

func TestNonUnixAtAndBatchExecutionFailsClosed(t *testing.T) {
	for _, queue := range []string{"a", "b"} {
		j := &Job{ID: "at-" + queue, Kind: "at", Queue: queue, Command: []string{"cmd", "/c", "exit 0"}, Umask: 0o027, UmaskSet: true}
		err := FireJob(j, io.Discard, nil)
		if err == nil || !strings.Contains(err.Error(), "session, process-group, controlling-terminal, and umask semantics") {
			t.Fatalf("queue %s error=%v", queue, err)
		}
	}
}
