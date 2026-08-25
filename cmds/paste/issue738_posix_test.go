// Tests closing the paste(1) POSIX Issue 7 audit residuals recorded in
// docs/posix-interface-audits/go-evidence-closure-batch-2.md and
// docs/posix-interface-audits/sprint-79-consolidated.md: locale-aware -d
// LIST decoding, repeated "-" under -s, the 12-operand minimum, the "\\"
// escape, serial "\0", injected read errors, and stdout write failures.
package pastecmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// TestIssue738LocaleAwareDelimiterDecoding pins that -d LIST is split into
// delimiter characters per the invocation's LC_CTYPE, not decoded as UTF-8
// unconditionally. The two-byte sequence 0xC3 0xA4 (UTF-8 for "ä") is one
// delimiter under a UTF-8 locale but two single-byte delimiters under
// C/POSIX and the carried de_DE.ISO-8859-1 alias.
func TestIssue738LocaleAwareDelimiterDecoding(t *testing.T) {
	twoByte := string([]byte{0xC3, 0xA4})
	files := map[string]string{"f1": "a\n", "f2": "b\n", "f3": "c\n"}
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{"default-posix-single-byte", nil, "a\xC3b\xA4c\n"},
		{"c-single-byte", []string{"LC_ALL=C"}, "a\xC3b\xA4c\n"},
		{"posix-single-byte", []string{"LC_ALL=POSIX"}, "a\xC3b\xA4c\n"},
		{"c-utf8-one-character", []string{"LC_ALL=C.UTF-8"}, "a" + twoByte + "b" + twoByte + "c\n"},
		{"posix-utf8-alias", []string{"LC_CTYPE=POSIX.UTF-8"}, "a" + twoByte + "b" + twoByte + "c\n"},
		{"de_DE-iso88591-single-byte", []string{"LC_ALL=de_DE.ISO-8859-1"}, "a\xC3b\xA4c\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeFiles(t, files)
			out, errb, code := runToolDirEnv(t, dir, tc.env, "", "-d", twoByte, "f1", "f2", "f3")
			if code != 0 || errb != "" || out != tc.want {
				t.Fatalf("out=%q errb=%q code=%d, want %q", out, errb, code, tc.want)
			}
		})
	}
}

// TestIssue738LCCTypePrecedence mirrors the fold precedent: LC_ALL beats
// the category variable, which beats LANG, which beats the POSIX default;
// empty values fall through to the next level.
func TestIssue738LCCTypePrecedence(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		want encodingMode
	}{
		{"default", nil, encodingSingleByte},
		{"lang", []string{"LANG=C.UTF-8"}, encodingUTF8},
		{"lc-ctype-over-lang", []string{"LANG=C.UTF-8", "LC_CTYPE=de_DE.ISO-8859-1"}, encodingSingleByte},
		{
			"lc-all-over-category",
			[]string{"LANG=C.UTF-8", "LC_CTYPE=C.UTF-8", "LC_ALL=POSIX"},
			encodingSingleByte,
		},
		{
			"empty-values-fall-through",
			[]string{"LANG=C.UTF-8", "LC_CTYPE=", "LC_ALL="}, encodingUTF8,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, err := resolveCharacterModel(tc.env)
			if err != nil || model.encoding != tc.want {
				t.Fatalf("model=(%v, %v), want encoding %v", model, err, tc.want)
			}
		})
	}
}

// TestIssue738UnsupportedLocaleFailsBeforeOpeningOperand pins that an
// unsupported LC_CTYPE is diagnosed, with status 1, before any operand
// (here a nonexistent file) is opened — the diagnostic never mentions the
// operand at all.
func TestIssue738UnsupportedLocaleFailsBeforeOpeningOperand(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(), Env: []string{"LC_ALL=x-test"},
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{"does-not-exist"})
	if code != 1 || out.Len() != 0 ||
		!strings.Contains(errb.String(), `LC_CTYPE "x-test"`) ||
		strings.Contains(errb.String(), "does-not-exist") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errb.String())
	}
}

