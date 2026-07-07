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

	"github.com/qiangli/coreutils/cmds/internal/hashenc"
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

// encodingNames lists the encoding-selection flags in registration
// order; when several are given, GNU uses the last one on the command
// line.
var encodingNames = []string{
	"base64", "base64url", "base32", "base32hex", "base16",
	"base2lsbf", "base2msbf", "z85", "base58",
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	fs.Bool("base64", false, "same as the base64 program")
	fs.Bool("base64url", false, "file- and url-safe base64")
	fs.Bool("base32", false, "same as the base32 program")
	fs.Bool("base32hex", false, "extended hex alphabet base32")
	fs.Bool("base16", false, "hex encoding")
	fs.Bool("base2lsbf", false, "bit string with least significant bit first")
	fs.Bool("base2msbf", false, "bit string with most significant bit first")
	fs.Bool("z85", false, "ascii85-like encoding")
	fs.Bool("base58", false, "visually unambiguous base58 encoding")
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
	// GNU takes the LAST encoding flag given, so recover the command-
	// line order by scanning the raw args (each selector is an exact
	// long flag with no argument).
	selected := ""
	for _, a := range args {
		if a == "--" {
			break
		}
		for _, n := range encodingNames {
			if a == "--"+n {
				selected = n
			}
		}
	}
	if selected == "" {
		// Fallback for spellings the scan doesn't see (--base64=true).
		for _, n := range encodingNames {
			if v, _ := fs.GetBool(n); v {
				selected = n
			}
		}
	}
	if selected == "" {
		return tool.UsageError(rc, cmd, "missing encoding type")
	}
	spec := encodingSpecFor(selected)
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
		return decodeBase(rc, spec, src, *ignore)
	}
	return encodeBase(rc, spec, src, cols)
}

func encodingSpecFor(name string) encodingSpec {
	switch name {
	case "base64":
		return encodingSpec{
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
		}
	case "base64url":
		return encodingSpec{
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
		}
	case "base32":
		return encodingSpec{
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
		}
	case "base32hex":
		return encodingSpec{
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
		}
	case "base16":
		return encodingSpec{
			name: "base16",
			newEncoder: func(w io.Writer) io.WriteCloser {
				return &upperHexEncoder{w: w}
			},
			inAlphabet: func(b byte) bool {
				return b >= 'A' && b <= 'F' || b >= 'a' && b <= 'f' || b >= '0' && b <= '9'
			},
			decodeAll: decodeBase16,
		}
	case "base2lsbf":
		return base2Spec("base2lsbf", true)
	case "base2msbf":
		return base2Spec("base2msbf", false)
	case "z85":
		return encodingSpec{
			name:       "z85",
			inAlphabet: isZ85Byte,
			encodeAll:  encodeZ85,
			decodeAll:  decodeZ85,
		}
	case "base58":
		return encodingSpec{
			name:       "base58",
			inAlphabet: isBase58Byte,
			encodeAll:  encodeBase58,
			decodeAll:  decodeBase58,
		}
	}
	panic("basenc: unknown encoding " + name)
}

// decodeBase16 decodes hex input. '=' is never valid base16 (it is
// dropped by -i like any other garbage byte); an odd number of hex
// digits at EOF is invalid input.
func decodeBase16(data []byte, ignore bool) ([]byte, error) {
	encoded := filterBytes(data, func(b byte) bool {
		return b >= 'A' && b <= 'F' || b >= 'a' && b <= 'f' || b >= '0' && b <= '9'
	}, ignore)
	if encoded == nil || len(encoded)%2 != 0 {
		return nil, errInvalidInput
	}
	out := make([]byte, hex.DecodedLen(len(encoded)))
	if _, err := hex.Decode(out, encoded); err != nil {
		return nil, errInvalidInput
	}
	return out, nil
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
	data, err := io.ReadAll(src)
	if err != nil {
		fmt.Fprintf(rc.Err, "basenc: %v\n", err)
		return 1
	}
	var decoded []byte
	if spec.decodeAll != nil {
		decoded, err = spec.decodeAll(data, ignore)
	} else {
		// RFC 4648 padded encodings share the GNU >= 9.5 decode rules
		// (auto-pad at EOF, reject non-zero padding bits) with the
		// standalone base64/base32 tools.
		decoded, err = hashenc.DecodeBase(data, spec.inAlphabet, ignore, spec.newEncoder, spec.newDecoder)
	}
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
		return nil, fmt.Errorf("invalid input (length must be multiple of 4 characters)")
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
