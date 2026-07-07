package hashenc

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"hash"
	"io"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runSumTool(t *testing.T, spec SumSpec, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = NewSumTool(spec).Run(rc, args)
	return out.String(), errb.String(), code
}

func md5Spec() SumSpec {
	return SumSpec{
		Name: "md5sum",
		Algo: "MD5",
		Bits: 128,
		New:  func() hash.Hash { return md5.New() },
	}
}

// -l/--length is only registered for variable-length tools (b2sum);
// fixed-size tools reject it as an unknown flag.
func TestLengthFlagOnlyForVariableTools(t *testing.T) {
	_, errb, code := runSumTool(t, md5Spec(), "abc", "--length=128")
	if code != 2 || !strings.Contains(errb, "length") {
		t.Fatalf("md5sum --length = (%q, %d), want unknown flag + exit 2", errb, code)
	}
	_, errb, code = runSumTool(t, md5Spec(), "abc", "-l", "128")
	if code != 2 {
		t.Fatalf("md5sum -l = (%q, %d), want exit 2", errb, code)
	}
}

func TestCheckOnlyOptionGating(t *testing.T) {
	for _, c := range []struct{ flag, want string }{
		{"--status", "the --status option is meaningful only when verifying checksums"},
		{"--warn", "the --warn option is meaningful only when verifying checksums"},
		{"--quiet", "the --quiet option is meaningful only when verifying checksums"},
		{"--strict", "the --strict option is meaningful only when verifying checksums"},
		{"--ignore-missing", "the --ignore-missing option is meaningful only when verifying checksums"},
	} {
		_, errb, code := runSumTool(t, md5Spec(), "abc", c.flag)
		if code != 2 || !strings.Contains(errb, c.want) {
			t.Errorf("%s = (%q, %d), want %q", c.flag, errb, code, c.want)
		}
	}
	_, errb, code := runSumTool(t, md5Spec(), "abc", "--tag", "-t")
	if code != 2 || !strings.Contains(errb, "--tag does not support --text mode") {
		t.Errorf("--tag -t = (%q, %d)", errb, code)
	}
}

func b64Enc(w io.Writer) io.WriteCloser { return base64.NewEncoder(base64.StdEncoding, w) }
func b64Dec(r io.Reader) io.Reader      { return base64.NewDecoder(base64.StdEncoding, r) }
func b64Alpha(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '+' || b == '/'
}

// DecodeBase implements the GNU >= 9.5 decode contract: auto-pad at
// EOF, reject non-zero padding bits and non-canonical padding.
func TestDecodeBase(t *testing.T) {
	ok := []struct{ in, want string }{
		{"", ""},
		{"QQ==", "A"},
		{"QQ", "A"}, // auto-padded
		{"Zm9vYmFy", "foobar"},
		{"Zm9v\nYmFy\n", "foobar"},
		{"QQQ=", "A\x04"}, // zero trailing bits: valid
	}
	for _, c := range ok {
		got, err := DecodeBase([]byte(c.in), b64Alpha, false, b64Enc, b64Dec)
		if err != nil || string(got) != c.want {
			t.Errorf("DecodeBase(%q) = (%q, %v), want %q", c.in, got, err, c.want)
		}
	}
	bad := []string{
		"QR==",      // non-zero padding bits
		"QR",        // ditto after auto-pad
		"Q",         // impossible length
		"QQ=",       // wrong padding length
		"Zm9v;YmFy", // garbage without ignore
	}
	for _, in := range bad {
		if got, err := DecodeBase([]byte(in), b64Alpha, false, b64Enc, b64Dec); err == nil {
			t.Errorf("DecodeBase(%q) = %q, want invalid input", in, got)
		}
	}
	// ignore-garbage drops non-alphabet bytes but keeps '='.
	got, err := DecodeBase([]byte("Q;Q=!="), b64Alpha, true, b64Enc, b64Dec)
	if err != nil || string(got) != "A" {
		t.Errorf("DecodeBase ignore = (%q, %v), want %q", got, err, "A")
	}
}