// TestIssue738RepeatedDashUnderSerial pins the OPERANDS/-s resolution this
// implementation makes for repeated "-" operands under -s: every "-" shares
// one invocation-wide stdin reader, so the first "-" reads to EOF and later
// "-" operands see an already-exhausted stream, producing the same bare
// newline an empty file would (POSIX states this for an empty *file*; here
// it documents the chosen, previously-untested resolution for stdin reuse).
func TestIssue738RepeatedDashUnderSerial(t *testing.T) {
	out, errb, code := runToolDir(t, t.TempDir(), "1\n2\n3\n", "-s", "-", "-")
	if code != 0 || errb != "" || out != "1\t2\t3\n\n" {
		t.Fatalf("out=%q errb=%q code=%d", out, errb, code)
	}
}

// TestIssue738TwelveOperands pins the POSIX Issue 7 OPERANDS requirement
// that implementations support at least twelve file operands, in both
// parallel and serial mode.
func TestIssue738TwelveOperands(t *testing.T) {
	files := map[string]string{}
	names := make([]string, 12)
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("f%02d", i)
		names[i] = name
		files[name] = fmt.Sprintf("%d\n", i)
	}
	dir := writeFiles(t, files)

	out, errb, code := runToolDir(t, dir, "", names...)
	want := "0\t1\t2\t3\t4\t5\t6\t7\t8\t9\t10\t11\n"
	if code != 0 || errb != "" || out != want {
		t.Fatalf("parallel: out=%q errb=%q code=%d, want %q", out, errb, code, want)
	}

	out, errb, code = runToolDir(t, dir, "", append([]string{"-s"}, names...)...)
	wantSerial := "0\n1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n"
	if code != 0 || errb != "" || out != wantSerial {
		t.Fatalf("serial: out=%q errb=%q code=%d, want %q", out, errb, code, wantSerial)
	}
}

// TestIssue738BackslashEscapedDelimiter pins the "\\" -d escape (a literal
// backslash delimiter), previously accepted by parseDelims but untested.
func TestIssue738BackslashEscapedDelimiter(t *testing.T) {
	dir := writeFiles(t, map[string]string{"f1": "a\n", "f2": "b\n"})
	out, errb, code := runToolDir(t, dir, "", "-d", `\\`, "f1", "f2")
	if code != 0 || errb != "" || out != "a\\b\n" {
		t.Fatalf("out=%q errb=%q code=%d, want %q", out, errb, code, `a\b`+"\n")
	}
}

// TestIssue738SerialZeroDelimiter pins "\0" (no delimiter) under -s: lines
// within a file are concatenated with nothing between them.
func TestIssue738SerialZeroDelimiter(t *testing.T) {
	dir := writeFiles(t, map[string]string{"f1": "a\nb\nc\n"})
	out, errb, code := runToolDir(t, dir, "", "-s", "-d", `\0`, "f1")
	if code != 0 || errb != "" || out != "abc\n" {
		t.Fatalf("out=%q errb=%q code=%d", out, errb, code)
	}
}

// issue738ErrorReader yields data bytes once, then a fixed error on every
// subsequent Read — an injected mid-stream read failure, not EOF.
type issue738ErrorReader struct {
	data []byte
	err  error
	sent bool
}

func (r *issue738ErrorReader) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		if len(r.data) > 0 {
			return copy(p, r.data), nil
		}
	}
	return 0, r.err
}

var errIssue738Injected = errors.New("injected read failure")

