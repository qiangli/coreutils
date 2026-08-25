//go:build linux || darwin

package whocmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// platformABIRecord is a behavioral fixture derived from platform libc
// offsetof/sizeof measurements, independent of the session parser. It drives
// who through session.Read, so incorrect stride or offsets fail at command
// level rather than merely exercising a decoder helper.
func platformABIRecord(typ int16, pid int32, user, id, line string, sec int64) []byte {
	if runtime.GOOS == "darwin" {
		rec := make([]byte, 628)
		copy(rec[0:256], user)
		copy(rec[256:260], id)
		copy(rec[260:292], line)
		binary.LittleEndian.PutUint32(rec[292:296], uint32(pid))
		binary.LittleEndian.PutUint16(rec[296:298], uint16(typ))
		binary.LittleEndian.PutUint32(rec[300:304], uint32(sec))
		return rec
	}

	size, secOff, secSize := 384, 340, 4
	var order binary.ByteOrder = binary.LittleEndian
	switch runtime.GOARCH {
	case "arm64", "riscv64", "loong64", "mips64", "mips64le":
		size, secOff, secSize = 400, 344, 8
	}
	switch runtime.GOARCH {
	case "s390x", "ppc", "ppc64", "mips", "mips64":
		order = binary.BigEndian
	}
	rec := make([]byte, size)
	order.PutUint16(rec[0:2], uint16(typ))
	order.PutUint32(rec[4:8], uint32(pid))
	copy(rec[8:40], line)
	copy(rec[40:44], id)
	copy(rec[44:76], user)
	if secSize == 8 {
		order.PutUint64(rec[secOff:secOff+8], uint64(sec))
	} else {
		order.PutUint32(rec[secOff:secOff+4], uint32(sec))
	}
	return rec
}

func TestWhoBinaryABIBehavior(t *testing.T) {
	data := append(platformABIRecord(7, 4242, "alice", "p/0", "pts/0", 1700000000),
		platformABIRecord(1, int32('S')+int32('5')*256, "runlevel", "~~", "~", 1700000001)...)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	invoke := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"TZ=UTC"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
		return run(rc, append(args, filepath.Join(dir, "utmp"))), out.String(), errb.String()
	}
	if code, out, errout := invoke("-q"); code != 0 || out != "alice\n# users=1\n" {
		t.Fatalf("binary -q: code=%d out=%q err=%q", code, out, errout)
	}
	if code, out, errout := invoke("-r"); code != 0 || !strings.Contains(out, "run-level S") || !strings.Contains(out, "last=5") {
		t.Fatalf("binary -r: code=%d out=%q err=%q", code, out, errout)
	}
}
