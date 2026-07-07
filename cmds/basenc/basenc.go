// Package basenccmd implements basenc(1) for common RFC 4648 encodings.
package basenccmd

import (
	"bufio"
	"bytes"
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
	encodeAll  func([]byte) ([]byte, error)
	decodeAll  func([]byte, bool) ([]byte, error)
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	base64Flag := fs.Bool("base64", false, "same as the base64 program")
	base64URLFlag := fs.Bool("base64url", false, "file- and url-safe base64")
	base32Flag := fs.Bool("base32", false, "same as the base32 program")
	base32HexFlag := fs.Bool("base32hex", false, "extended hex alphabet base32")
	base16Flag := fs.Bool("base16", false, "hex encoding")
	base2LSBFlag := fs.Bool("base2lsbf", false, "bit string with least significant bit first")
	base2MSBFlag := fs.Bool("base2msbf", false, "bit string with most significant bit first")
	z85Flag := fs.Bool("z85", false, "ascii85-like encoding")
	base58Flag := fs.Bool("base58", false, "visually unambiguous base58 encoding")
	decode := fs.BoolP("decode", "d", false, "decode data")
	decodeAlias := fs.BoolP("decode-alias", "D", false, "decode data")
	_ = fs.MarkHidden("decode-alias")
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
	specs := selectedEncodings(*base64Flag, *base64URLFlag, *base32Flag, *base32HexFlag, *base16Flag, *base2LSBFlag, *base2MSBFlag, *z85Flag, *base58Flag)
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
	if *decode || *decodeAlias {
		return decodeBase(rc, specs[0], src, *ignore)
	}
	return encodeBase(rc, specs[0], src, cols)
}

