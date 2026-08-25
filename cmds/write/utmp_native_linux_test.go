//go:build linux

package writecmd

import (
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This is intentionally independent of encodeUtmp: the host C library creates
// a real struct utmp and reports its ABI. It catches the exact class of bug
// where a Go encoder and decoder agree with each other about the wrong layout.
func TestNativeLinuxUtmpABIAndFixture(t *testing.T) {
	if !platformSupported {
		t.Skipf("linux/%s is deliberately unsupported", runtime.GOARCH)
	}
	cc, err := osexec.LookPath("cc")
	if err != nil {
		t.Skip("native C ABI probe requires cc")
	}
	dir := t.TempDir()
	helper := filepath.Join(dir, "mkutmp")
	fixture := filepath.Join(dir, "utmp")
	build := osexec.Command(cc, filepath.Join("testdata", "mkutmp_linux.c"), "-o", helper)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compile native utmp fixture: %v\n%s", err, output)
	}
	probe := osexec.Command(helper, fixture)
	output, err := probe.CombinedOutput()
	if err != nil {
		t.Fatalf("create native utmp fixture: %v\n%s", err, output)
	}
	fields := strings.Fields(string(output))
	if len(fields) != 8 {
		t.Fatalf("native ABI output %q", output)
	}
	values := make([]int, len(fields))
	for i, field := range fields {
		value, err := strconv.Atoi(field)
		if err != nil {
			t.Fatalf("native ABI field %q: %v", field, err)
		}
		values[i] = value
	}
	want := []int{
		activeUtmpLayout.Size, activeUtmpLayout.UserOff,
		activeUtmpLayout.LineOff, activeUtmpLayout.HostOff,
		activeUtmpLayout.TypeOff, activeUtmpLayout.PIDOff,
		activeUtmpLayout.TimeOff, activeUtmpLayout.TimeLen,
	}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("linux/%s native ABI=%v selected layout=%v", runtime.GOARCH, values, want)
		}
	}

	f, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	records, err := decodeUtmp(f, activeUtmpLayout)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("decoded native records=%+v", records)
	}
	got := records[0]
	if got.User != "native-user" || got.Line != "pts/42" || got.Host != "native-host" ||
		got.PID != 4242 || !got.Time.Equal(time.Unix(1700000000, 0)) {
		t.Fatalf("decoded native record=%+v", got)
	}
}
