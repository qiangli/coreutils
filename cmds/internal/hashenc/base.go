// Portions adapted from https://github.com/guonaihong/coreutils
// basecore/basecore.go (Apache-2.0) and https://github.com/u-root/u-root
// cmds/core/base64 (BSD-3-Clause).
// Changes: rewired to the tool framework (RunContext stdio/cwd, strict
// flag layer); wrap writer and decode filtering rewritten to match GNU
// exactly (default 76-column wrap with trailing newline, -w 0 emits no
// trailing newline, decoder tolerates embedded newlines only, -i drops
// every non-alphabet byte, "invalid input" diagnostic + exit 1).

package hashenc

import (
	"encoding/base32"
	"encoding/base64"
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
		Usage:    fmt.Sprintf("%s [OPTION]... [FILE]", spec.Name),
	}
	t.Run = func(rc *tool.RunContext, args []string) int {
		return runBase(rc, t, spec, args)
	}
	return t
}

func runBase(rc *tool.RunContext, t *tool.Tool, spec BaseSpec, args []string) int {
	fs := tool.NewFlags(t.Name)
	decode := fs.BoolP("decode", "d", false, "decode data")
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

	if *decode {
		// -w is accepted but has no effect when decoding (GNU).
		return runBaseDecode(rc, t, spec, src, *ignore)
	}
	return runBaseEncode(rc, t, src, spec, cols)
}

func runBaseEncode(rc *tool.RunContext, t *tool.Tool, src io.Reader, spec BaseSpec, cols int64) int {
	ww := &wrapWriter{w: rc.Out, cols: cols}
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
	return 0
}

func runBaseDecode(rc *tool.RunContext, t *tool.Tool, spec BaseSpec, src io.Reader, ignoreGarbage bool) int {
	fr := &filterReader{r: src, inAlphabet: spec.InAlphabet, ignore: ignoreGarbage}
	dec := spec.NewDecoder(fr)
	if _, err := io.Copy(rc.Out, dec); err != nil {
		var b64 base64.CorruptInputError
		var b32 base32.CorruptInputError
		if errors.Is(err, errInvalidInput) || errors.Is(err, io.ErrUnexpectedEOF) ||
			errors.As(err, &b64) || errors.As(err, &b32) {
			fmt.Fprintf(rc.Err, "%s: invalid input\n", t.Name)
		} else {
			fmt.Fprintf(rc.Err, "%s: %v\n", t.Name, err)
		}
		return 1
	}
	return 0
}

// errInvalidInput is the sentinel for garbage bytes found while
// decoding without -i.
var errInvalidInput = errors.New("invalid input")

// filterReader prepares the encoded stream for the stdlib decoder.
// Without -i it passes alphabet bytes and padding, silently drops
// newlines (GNU decodes its own wrapped output), and fails on
// anything else. With -i it drops every byte outside alphabet+padding.
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
				// dropped
			default:
				return out, errInvalidInput
			}
		}
		if out > 0 || err != nil {
			return out, err
		}
		// Everything in this chunk was filtered out; read more.
	}
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