// TestIssue738InjectedReadErrorParallel pins that a mid-stream (non-EOF)
// read error on one operand diagnoses that operand and aborts with status
// 1, while rows already written to stdout before the failing row remain.
func TestIssue738InjectedReadErrorParallel(t *testing.T) {
	var errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Err: &errb}}
	open := func(name string) (*bufio.Reader, io.Closer, error) {
		switch name {
		case "good":
			return bufio.NewReader(strings.NewReader("x\ny\n")), nil, nil
		case "bad":
			return bufio.NewReader(&issue738ErrorReader{data: []byte("a\n"), err: errIssue738Injected}), nil, nil
		}
		return nil, nil, fmt.Errorf("unexpected operand %q", name)
	}
	var outBuf bytes.Buffer
	out := bufio.NewWriter(&outBuf)
	dc := &delimCycle{list: [][]byte{[]byte("\t")}}
	code := pasteParallel(rc, []string{"good", "bad"}, open, dc, out, '\n')
	out.Flush()
	if code != 1 || outBuf.String() != "x\ta\n" || !strings.Contains(errb.String(), "bad") ||
		!strings.Contains(errb.String(), errIssue738Injected.Error()) {
		t.Fatalf("out=%q errb=%q code=%d", outBuf.String(), errb.String(), code)
	}
}

// TestIssue738InjectedReadErrorSerial pins that under -s a mid-stream read
// error diagnoses and skips only its own file's output line: earlier and
// later files' lines are still written, and the overall status is >0.
func TestIssue738InjectedReadErrorSerial(t *testing.T) {
	var errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Err: &errb}}
	open := func(name string) (*bufio.Reader, io.Closer, error) {
		switch name {
		case "good1":
			return bufio.NewReader(strings.NewReader("x\ny\n")), nil, nil
		case "bad":
			return bufio.NewReader(&issue738ErrorReader{data: []byte("a\n"), err: errIssue738Injected}), nil, nil
		case "good2":
			return bufio.NewReader(strings.NewReader("p\nq\n")), nil, nil
		}
		return nil, nil, fmt.Errorf("unexpected operand %q", name)
	}
	var outBuf bytes.Buffer
	out := bufio.NewWriter(&outBuf)
	dc := &delimCycle{list: [][]byte{[]byte("\t")}}
	code := pasteSerial(rc, []string{"good1", "bad", "good2"}, open, dc, out, '\n')
	out.Flush()
	want := "x\ty\np\tq\n"
	if code != 1 || outBuf.String() != want || !strings.Contains(errb.String(), "bad") ||
		!strings.Contains(errb.String(), errIssue738Injected.Error()) {
		t.Fatalf("out=%q errb=%q code=%d, want out %q", outBuf.String(), errb.String(), code, want)
	}
}

// issue738FailWriter always fails; issue738ShortWriter silently writes one
// byte short with a nil error, which bufio.Writer turns into a sticky
// io.ErrShortWrite once its buffer is flushed.
type issue738FailWriter struct{}

func (issue738FailWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

type issue738ShortWriter struct{}

func (issue738ShortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

// TestIssue738OutputWriteErrors pins that a stdout write/flush failure is
// diagnosed and reported with status 1, for a hard failure and for a short
// write, in both parallel and serial mode.
func TestIssue738OutputWriteErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		serial bool
		out    io.Writer
	}{
		{"parallel-fail", false, issue738FailWriter{}},
		{"parallel-short-write", false, issue738ShortWriter{}},
		{"serial-fail", true, issue738FailWriter{}},
		{"serial-short-write", true, issue738ShortWriter{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errb bytes.Buffer
			rc := &tool.RunContext{
				Ctx: context.Background(), Dir: t.TempDir(),
				Stdio: tool.Stdio{In: strings.NewReader("a\nb\n"), Out: tc.out, Err: &errb},
			}
			args := []string{}
			if tc.serial {
				args = append(args, "-s")
			}
			code := cmd.Run(rc, args)
			if code != 1 || !strings.Contains(errb.String(), "write error") {
				t.Fatalf("code=%d errb=%q, want status 1 and a write-error diagnostic", code, errb.String())
			}
		})
	}
}
