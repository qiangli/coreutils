// Package basenccmd implements basenc(1) for common RFC 4648 encodings.
package basenccmd

import (
	"bufio"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "basenc",
	Synopsis: "Encode or decode FILE, or standard input, to standard output. With no FILE, or when FILE is -, read standard input.",
	Usage:    "basenc [OPTION]... [FILE]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type encodingSpec struct {
	name       string
	newEncoder func(io.Writer) io.WriteCloser
	newDecoder func(io.Reader) io.Reader
	inAlphabet func(byte) bool
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	base64Flag := fs.Bool("base64", false, "same as the base64 program")
	base64URLFlag := fs.Bool("base64url", false, "file- and url-safe base64")
	base32Flag := fs.Bool("base32", false, "same as the base32 program")
	base32HexFlag := fs.Bool("base32hex", false, "extended hex alphabet base32")
	base16Flag := fs.Bool("base16", false, "hex encoding")
	decode := fs.BoolP("decode", "d", false, "decode data")
	ignore := fs.BoolP("ignore-garbage", "i", false, "when decoding, ignore non-alphabet characters")
	wrap := fs.StringP("wrap", "w", "76", "wrap encoded lines after COLS character (default 76); use 0 to disable line wrapping")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	cols, err := strconv.ParseInt(*wrap, 10, 64)
	if err != nil || cols < 0 {
		fmt.Fprintf(rc.Err, "basenc: invalid wrap size: '%s'\n", *wrap)
		return 1
	}
	specs := selectedEncodings(*base64Flag, *base64URLFlag, *base32Flag, *base32HexFlag, *base16Flag)
	if len(specs) == 0 {
		return tool.UsageError(rc, cmd, "missing encoding type")
	}
	if len(specs) > 1 {
		return tool.UsageError(rc, cmd, "multiple encoding types specified")
	}
	if len(operands) > 1 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[1])
	}
	name := "-"
	if len(operands) == 1 {
		name = operands[0]
	}
	src, closeFn, err := openInput(rc, name)
	if err != nil {
		fmt.Fprintf(rc.Err, "basenc: %s: %s\n", name, errMsg(err))
		return 1
	}
	if closeFn != nil {
		defer closeFn()
	}
	if *decode {
		return decodeBase(rc, specs[0], src, *ignore)
	}
	return encodeBase(rc, specs[0], src, cols)
}

func selectedEncodings(b64, b64URL, b32, b32Hex, b16 bool) []encodingSpec {
	var out []encodingSpec
	if b64 {
		out = append(out, encodingSpec{
			name: "base64",
			newEncoder: func(w io.Writer) io.WriteCloser {
				return base64.NewEncoder(base64.StdEncoding, w)
			},
			newDecoder: func(r io.Reader) io.Reader {
				return base64.NewDecoder(base64.StdEncoding, r)
			},
			inAlphabet: func(b byte) bool {
				return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '+' || b == '/'
			},
		})
	}
	if b64URL {
		out = append(out, encodingSpec{
			name: "base64url",
			newEncoder: func(w io.Writer) io.WriteCloser {
				return base64.NewEncoder(base64.URLEncoding, w)
			},
			newDecoder: func(r io.Reader) io.Reader {
				return base64.NewDecoder(base64.URLEncoding, r)
			},
			inAlphabet: func(b byte) bool {
				return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' || b == '_'
			},
		})
	}
	if b32 {
		out = append(out, encodingSpec{
			name: "base32",
			newEncoder: func(w io.Writer) io.WriteCloser {
				return base32.NewEncoder(base32.StdEncoding, w)
			},
			newDecoder: func(r io.Reader) io.Reader {
				return base32.NewDecoder(base32.StdEncoding, r)
			},
			inAlphabet: func(b byte) bool {
				return b >= 'A' && b <= 'Z' || b >= '2' && b <= '7'
			},
		})
	}
	if b32Hex {
		out = append(out, encodingSpec{
			name: "base32hex",
			newEncoder: func(w io.Writer) io.WriteCloser {
				return base32.NewEncoder(base32.HexEncoding, w)
			},
			newDecoder: func(r io.Reader) io.Reader {
				return base32.NewDecoder(base32.HexEncoding, r)
			},
			inAlphabet: func(b byte) bool {
				return b >= 'A' && b <= 'V' || b >= '0' && b <= '9'
			},
		})
	}
	if b16 {
		out = append(out, encodingSpec{
			name: "base16",
			newEncoder: func(w io.Writer) io.WriteCloser {
				return &upperHexEncoder{w: w}
			},
			newDecoder: func(r io.Reader) io.Reader {
				return hex.NewDecoder(r)
			},
			inAlphabet: func(b byte) bool {
				return b >= 'A' && b <= 'F' || b >= 'a' && b <= 'f' || b >= '0' && b <= '9'
			},
		})
	}
	return out
}

