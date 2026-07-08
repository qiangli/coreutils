package cksumcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
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

func TestCKSumDeprecatedBinaryTextAliases(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-b", "a.txt"},
		{"--binary", "a.txt"},
		{"-t", "a.txt"},
		{"--text", "a.txt"},
	} {
		out, errb, code := runTool(t, dir, "", args...)
		if out != "1219131554 3 a.txt\n" || errb != "" || code != 0 {
			t.Fatalf("cksum %v = (%q, %q, %d)", args, out, errb, code)
		}
	}
}

func TestCKSumAlgorithms(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--algorithm=bsd"}, "16556     1\n"},
		{[]string{"-r"}, "16556     1\n"},
		{[]string{"--algorithm=sysv"}, "294 1\n"},
		{[]string{"-s"}, "294 1\n"},
		{[]string{"--algorithm=sha1"}, "SHA1 (-) = a9993e364706816aba3e25717850c26c9cd0d89d\n"},
		{[]string{"--algorithm=sha1", "--untagged"}, "a9993e364706816aba3e25717850c26c9cd0d89d  -\n"},
		{[]string{"--algorithm=sha1", "--base64"}, "SHA1 (-) = qZk+NkcGgWq6PiVxeFDCbJzQ2J0=\n"},
		{[]string{"--algorithm=md5"}, "MD5 (-) = 900150983cd24fb0d6963f7d28e17f72\n"},
		// crc32b prints a DECIMAL untagged checksum + length, like crc
		// (GNU dispatches it to output_crc). 0x352441c2 = 891568578.
		{[]string{"--algorithm=crc32b"}, "891568578 3\n"},
		{[]string{"--algorithm=sha2", "--length=256"}, "SHA256 (-) = ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\n"},
		{[]string{"--algorithm=sha3", "--length=256"}, "SHA3-256 (-) = 3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532\n"},
		{[]string{"--algorithm=sha3", "--length=512"}, "SHA3-512 (-) = b751850b1a57168a5693cd924b6b096e08f621827444f70d884f5d0240d2712e10e116e9192af3c91a7ec57647e3934057340b4cf408d5a56592f8274eec53f0\n"},
		{[]string{"--algorithm=blake2b", "--length=128", "--tag"}, "BLAKE2b-128 (-) = cf4ab791c62b8d2b2109c90275287816\n"},
		{[]string{"--algorithm=blake2b", "--length=0"}, "BLAKE2b (-) = ba80a53f981c4d0d6a2797b69f12f6e94c212f14685ac4b74b12bb6fdbffa2d17d87c5392aab792dc252d5de4533cc9518d38aa8dbf1925ab92386edd4009923\n"},
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

	out, errb, code := runTool(t, "", "abc", "-rs")
	if out != "294 1\n" || errb != "" || code != 0 {
		t.Fatalf("cksum -rs = (%q, %q, %d), want sysv", out, errb, code)
	}
	out, errb, code = runTool(t, "", "abc", "-sr")
	if out != "16556     1\n" || errb != "" || code != 0 {
		t.Fatalf("cksum -sr = (%q, %q, %d), want bsd", out, errb, code)
	}

	out, errb, code = runTool(t, "", "abc", "--algorithm=sha1", "--raw")
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

	// Plain `cksum -c` verifies only BSD-tagged digest lines, auto-
	// detecting the algorithm per line from the tag (GNU). Length-
	// suffixed tags select the digest size.
	checks := "SHA1 (a.txt) = a9993e364706816aba3e25717850c26c9cd0d89d\n" +
		"SHA256 (a.txt) = ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\n" +
		"BLAKE2b-128 (a.txt) = cf4ab791c62b8d2b2109c90275287816\n" +
		"SHA3-256 (a.txt) = 3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532\n"
	out, errb, code := runTool(t, dir, checks, "-c")
	if code != 0 || out != strings.Repeat("a.txt: OK\n", 4) || errb != "" {
		t.Fatalf("auto-detect check = (%q, %q, %d)", out, errb, code)
	}

	// `CRC LEN FILE` lines are NOT parsed by cksum -c.
	checks = "1219131554 3 a.txt\n"
	out, errb, code = runTool(t, dir, checks, "-c")
	if code != 1 || out != "" || !strings.Contains(errb, "no properly formatted checksum lines found") {
		t.Fatalf("crc line check = (%q, %q, %d)", out, errb, code)
	}

	// --check with a non-digest -a is a hard error (GNU).
	_, errb, code = runTool(t, dir, "", "-c", "-a", "crc")
	if code != 2 || !strings.Contains(errb, "--check is not supported with --algorithm={bsd,sysv,crc,crc32b}") {
		t.Fatalf("crc -c = (%q, %d)", errb, code)
	}
	_, errb, code = runTool(t, dir, "", "-c", "-a", "bsd")
	if code != 2 || !strings.Contains(errb, "--check is not supported with --algorithm={bsd,sysv,crc,crc32b}") {
		t.Fatalf("bsd -c = (%q, %d)", errb, code)
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

	// -a sha2 -c needs no --length; the tag picks the member.
	checks = "SHA256 (a.txt) = ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\n"
	out, errb, code = runTool(t, dir, checks, "--algorithm=sha2", "-c")
	if code != 0 || out != "a.txt: OK\n" || errb != "" {
		t.Fatalf("sha2 tagged check = (%q, %q, %d)", out, errb, code)
	}

	checks = "MD5 (missing.txt) = 900150983cd24fb0d6963f7d28e17f72\n"
	out, errb, code = runTool(t, dir, checks, "-c", "--ignore-missing")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("ignore missing = (%q, %q, %d)", out, errb, code)
	}

	// --zero is rejected when verifying (GNU).
	_, errb, code = runTool(t, dir, "", "-c", "--zero")
	if code != 2 || !strings.Contains(errb, "the --zero option is not supported when verifying checksums") {
		t.Fatalf("zero check = (%q, %d)", errb, code)
	}

	checks = "not a checksum\n"
	out, errb, code = runTool(t, dir, checks, "-c", "--strict")
	if code != 1 || out != "" || !strings.Contains(errb, "no properly formatted checksum lines found") {
		t.Fatalf("strict malformed = (%q, %q, %d)", out, errb, code)
	}

	checks = "SHA1 (a.txt) = 0000000000000000000000000000000000000000\n"
	out, errb, code = runTool(t, dir, checks, "-c", "--status")
	if code != 1 || out != "" || errb != "" {
		t.Fatalf("status mismatch = (%q, %q, %d)", out, errb, code)
	}

	// --warn diagnoses each malformed line with its number.
	checks = "SHA1 (a.txt) = a9993e364706816aba3e25717850c26c9cd0d89d\ngarbage\n"
	out, errb, code = runTool(t, dir, checks, "-c", "--warn")
	if code != 0 || out != "a.txt: OK\n" ||
		!strings.Contains(errb, "cksum: 'standard input': 2: improperly formatted SHA1 checksum line") {
		t.Fatalf("warn = (%q, %q, %d)", out, errb, code)
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
	// --debug information goes to stderr, not stdout (GNU).
	out, errb, code := runTool(t, "", "", "--debug")
	if code != 0 || !strings.Contains(errb, "hardware acceleration managed by Go runtime") || out != "4294967295 0\n" {
		t.Fatalf("debug = (%q, %q, %d)", out, errb, code)
	}
	// GNU algorithm names are matched exactly.
	_, errb, code = runTool(t, "", "", "--algorithm=MD5")
	if code != 2 || !strings.Contains(errb, "invalid algorithm") {
		t.Fatalf("uppercase GNU algorithm = (%q, %d)", errb, code)
	}
}

func TestCKSumHelpVersionAliases(t *testing.T) {
	out, _, code := runTool(t, "", "", "-h")
	if code != 0 || !strings.Contains(out, "Usage: cksum") ||
		!strings.Contains(out, "-r          use BSD sum algorithm") ||
		!strings.Contains(out, "-s          use System V sum algorithm") ||
		!strings.Contains(out, "-h, --help") || !strings.Contains(out, "-V, --version") {
		t.Fatalf("-h = (%q, %d)", out, code)
	}
	for _, hidden := range []string{"binary", "text"} {
		if strings.Contains(out, hidden) {
			t.Fatalf("-h contains hidden alias %q in %q", hidden, out)
		}
	}
	out, _, code = runTool(t, "", "", "-V")
	if code != 0 || !strings.Contains(out, "cksum") {
		t.Fatalf("-V = (%q, %d)", out, code)
	}
}

func TestCKSumLengthValidation(t *testing.T) {
	// sha2/sha3 require --length outside check mode.
	_, errb, code := runTool(t, "", "abc", "--algorithm=sha2")
	if code != 2 || !strings.Contains(errb, "--algorithm=sha2 requires specifying --length 224, 256, 384, or 512") {
		t.Fatalf("sha2 without length = (%q, %d)", errb, code)
	}
	_, errb, code = runTool(t, "", "abc", "--algorithm=sha3")
	if code != 2 || !strings.Contains(errb, "--algorithm=sha3 requires specifying --length 224, 256, 384, or 512") {
		t.Fatalf("sha3 without length = (%q, %d)", errb, code)
	}
	_, errb, code = runTool(t, "", "abc", "--algorithm=sha2", "--length=100")
	if code != 2 || !strings.Contains(errb, "cksum: invalid length: '100'") ||
		!strings.Contains(errb, "digest length for 'SHA2' must be 224, 256, 384, or 512") {
		t.Fatalf("sha2 bad length = (%q, %d)", errb, code)
	}
	// --length with an algorithm that doesn't take it.
	_, errb, code = runTool(t, "", "abc", "--algorithm=md5", "--length=128")
	if code != 2 || !strings.Contains(errb, "--length is only supported with --algorithm blake2b, sha2, or sha3") {
		t.Fatalf("md5 with length = (%q, %d)", errb, code)
	}
	_, errb, code = runTool(t, "", "abc", "--length=32")
	if code != 2 || !strings.Contains(errb, "--length is only supported with --algorithm blake2b, sha2, or sha3") {
		t.Fatalf("crc with length = (%q, %d)", errb, code)
	}
	_, errb, code = runTool(t, "", "abc", "--algorithm=blake2b", "--length=1024")
	if code != 2 || !strings.Contains(errb, "maximum digest length for 'BLAKE2b' is 512 bits") {
		t.Fatalf("blake2b too long = (%q, %d)", errb, code)
	}
	_, errb, code = runTool(t, "", "abc", "--algorithm=blake2b", "--length=100")
	if code != 2 || !strings.Contains(errb, "length is not a multiple of 8") {
		t.Fatalf("blake2b not mult 8 = (%q, %d)", errb, code)
	}
}

func TestCKSumRawAndNames(t *testing.T) {
	// --raw works for crc/crc32b (big-endian u32) and bsd/sysv
	// (big-endian u16).
	out, errb, code := runTool(t, "", "abc", "--raw")
	if code != 0 || errb != "" || !bytes.Equal([]byte(out), []byte{0x48, 0xaa, 0x78, 0xa2}) {
		t.Fatalf("crc raw = (%q, %q, %d)", out, errb, code)
	}
	out, errb, code = runTool(t, "", "abc", "-a", "crc32b", "--raw")
	if code != 0 || errb != "" || !bytes.Equal([]byte(out), []byte{0x35, 0x24, 0x41, 0xc2}) {
		t.Fatalf("crc32b raw = (%q, %q, %d)", out, errb, code)
	}
	out, errb, code = runTool(t, "", "abc", "-a", "bsd", "--raw")
	if code != 0 || errb != "" || !bytes.Equal([]byte(out), []byte{0x40, 0xac}) {
		t.Fatalf("bsd raw = (%q, %q, %d)", out, errb, code)
	}
	out, errb, code = runTool(t, "", "abc", "-a", "sysv", "--raw")
	if code != 0 || errb != "" || !bytes.Equal([]byte(out), []byte{0x01, 0x26}) {
		t.Fatalf("sysv raw = (%q, %q, %d)", out, errb, code)
	}

	// --raw with multiple files is an error.
	dir := t.TempDir()
	for _, n := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("abc"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, errb, code = runTool(t, dir, "", "--raw", "a", "b")
	if code != 2 || !strings.Contains(errb, "the --raw option is not supported with multiple files") {
		t.Fatalf("raw multiple = (%q, %d)", errb, code)
	}

	// An explicit "-" operand prints the name.
	out, _, code = runTool(t, "", "abc", "-")
	if out != "1219131554 3 -\n" || code != 0 {
		t.Fatalf("explicit dash = (%q, %d)", out, code)
	}
	out, _, code = runTool(t, "", "abc", "-a", "bsd", "-")
	if out != "16556     1 -\n" || code != 0 {
		t.Fatalf("bsd explicit dash = (%q, %d)", out, code)
	}

	// Check-only options are rejected outside --check (GNU).
	_, errb, code = runTool(t, "", "abc", "--warn")
	if code != 2 || !strings.Contains(errb, "the --warn option is meaningful only when verifying checksums") {
		t.Fatalf("warn without check = (%q, %d)", errb, code)
	}
}

func TestCKSumEscapedFilename(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("backslash is a path separator on windows")
	}
	dir := t.TempDir()
	name := `a\b`
	if err := os.WriteFile(filepath.Join(dir, name), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, "", "-a", "sha1", name)
	if out != `\SHA1 (a\\b) = a9993e364706816aba3e25717850c26c9cd0d89d`+"\n" || code != 0 {
		t.Fatalf("escaped tagged = (%q, %d)", out, code)
	}
	out, _, code = runTool(t, dir, "", "-a", "sha1", "--untagged", name)
	if out != `\a9993e364706816aba3e25717850c26c9cd0d89d  a\\b`+"\n" || code != 0 {
		t.Fatalf("escaped untagged = (%q, %d)", out, code)
	}
}
