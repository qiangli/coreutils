package paxcmd

import (
	"archive/tar"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func makeTimestampBoundaryArchive(t *testing.T, now time.Time) []byte {
	t.Helper()
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	members := []struct {
		name  string
		mtime time.Time
	}{
		{"now", now},
		{"recent-edge", now.Add(-paxSixMonths + time.Second)},
		{"six-month-boundary", now.Add(-paxSixMonths)},
		{"old", now.Add(-paxSixMonths - time.Second)},
		{"future", now.Add(time.Second)},
	}
	for _, member := range members {
		h := &tar.Header{
			Name: member.name, Mode: 0o644, ModTime: member.mtime,
			Typeflag: tar.TypeReg, Format: tar.FormatPAX,
			Uname: "user", Gname: "group",
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func TestPAXVerboseTimestampAgeBoundaries(t *testing.T) {
	now := time.Date(2026, time.August, 26, 4, 30, 0, 0, time.UTC)
	archive := makeTimestampBoundaryArchive(t, now)
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Env:   []string{"LC_TIME=C", "TZ=UTC"},
		Stdio: tool.Stdio{Out: &out, Err: &errOut},
	}
	clockCalls := 0
	o := &options{verbose: true, now: func() time.Time {
		clockCalls++
		return now
	}}
	opener := func(*tool.RunContext, *options) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(archive)), nil
	}
	if code := listModeWithOpener(rc, o, nil, opener); code != 0 || errOut.Len() != 0 {
		t.Fatalf("list = (code %d, stderr %q), want success", code, errOut.String())
	}
	if clockCalls != 1 {
		t.Fatalf("clock called %d times, want one invocation snapshot", clockCalls)
	}

	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("got %d lines, want 5:\n%s", len(lines), out.String())
	}
	mtimes := []time.Time{
		now,
		now.Add(-paxSixMonths + time.Second),
		now.Add(-paxSixMonths),
		now.Add(-paxSixMonths - time.Second),
		now.Add(time.Second),
	}
	names := []string{"now", "recent-edge", "six-month-boundary", "old", "future"}
	for i := range lines {
		layout := "Jan _2  2006"
		if i < 2 {
			layout = "Jan _2 15:04"
		}
		wantSuffix := mtimes[i].Format(layout) + " " + names[i]
		if !strings.HasSuffix(lines[i], wantSuffix) {
			t.Errorf("line %d = %q, want suffix %q", i, lines[i], wantSuffix)
		}
	}
}
