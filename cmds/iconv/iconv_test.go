package iconvcmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func invoke(t *testing.T, input string, args ...string) (int, []byte, string) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &err},
	}
	return run(rc, args), out.Bytes(), err.String()
}

func TestUTF8ToISO88591(t *testing.T) {
	code, out, errout := invoke(t, "café\n", "-f", "UTF-8", "-t", "ISO-8859-1")
	if code != 0 || !bytes.Equal(out, []byte{'c', 'a', 'f', 0xe9, '\n'}) || errout != "" {
		t.Fatalf("code=%d out=% x err=%q", code, out, errout)
	}
}

func TestISO88591ToUTF8(t *testing.T) {
	code, out, errout := invoke(t, string([]byte{'c', 'a', 'f', 0xe9}), "--from-code=ISO-8859-1", "--to-code=UTF-8")
	if code != 0 || string(out) != "café" || errout != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errout)
	}
}

func TestGB18030RoundTrip(t *testing.T) {
	code, encoded, errout := invoke(t, "中国", "-f", "UTF-8", "-t", "GB18030")
	if code != 0 || errout != "" {
		t.Fatalf("encode: code=%d err=%q", code, errout)
	}
	code, decoded, errout := invoke(t, string(encoded), "-f", "GB18030", "-t", "UTF-8")
	if code != 0 || string(decoded) != "中国" || errout != "" {
		t.Fatalf("decode: code=%d out=%q err=%q", code, decoded, errout)
	}
}

func TestFilesResolveAgainstRunContextDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/one", []byte("one "), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/two", []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errout}}
	if code := run(rc, []string{"-f", "UTF-8", "-t", "UTF-8", "one", "two"}); code != 0 {
		t.Fatalf("code=%d err=%q", code, errout.String())
	}
	if out.String() != "one two" {
		t.Fatalf("out=%q", out.String())
	}
}

func TestMalformedInputFailsAndSilentOnlySuppressesConversionMessage(t *testing.T) {
	for _, args := range [][]string{
		{"-f", "UTF-8", "-t", "ISO-8859-1"},
		{"-s", "-f", "UTF-8", "-t", "ISO-8859-1"},
	} {
		code, _, errout := invoke(t, string([]byte{0xff}), args...)
		if code != 1 {
			t.Fatalf("args=%v code=%d", args, code)
		}
		if args[0] == "-s" && errout != "" {
			t.Fatalf("silent stderr=%q", errout)
		}
		if args[0] != "-s" && !strings.Contains(errout, "standard input") {
			t.Fatalf("stderr=%q", errout)
		}
	}
}

func TestUnrepresentableOutputFails(t *testing.T) {
	code, _, errout := invoke(t, "€", "-f", "UTF-8", "-t", "ISO-8859-1")
	if code != 1 || !strings.Contains(errout, "standard input") {
		t.Fatalf("code=%d err=%q", code, errout)
	}
}

func TestUnsupportedEncodingAndDiscardOptionFailLoudly(t *testing.T) {
	code, _, errout := invoke(t, "", "-f", "not-a-charset", "-t", "UTF-8")
	if code != 1 || !strings.Contains(errout, "unsupported encoding") {
		t.Fatalf("encoding: code=%d err=%q", code, errout)
	}
	code, _, errout = invoke(t, "", "-c", "-f", "UTF-8", "-t", "UTF-8")
	if code != 2 || !strings.Contains(errout, "option '-c' is not supported") {
		t.Fatalf("-c: code=%d err=%q", code, errout)
	}
}

func TestMissingEncodingIsUsageError(t *testing.T) {
	code, _, errout := invoke(t, "", "-t", "UTF-8")
	if code != 2 || !strings.Contains(errout, "missing source encoding") {
		t.Fatalf("code=%d err=%q", code, errout)
	}
}