func selectedEncodings(b64, b64URL, b32, b32Hex, b16, b2LSB, b2MSB, z85, b58 bool) []encodingSpec {
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
	if b2LSB {
		out = append(out, base2Spec("base2lsbf", true))
	}
	if b2MSB {
		out = append(out, base2Spec("base2msbf", false))
	}
	if z85 {
		out = append(out, encodingSpec{
			name:       "z85",
			inAlphabet: isZ85Byte,
			encodeAll:  encodeZ85,
			decodeAll:  decodeZ85,
		})
	}
	if b58 {
		out = append(out, encodingSpec{
			name:       "base58",
			inAlphabet: isBase58Byte,
			encodeAll:  encodeBase58,
			decodeAll:  decodeBase58,
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
	if spec.encodeAll != nil {
		data, err := io.ReadAll(src)
		if err != nil {
			fmt.Fprintf(rc.Err, "basenc: %v\n", err)
			return 1
		}
		encoded, err := spec.encodeAll(data)
		if err != nil {
			fmt.Fprintf(rc.Err, "basenc: %v\n", err)
			return 1
		}
		return writeWrapped(rc, encoded, cols)
	}
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
	if spec.decodeAll != nil {
		data, err := io.ReadAll(src)
		if err != nil {
			fmt.Fprintf(rc.Err, "basenc: %v\n", err)
			return 1
		}
		decoded, err := spec.decodeAll(data, ignore)
		if err != nil {
			fmt.Fprintln(rc.Err, "basenc: invalid input")
			return 1
		}
		if _, err := rc.Out.Write(decoded); err != nil {
			fmt.Fprintf(rc.Err, "basenc: %v\n", err)
			return 1
		}
		return 0
	}
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

func writeWrapped(rc *tool.RunContext, data []byte, cols int64) int {
	bw := bufio.NewWriterSize(rc.Out, 64<<10)
	ww := &wrapWriter{w: bw, cols: cols}
	if _, err := ww.Write(data); err != nil {
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

func base2Spec(name string, lsbFirst bool) encodingSpec {
	return encodingSpec{
		name:       name,
		inAlphabet: func(b byte) bool { return b == '0' || b == '1' },
		encodeAll: func(data []byte) ([]byte, error) {
			out := make([]byte, 0, len(data)*8)
			for _, b := range data {
				for i := 0; i < 8; i++ {
					shift := 7 - i
					if lsbFirst {
						shift = i
					}
					if b&(1<<shift) != 0 {
						out = append(out, '1')
					} else {
						out = append(out, '0')
					}
				}
			}
			return out, nil
		},
		decodeAll: func(data []byte, ignore bool) ([]byte, error) {
			bits := filterBytes(data, func(b byte) bool { return b == '0' || b == '1' }, ignore)
			if bits == nil || len(bits)%8 != 0 {
				return nil, errInvalidInput
			}
			out := make([]byte, len(bits)/8)
			for i := range out {
				var b byte
				for j := 0; j < 8; j++ {
					if bits[i*8+j] == '1' {
						shift := 7 - j
						if lsbFirst {
							shift = j
						}
						b |= 1 << shift
					}
				}
				out[i] = b
			}
			return out, nil
		},
	}
}

const z85Alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.-:+=^!/*?&<>()[]{}@%$#"

func isZ85Byte(b byte) bool {
	return strings.IndexByte(z85Alphabet, b) >= 0
}

func encodeZ85(data []byte) ([]byte, error) {
	if len(data)%4 != 0 {
		return nil, fmt.Errorf("z85 input length must be a multiple of 4")
	}
	out := make([]byte, 0, len(data)/4*5)
	for len(data) > 0 {
		value := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
		var block [5]byte
		for i := 4; i >= 0; i-- {
			block[i] = z85Alphabet[value%85]
			value /= 85
		}
		out = append(out, block[:]...)
		data = data[4:]
	}
	return out, nil
}

func decodeZ85(data []byte, ignore bool) ([]byte, error) {
	encoded := filterBytes(data, isZ85Byte, ignore)
	if encoded == nil || len(encoded)%5 != 0 {
		return nil, errInvalidInput
	}
	out := make([]byte, 0, len(encoded)/5*4)
	for len(encoded) > 0 {
		var value uint32
		for _, b := range encoded[:5] {
			idx := strings.IndexByte(z85Alphabet, b)
			if idx < 0 {
				return nil, errInvalidInput
			}
			value = value*85 + uint32(idx)
		}
		out = append(out, byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
		encoded = encoded[5:]
	}
	return out, nil
}

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func isBase58Byte(b byte) bool {
	return strings.IndexByte(base58Alphabet, b) >= 0
}

func encodeBase58(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	zeros := 0
	for zeros < len(data) && data[zeros] == 0 {
		zeros++
	}
	num := bytes.TrimLeft(data, "\x00")
	digits := []byte{0}
	for _, b := range num {
		carry := int(b)
		for i := len(digits) - 1; i >= 0; i-- {
			carry += int(digits[i]) << 8
			digits[i] = byte(carry % 58)
			carry /= 58
		}
		for carry > 0 {
			digits = append([]byte{byte(carry % 58)}, digits...)
			carry /= 58
		}
	}
	out := bytes.Repeat([]byte{'1'}, zeros)
	for _, d := range digits {
		if len(num) == 0 && d == 0 {
			continue
		}
		out = append(out, base58Alphabet[d])
	}
	return out, nil
}

func decodeBase58(data []byte, ignore bool) ([]byte, error) {
	encoded := filterBytes(data, isBase58Byte, ignore)
	if encoded == nil {
		return nil, errInvalidInput
	}
	if len(encoded) == 0 {
		return nil, nil
	}
	zeros := 0
	for zeros < len(encoded) && encoded[zeros] == '1' {
		zeros++
	}
	bytesOut := []byte{0}
	for _, ch := range encoded[zeros:] {
		idx := strings.IndexByte(base58Alphabet, ch)
		if idx < 0 {
			return nil, errInvalidInput
		}
		carry := idx
		for i := len(bytesOut) - 1; i >= 0; i-- {
			carry += int(bytesOut[i]) * 58
			bytesOut[i] = byte(carry)
			carry >>= 8
		}
		for carry > 0 {
			bytesOut = append([]byte{byte(carry)}, bytesOut...)
			carry >>= 8
		}
	}
	out := bytes.Repeat([]byte{0}, zeros)
	if len(encoded) != zeros {
		out = append(out, bytesOut...)
	}
	return out, nil
}

func filterBytes(data []byte, inAlphabet func(byte) bool, ignore bool) []byte {
	out := make([]byte, 0, len(data))
	for _, b := range data {
		switch {
		case inAlphabet(b):
			out = append(out, b)
		case ignore || b == '\n':
		default:
			return nil
		}
	}
	return out
}

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
