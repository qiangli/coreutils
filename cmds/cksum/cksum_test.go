package cksumcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestCKSumStdinAndFiles(t *testing.T) {
	out, _, code := runTool(t, "", "abc")
	if out != "1219131554 3\n" || code != 0 {
		t.Fatalf("stdin = (%q, %d)", out, code)
	}
	out, _, code = runTool(t, "", "")
	if out != "4294967295 0\n" || code != 0 {
		t.Fatalf("empty stdin = (%q, %d)", out, code)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code = runTool(t, dir, "", "a.txt")
	if out != "1219131554 3 a.txt\n" || code != 0 {
		t.Fatalf("file = (%q, %d)", out, code)
	}
}

func TestCKSumAlgorithms(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--algorithm=bsd"}, "16556     1\n"},
		{[]string{"--algorithm=sysv"}, "294 1\n"},
		{[]string{"--algorithm=sha1"}, "SHA1 (-) = a9993e364706816aba3e25717850c26c9cd0d89d\n"},
		{[]string{"--algorithm=sha1", "--untagged"}, "a9993e364706816aba3e25717850c26c9cd0d89d  -\n"},
		{[]string{"--algorithm=sha1", "--base64"}, "SHA1 (-) = qZk+NkcGgWq6PiVxeFDCbJzQ2J0=\n"},
		{[]string{"--algorithm=md5"}, "MD5 (-) = 900150983cd24fb0d6963f7d28e17f72\n"},
		{[]string{"--algorithm=crc32b"}, "CRC32B (-) = 352441c2\n"},
		{[]string{"--algorithm=sha3"}, "SHA3-256 (-) = 3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532\n"},
		{[]string{"--algorithm=sha3-512"}, "SHA3-512 (-) = b751850b1a57168a5693cd924b6b096e08f621827444f70d884f5d0240d2712e10e116e9192af3c91a7ec57647e3934057340b4cf408d5a56592f8274eec53f0\n"},
		{[]string{"--algorithm=sm3"}, "SM3 (-) = 66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0\n"},
		{[]string{"--algorithm=blake3"}, "BLAKE3-256 (-) = 6437b3ac38465133ffb63b75273a8db548c558465d79db03fd359c6cd5bd9d85\n"},
		{[]string{"--algorithm=blake3", "--length=512"}, "BLAKE3-512 (-) = 6437b3ac38465133ffb63b75273a8db548c558465d79db03fd359c6cd5bd9d851fb250ae7393f5d02813b65d521a0d492d9ba09cf7ce7f4cffd900f23374bf0b\n"},
		{[]string{"--zero"}, "1219131554 3\x00"},
	}
	for _, tt := range tests {
		out, errb, code := runTool(t, "", "abc", tt.args...)
		if out != tt.want || errb != "" || code != 0 {
			t.Fatalf("cksum %v = (%q, %q, %d), want %q", tt.args, out, errb, code, tt.want)
		}
	}

	out, errb, code := runTool(t, "", "abc", "--algorithm=sha1", "--raw")
	wantRaw := []byte{0xa9, 0x99, 0x3e, 0x36, 0x47, 0x06, 0x81, 0x6a, 0xba, 0x3e, 0x25, 0x71, 0x78, 0x50, 0xc2, 0x6c, 0x9c, 0xd0, 0xd8, 0x9d}
	if !bytes.Equal([]byte(out), wantRaw) || errb != "" || code != 0 {
		t.Fatalf("raw sha1 = (%q, %q, %d)", out, errb, code)
	}

	out, errb, code = runTool(t, "", "abc", "--algorithm=shake128", "--length=128")
	if out != "SHAKE128 (-) = 5881092dd818bf5cf8a3ddb793fbcba7\n" || errb != "" || code != 0 {
		t.Fatalf("shake128 = (%q, %q, %d)", out, errb, code)
	}
}

func TestCKSumCheckMode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	checks := "1219131554 3 a.txt\n"
	out, errb, code := runTool(t, dir, checks, "-c")
	if code != 0 || out != "a.txt: OK\n" || errb != "" {
		t.Fatalf("crc check = (%q, %q, %d)", out, errb, code)
	}

	checks = "SHA1 (a.txt) = a9993e364706816aba3e25717850c26c9cd0d89d\n"
	out, errb, code = runTool(t, dir, checks, "--algorithm=sha1", "-c")
	if code != 0 || out != "a.txt: OK\n" || errb != "" {
		t.Fatalf("sha1 tagged check = (%q, %q, %d)", out, errb, code)
	}

	checks = "900150983cd24fb0d6963f7d28e17f72  a.txt\n"
	out, errb, code = runTool(t, dir, checks, "--algorithm=md5", "--untagged", "-c", "--quiet")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("md5 quiet check = (%q, %q, %d)", out, errb, code)
	}

	checks = "1219131554 3 missing.txt\n"
	out, errb, code = runTool(t, dir, checks, "-c", "--ignore-missing")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("ignore missing = (%q, %q, %d)", out, errb, code)
	}

	checks = "1219131554 3 a.txt\x00"
	out, errb, code = runTool(t, dir, checks, "-c", "--zero")
	if code != 0 || out != "a.txt: OK\n" || errb != "" {
		t.Fatalf("zero check = (%q, %q, %d)", out, errb, code)
	}

	checks = "not a checksum\n"
	out, errb, code = runTool(t, dir, checks, "-c", "--strict")
	if code != 1 || out != "" || !strings.Contains(errb, "no properly formatted checksum lines found") {
		t.Fatalf("strict malformed = (%q, %q, %d)", out, errb, code)
	}
}

func TestCKSumErrors(t *testing.T) {
	_, errb, code := runTool(t, "", "", "missing")
	if code != 1 || !strings.Contains(errb, "cksum: missing: No such file or directory") {
		t.Fatalf("missing = (%q, %d)", errb, code)
	}
	_, errb, code = runTool(t, "", "", "--algorithm=definitely-not-real")
	if code != 2 || !strings.Contains(errb, "invalid algorithm") {
		t.Fatalf("unsupported algorithm = (%q, %d)", errb, code)
	}
	_, errb, code = runTool(t, "", "", "--algorithm=sha1", "--raw", "--base64")
	if code != 2 || !strings.Contains(errb, "mutually exclusive") {
		t.Fatalf("conflicting encodings = (%q, %d)", errb, code)
	}
	out, errb, code := runTool(t, "", "", "--debug")
	if code != 0 || errb != "" || !strings.Contains(out, "hardware acceleration managed by Go runtime") || !strings.Contains(out, "4294967295 0") {
		t.Fatalf("debug = (%q, %q, %d)", out, errb, code)
	}
}
