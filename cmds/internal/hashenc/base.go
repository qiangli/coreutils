// Portions adapted from https://github.com/guonaihong/coreutils
// basecore/basecore.go (Apache-2.0) and https://github.com/u-root/u-root
// cmds/core/base64 (BSD-3-Clause).
// Changes: rewired to the tool framework (RunContext stdio/cwd, strict
// flag layer); wrap writer and decode filtering rewritten to match GNU
// exactly (default 76-column wrap with trailing newline, -w 0 emits no
// trailing newline, decoder tolerates embedded newlines only, -i drops
// every non-alphabet byte, auto-padding of unpadded input at EOF and
// rejection of non-zero padding bits per GNU >= 9.5, "invalid input"
// diagnostic + exit 1).

package hashenc

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

// BaseSpec describes one base-encoding tool (base64, base32).
type BaseSpec struct {
	Name       string // command name, e.g. "base64"
	Display    string // synopsis label, e.g. "Base64"
	NewEncoder func(io.Writer) io.WriteCloser
	NewDecoder func(io.Reader) io.Reader
	InAlphabet func(byte) bool // encoding alphabet, excluding '=' padding
}

// NewBaseTool builds the registered Tool for one base-encoding command.
func NewBaseTool(spec BaseSpec) *tool.Tool {
	t := &tool.Tool{
		Name:     spec.Name,
		Synopsis: fmt.Sprintf("%s encode or decode FILE, or standard input, to standard output. With no FILE, or when FILE is -, read standard input.", spec.Display),
		Usage: fmt.Sprintf("%s [OPTION]... [FILE]\n\n"+
			"  -D          same as --decode", spec.Name),
	}
	t.Run = func(rc *tool.RunContext, args []string) int {
		return runBase(rc, t, spec, args)
	}
	return t
}

func runBase(rc *tool.RunContext, t *tool.Tool, spec BaseSpec, args []string) int {
	fs := tool.NewFlags(t.Name)
	decode := fs.BoolP("decode", "d", false, "decode data")
	decodeUpper := fs.BoolP("decode-uppercase", "D", false, "same as --decode")
	if flag := fs.Lookup("decode-uppercase"); flag != nil {
		flag.Hidden = true
	}
	ignore := fs.BoolP("ignore-garbage", "i", false, "when decoding, ignore non-alphabet characters")
	wrap := fs.StringP("wrap", "w", "76", "wrap encoded lines after COLS character (default 76); use 0 to disable line wrapping")
	operands, code := tool.Parse(rc, t, fs, args)
	if code >= 0 {
		return code
	}
	cols, err := strconv.ParseInt(*wrap, 10, 64)
	if err != nil || cols < 0 {
		fmt.Fprintf(rc.Err, "%s: invalid wrap size: '%s'\n", t.Name, *wrap)
		return 1
	}
	if len(operands) > 1 {
		return tool.UsageError(rc, t, "extra operand %q", operands[1])
	}
	name := "-"
	if len(operands) == 1 {
		name = operands[0]
	}

	var src io.Reader
	if name == "-" {
		src = rc.In
		if src == nil {
			src = strings.NewReader("")
		}
	} else {
		f, err := os.Open(rc.Path(name))
		if err != nil {
			fmt.Fprintf(rc.Err, "%s: %s: %s\n", t.Name, name, gnuErrMsg(err))
			return 1
		}
		defer f.Close()
		if fi, err := f.Stat(); err == nil && fi.IsDir() {
			fmt.Fprintf(rc.Err, "%s: %s: %s\n", t.Name, name, gnuErrMsg(errIsDirectory))
			return 1
		}
		src = f
	}

	if *decode || *decodeUpper {
		// -w is accepted but has no effect when decoding (GNU).
		return runBaseDecode(rc, t, spec, src, *ignore)
	}
	return runBaseEncode(rc, t, src, spec, cols)
}

func runBaseEncode(rc *tool.RunContext, t *tool.Tool, src io.Reader, spec BaseSpec, cols int64) int {
	bw := bufio.NewWriterSize(rc.Out, 64<<10)
	ww := &wrapWriter{w: bw, cols: cols}
	enc := spec.NewEncoder(ww)
	if _, err := io.Copy(enc, src); err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", t.Name, err)
		return 1
	}
	if err := enc.Close(); err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", t.Name, err)
		return 1
	}
	if err := ww.finish(); err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", t.Name, err)
		return 1
	}
	if err := bw.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", t.Name, err)
		return 1
	}
	return 0
}