func openInput(rc *tool.RunContext, name string) (io.Reader, func() error, error) {
	if name == "-" {
		if rc.In == nil {
			return strings.NewReader(""), nil, nil
		}
		return rc.In, nil, nil
	}
	f, err := os.Open(rc.Path(name))
	if err != nil {
		return nil, nil, err
	}
	if fi, err := f.Stat(); err == nil && fi.IsDir() {
		_ = f.Close()
		return nil, nil, errIsDirectory
	}
	return f, f.Close, nil
}

func encodeBase(rc *tool.RunContext, spec encodingSpec, src io.Reader, cols int64) int {
	bw := bufio.NewWriterSize(rc.Out, 64<<10)
	ww := &wrapWriter{w: bw, cols: cols}
	enc := spec.newEncoder(ww)
	if _, err := io.Copy(enc, src); err != nil {
		fmt.Fprintf(rc.Err, "basenc: %v\n", err)
		return 1
	}
	if err := enc.Close(); err != nil {
		fmt.Fprintf(rc.Err, "basenc: %v\n", err)
		return 1
	}
	if err := ww.finish(); err != nil {
		fmt.Fprintf(rc.Err, "basenc: %v\n", err)
		return 1
	}
	if err := bw.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "basenc: %v\n", err)
		return 1
	}
	return 0
}

func decodeBase(rc *tool.RunContext, spec encodingSpec, src io.Reader, ignore bool) int {
	bw := bufio.NewWriterSize(rc.Out, 64<<10)
	fr := &filterReader{r: src, inAlphabet: spec.inAlphabet, ignore: ignore}
	dec := spec.newDecoder(fr)
	if _, err := io.Copy(bw, dec); err != nil {
		var b64 base64.CorruptInputError
		var b32 base32.CorruptInputError
		if errors.Is(err, errInvalidInput) || errors.Is(err, io.ErrUnexpectedEOF) ||
			errors.As(err, &b64) || errors.As(err, &b32) || strings.Contains(err.Error(), "invalid byte") {
			fmt.Fprintln(rc.Err, "basenc: invalid input")
		} else {
			fmt.Fprintf(rc.Err, "basenc: %v\n", err)
		}
		return 1
	}
	if err := bw.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "basenc: %v\n", err)
		return 1
	}
	return 0
}

type upperHexEncoder struct {
	w io.Writer
}

func (e *upperHexEncoder) Write(p []byte) (int, error) {
	const digits = "0123456789ABCDEF"
	buf := make([]byte, len(p)*2)
	for i, b := range p {
		buf[i*2] = digits[b>>4]
		buf[i*2+1] = digits[b&0x0f]
	}
	_, err := e.w.Write(buf)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (e *upperHexEncoder) Close() error { return nil }

var errInvalidInput = errors.New("invalid input")

type filterReader struct {
	r          io.Reader
	inAlphabet func(byte) bool
	ignore     bool
	buf        [4096]byte
}

func (fr *filterReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	max := len(p)
	if max > len(fr.buf) {
		max = len(fr.buf)
	}
	for {
		n, err := fr.r.Read(fr.buf[:max])
		out := 0
		for _, b := range fr.buf[:n] {
			switch {
			case fr.inAlphabet(b) || b == '=':
				p[out] = b
				out++
			case fr.ignore || b == '\n':
			default:
				return out, errInvalidInput
			}
		}
		if out > 0 || err != nil {
			return out, err
		}
	}
}

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

var errIsDirectory = fmt.Errorf("Is a directory")

func errMsg(err error) string {
	if err == errIsDirectory {
		return err.Error()
	}
	if os.IsNotExist(err) {
		return "No such file or directory"
	}
	if os.IsPermission(err) {
		return "Permission denied"
	}
	return err.Error()
}
