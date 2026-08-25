//go:build linux

package pscmd

import (
	"os"
	"strings"
	"testing"
)

func TestPSTTYNameDecodesLinuxDevptsMajorRange(t *testing.T) {
	dev := func(major, minor uint64) uint64 {
		return (major << 8) | (minor & 0xff) | ((minor & 0xfff00) << 12)
	}
	if got := ttyName(dev(136, 7)); got != "pts/7" {
		t.Fatalf("major 136 tty=%q", got)
	}
	if got := ttyName(dev(137, 7)); got != "pts/263" {
		t.Fatalf("major 137 tty=%q", got)
	}
	readFile := func(path string) ([]byte, error) {
		if path == "/sys/dev/char/188:0/uevent" {
			return []byte("MAJOR=188\nMINOR=0\nDEVNAME=ttyUSB0\n"), nil
		}
		return nil, os.ErrNotExist
	}
	if got := ttyNameWithReader(dev(188, 0), readFile); got != "ttyUSB0" {
		t.Fatalf("sysfs terminal name=%q", got)
	}
	if got := ttyNameWithReader(dev(189, 9), readFile); got != "" {
		t.Fatalf("unknown terminal name=%q, want unavailable", got)
	}
}

func TestPSEnrichLinuxCmdlinePreservesArgvZeroAndEmptyArguments(t *testing.T) {
	files := map[string][]byte{
		"/proc/stat":      []byte("btime 1700000000\n"),
		"/proc/self/auxv": auxvClockTicks(100),
		"/proc/8/stat":    []byte("8 (kernel fallback) S 1 2 3 0 0 0 0 0 0 0 0 0 0 0 20 0 1 0 100 4096 1 0 0"),
		"/proc/8/cmdline": []byte("custom-argv0\x00\x00tail\x00"),
	}
	readFile := func(path string) ([]byte, error) {
		if data, ok := files[path]; ok {
			return data, nil
		}
		return nil, os.ErrNotExist
	}
	p := process{pid: 8}
	if !enrichWithReader(&p, readFile) {
		t.Fatal("valid proc row was discarded")
	}
	if p.command != "custom-argv0" || p.args != "custom-argv0  tail" || !p.argvKnown {
		t.Fatalf("cmdline fields=%#v", p)
	}
	files["/proc/8/cmdline"] = []byte{'\x00'}
	p = process{pid: 8}
	if !enrichWithReader(&p, readFile) || p.command != "" || p.args != "" || !p.argvKnown {
		t.Fatalf("empty argv[0] not preserved: command=%q args=%q known=%v", p.command, p.args, p.argvKnown)
	}
	if strings.Contains(p.command, "kernel") {
		t.Fatal("empty argv[0] was replaced by kernel comm")
	}
}

func TestPSLinuxTimingRejectsOverflowAndEmptyWchan(t *testing.T) {
	files := map[string][]byte{
		"/proc/stat":      []byte("btime 1700000000\n"),
		"/proc/self/auxv": auxvClockTicks(100),
		"/proc/9/stat":    []byte("9 (p) S 1 2 3 0 0 0 0 0 0 0 18446744073709551615 1 0 0 20 0 1 0 18446744073709551615 4096 1 0 0"),
		"/proc/9/wchan":   []byte("\n"),
	}
	readFile := func(path string) ([]byte, error) {
		if data, ok := files[path]; ok {
			return data, nil
		}
		return nil, os.ErrNotExist
	}
	p := process{pid: 9}
	if !enrichWithReader(&p, readFile) {
		t.Fatal("valid proc row was discarded")
	}
	if p.cpuKnown || !p.start.IsZero() {
		t.Fatalf("overflowing clock-tick fields produced timing: %#v", p)
	}
	if p.wchanKnown || value(p, "wchan") != "-" {
		t.Fatalf("empty wait channel treated as a known running process: %#v", p)
	}
}