func runBaseDecode(rc *tool.RunContext, t *tool.Tool, spec BaseSpec, src io.Reader, ignoreGarbage bool) int {
	data, err := io.ReadAll(src)
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", t.Name, err)
		return 1
	}
	decoded, err := DecodeBase(data, spec.InAlphabet, ignoreGarbage, spec.NewEncoder, spec.NewDecoder)
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: invalid input\n", t.Name)
		return 1
	}
	if _, err := rc.Out.Write(decoded); err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", t.Name, err)
		return 1
	}
	return 0
}

// ErrInvalidInput is the sentinel for undecodable input (garbage bytes
// without -i, bad padding, non-zero padding bits).
var ErrInvalidInput = errors.New("invalid input")

// DecodeBase decodes a whole encoded buffer with GNU base64/base32/
// basenc semantics (coreutils >= 9.5):
//
//   - newlines are always tolerated; with ignore every byte outside
//     alphabet+padding is dropped, without it such a byte is an error;
//   - input whose final group is unpadded is auto-padded at EOF
//     ("QQ" decodes like "QQ==");
//   - non-canonical input — non-zero padding bits ("QR=="), wrong
//     padding length, padding in the middle — is rejected. This is
//     enforced by re-encoding the decoded bytes and requiring an exact
//     round trip, which is equivalent to decoding in the stdlib's
//     Strict mode plus GNU's padding checks.
//
// The encoded block size (4 for base64, 8 for base32) is derived from
// the encoder itself so the helper works for any RFC 4648 encoding.
func DecodeBase(data []byte, inAlphabet func(byte) bool, ignore bool,
	newEncoder func(io.Writer) io.WriteCloser, newDecoder func(io.Reader) io.Reader) ([]byte, error) {
	filtered := make([]byte, 0, len(data))
	for _, b := range data {
		switch {
		case inAlphabet(b) || b == '=':
			filtered = append(filtered, b)
		case ignore || b == '\n':
			// dropped
		default:
			return nil, ErrInvalidInput
		}
	}
	if len(filtered) == 0 {
		return nil, nil
	}
	block := encodedBlockLen(newEncoder)
	// Auto-pad at EOF (GNU >= 9.5): complete a final partial group
	// with '=' unless the input already carries its own padding.
	if r := len(filtered) % block; r != 0 && filtered[len(filtered)-1] != '=' {
		filtered = append(filtered, bytes.Repeat([]byte{'='}, block-r)...)
	}
	decoded, err := io.ReadAll(newDecoder(bytes.NewReader(filtered)))
	if err != nil {
		return nil, ErrInvalidInput
	}
	var canon bytes.Buffer
	enc := newEncoder(&canon)
	_, _ = enc.Write(decoded)
	_ = enc.Close()
	if !bytes.Equal(canon.Bytes(), filtered) {
		return nil, ErrInvalidInput
	}
	return decoded, nil
}

// encodedBlockLen returns the padded output length of a one-byte
// encode: the encoding's block size (base64: 4, base32: 8).
func encodedBlockLen(newEncoder func(io.Writer) io.WriteCloser) int {
	var buf bytes.Buffer
	enc := newEncoder(&buf)
	_, _ = enc.Write([]byte{0})
	_ = enc.Close()
	if n := buf.Len(); n > 0 {
		return n
	}
	return 4
}

// wrapWriter wraps encoder output at cols characters. GNU semantics:
// with cols > 0 every line — including the final partial one — ends
// with a newline; with cols == 0 nothing is inserted and no trailing
// newline is emitted; empty input produces no output at all.
type wrapWriter struct {
	w    io.Writer
	cols int64
	col  int64
}

func (ww *wrapWriter) Write(p []byte) (int, error) {
	if ww.cols <= 0 {
		return ww.w.Write(p)
	}
	written := 0
	for len(p) > 0 {
		if ww.col == ww.cols {
			if _, err := io.WriteString(ww.w, "\n"); err != nil {
				return written, err
			}
			ww.col = 0
		}
		chunk := p
		if room := ww.cols - ww.col; int64(len(chunk)) > room {
			chunk = chunk[:room]
		}
		n, err := ww.w.Write(chunk)
		written += n
		ww.col += int64(n)
		if err != nil {
			return written, err
		}
		p = p[n:]
	}
	return written, nil
}

func (ww *wrapWriter) finish() error {
	if ww.cols > 0 && ww.col > 0 {
		_, err := io.WriteString(ww.w, "\n")
		return err
	}
	return nil
}
