// Package ddcmd implements a practical dd(1) subset: copy bytes from
// input to output using dd-style KEY=VALUE operands.
//
// POSIX/GNU block semantics: seek= preserves the skipped-over output
// blocks, and (unless conv=notrunc) the output file is truncated at the
// seek offset before copying. When ibs=/obs= are given, output is
// re-blocked into obs-sized records; bs= disables re-blocking (each
// input block is written as read), exactly as GNU documents.
//
// Documented deviation: the default status trailer is a plain
// "N bytes copied" line — GNU appends wall-clock time and throughput,
// which this repo omits for deterministic output.
package ddcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "dd",
	Synopsis: "Copy a file, converting and formatting according to operands.",
	Usage:    "dd [OPERAND]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type config struct {
	ifile, ofile string
	ibs, obs     int64
	cbs          int64
	count        int64
	skip, seek   int64
	notrunc      bool
	noerror      bool
	fullblock    bool
	sync         bool
	block        bool
	unblock      bool
	lcase        bool
	ucase        bool
	swab         bool
	status       string
	reblock      bool
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	cfg := config{ibs: 512, obs: 512, count: -1, reblock: true}
	var bs int64
	for _, op := range operands {
		k, v, ok := strings.Cut(op, "=")
		if !ok || k == "" {
			return tool.UsageError(rc, cmd, "unrecognized operand '%s'", op)
		}
		switch k {
		case "if":
			cfg.ifile = v
		case "of":
			cfg.ofile = v
		case "bs":
			n, err := parseBytes(v)
			if err != nil || n <= 0 {
				return tool.UsageError(rc, cmd, "invalid number: '%s'", v)
			}
			bs = n
		case "ibs":
			n, err := parseBytes(v)
			if err != nil || n <= 0 {
				return tool.UsageError(rc, cmd, "invalid number: '%s'", v)
			}
			cfg.ibs = n
			cfg.reblock = true
		case "obs":
			n, err := parseBytes(v)
			if err != nil || n <= 0 {
				return tool.UsageError(rc, cmd, "invalid number: '%s'", v)
			}
			cfg.obs = n
			cfg.reblock = true
		case "cbs":
			n, err := parseBytes(v)
			if err != nil || n <= 0 {
				return tool.UsageError(rc, cmd, "invalid number: '%s'", v)
			}
			cfg.cbs = n
		case "count":
			n, err := parseCount(v)
			if err != nil {
				return tool.UsageError(rc, cmd, "invalid number: '%s'", v)
			}
			cfg.count = n
		case "skip":
			n, err := parseCount(v)
			if err != nil {
				return tool.UsageError(rc, cmd, "invalid number: '%s'", v)
			}
			cfg.skip = n
		case "seek":
			n, err := parseCount(v)
			if err != nil {
				return tool.UsageError(rc, cmd, "invalid number: '%s'", v)
			}
			cfg.seek = n
		case "status":
			if v != "none" && v != "noxfer" {
				return tool.NotSupported(rc, cmd, "status="+v)
			}
			cfg.status = v
		case "iflag":
			for _, flag := range strings.Split(v, ",") {
				switch flag {
				case "fullblock":
					cfg.fullblock = true
				default:
					return tool.NotSupported(rc, cmd, "iflag="+flag)
				}
			}
		case "conv":
			for _, conversion := range strings.Split(v, ",") {
				switch conversion {
				case "notrunc":
					cfg.notrunc = true
				case "noerror":
					cfg.noerror = true
				case "sync":
					cfg.sync = true
				case "block":
					cfg.block = true
				case "unblock":
					cfg.unblock = true
				case "lcase":
					cfg.lcase = true
				case "ucase":
					cfg.ucase = true
				case "swab":
					cfg.swab = true
				default:
					return tool.NotSupported(rc, cmd, "conv="+conversion)
				}
			}
		default:
			return tool.UsageError(rc, cmd, "unrecognized operand '%s'", op)
		}
	}
	if cfg.block && cfg.unblock {
		return tool.UsageError(rc, cmd, "conv=block and conv=unblock are mutually exclusive")
	}
	if cfg.lcase && cfg.ucase {
		return tool.UsageError(rc, cmd, "conv=lcase and conv=ucase are mutually exclusive")
	}
	if (cfg.block || cfg.unblock) && cfg.cbs == 0 {
		return tool.UsageError(rc, cmd, "cbs= is required with conv=block or conv=unblock")
	}
	if bs > 0 {
		// bs= takes precedence over ibs=/obs= regardless of operand order.
		// With the currently supported conversions, GNU writes each input
		// block as read rather than aggregating short blocks.
		cfg.ibs, cfg.obs = bs, bs
		// POSIX permits bs= to bypass output reblocking only when no data
		// conversion is requested. sync, noerror, and notrunc are file/input
		// handling conversions, while the following alter the byte stream.
		cfg.reblock = cfg.block || cfg.unblock || cfg.lcase || cfg.ucase || cfg.swab
	}
	return copyDD(rc, cfg)
}

func copyDD(rc *tool.RunContext, cfg config) int {
	var in io.Reader = rc.In
	var inf *os.File
	inputFIFO := false
	if cfg.ifile != "" {
		f, err := os.Open(rc.Path(cfg.ifile))
		if err != nil {
			fmt.Fprintf(rc.Err, "dd: failed to open '%s': %v\n", cfg.ifile, reason(err))
			return 1
		}
		defer f.Close()
		inf = f
		in = f
		if fi, err := f.Stat(); err == nil {
			inputFIFO = fi.Mode()&os.ModeNamedPipe != 0
		}
	}
	var out io.Writer = rc.Out
	var outf *os.File
	outputFIFO := false
	var seekBytes int64
	if cfg.seek > 0 {
		var ok bool
		seekBytes, ok = multiplyBlocks(cfg.seek, cfg.obs)
		if !ok {
			fmt.Fprintln(rc.Err, "dd: seek is too large")
			return 2
		}
	}
	if cfg.ofile != "" {
		path := rc.Path(cfg.ofile)
		if fi, err := os.Stat(path); err == nil {
			outputFIFO = fi.Mode()&os.ModeNamedPipe != 0
		}
		// A seek on a FIFO is simulated by reading and discarding output
		// blocks. Opening it write-only first would deadlock with the writer
		// which supplies those blocks. For count=0 no write-side open is
		// needed after the simulated seek.
		if outputFIFO && cfg.seek > 0 {
			f, err := os.Open(path)
			if err != nil {
				fmt.Fprintf(rc.Err, "dd: failed to open '%s': %v\n", cfg.ofile, reason(err))
				return 1
			}
			_, err = io.CopyN(io.Discard, f, seekBytes)
			closeErr := f.Close()
			if err != nil && !errors.Is(err, io.EOF) {
				fmt.Fprintf(rc.Err, "dd: failed to seek '%s': %v\n", cfg.ofile, reason(err))
				return 1
			}
			if closeErr != nil {
				fmt.Fprintf(rc.Err, "dd: error closing '%s': %v\n", cfg.ofile, reason(closeErr))
				return 1
			}
		}
		if outputFIFO && cfg.count == 0 {
			out = io.Discard
		} else {
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o666)
			if err != nil {
				fmt.Fprintf(rc.Err, "dd: failed to open '%s': %v\n", cfg.ofile, reason(err))
				return 1
			}
			outf = f
			out = f
		}
		if outf != nil && !outputFIFO && !cfg.notrunc {
			// POSIX: truncate at the seek offset, preserving the blocks
			// dd seeks over. Truncate can fail on special files
			// (/dev/null); GNU ignores that, so only surface the error
			// for regular files where it would mean silent stale data.
			if err := outf.Truncate(seekBytes); err != nil {
				if fi, serr := outf.Stat(); serr == nil && fi.Mode().IsRegular() {
					fmt.Fprintf(rc.Err, "dd: failed to truncate '%s': %v\n", cfg.ofile, reason(err))
					return 1
				}
			}
		}
	}
	if cfg.skip > 0 {
		n, ok := multiplyBlocks(cfg.skip, cfg.ibs)
		if !ok {
			fmt.Fprintln(rc.Err, "dd: skip is too large")
			return 2
		}
		if inf != nil && !inputFIFO {
			if _, err := inf.Seek(n, io.SeekStart); err != nil {
				fmt.Fprintf(rc.Err, "dd: failed to skip '%s': %v\n", cfg.ifile, reason(err))
				return 1
			}
		} else if _, err := io.CopyN(io.Discard, in, n); err != nil && !errors.Is(err, io.EOF) {
			fmt.Fprintf(rc.Err, "dd: error skipping input: %v\n", reason(err))
			return 1
		}
	}
	if cfg.seek > 0 {
		if outf == nil && !outputFIFO {
			return tool.NotSupported(rc, cmd, "seek= with standard output")
		}
		if outputFIFO {
			// The offset was consumed from the FIFO before its write side was
			// opened; FIFOs themselves are not seekable.
		} else if _, err := outf.Seek(seekBytes, io.SeekStart); err != nil {
			fmt.Fprintf(rc.Err, "dd: failed to seek '%s': %v\n", cfg.ofile, reason(err))
			return 1
		}
	}

	counter := &countWriter{w: out}
	out = counter
	var blocker *obsWriter
	if cfg.reblock {
		blocker = &obsWriter{w: out, obs: cfg.obs}
		out = blocker
	}
	var blockConv *blockWriter
	var unblockConv *unblockWriter
	if cfg.block || cfg.unblock {
		conversionBuf, err := makeBuffer(cfg.cbs)
		if err != nil {
			fmt.Fprintf(rc.Err, "dd: conversion buffer: %v\n", err)
			return 1
		}
		if cfg.block {
			blockConv = &blockWriter{w: out, record: conversionBuf[:0], width: len(conversionBuf)}
			out = blockConv
		} else {
			unblockConv = &unblockWriter{w: out, record: conversionBuf[:0], width: len(conversionBuf)}
			out = unblockConv
		}
	}
	buf, err := makeBuffer(cfg.ibs)
	if err != nil {
		fmt.Fprintf(rc.Err, "dd: input buffer: %v\n", err)
		return 1
	}
	var full, partial int64
	var outFull, outPartial int64
	var hadReadError bool
	for cfg.count < 0 || full+partial < cfg.count {
		n, rerr := readInputBlock(in, buf, cfg.fullblock)
		if n > 0 {
			if int64(n) == cfg.ibs {
				full++
			} else {
				partial++
			}
			if cfg.sync && int64(n) < cfg.ibs {
				// POSIX conv=sync pads each input record before any output
				// reblocking. The normal fill byte is NUL; block/unblock
				// select a space.
				padInputBlock(buf, n, cfg)
				n = len(buf)
			}
			data := buf[:n]
			convertBytes(data, cfg)
			if err := writeAll(out, data); err != nil {
				fmt.Fprintf(rc.Err, "dd: error writing output: %v\n", reason(err))
				return 1
			}
			if blocker == nil {
				if int64(n) == cfg.obs {
					outFull++
				} else {
					outPartial++
				}
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			fmt.Fprintf(rc.Err, "dd: error reading input: %v\n", reason(rerr))
			if !cfg.noerror {
				return 1
			}
			hadReadError = true
			if n == 0 {
				if cfg.sync {
					padInputBlock(buf, 0, cfg)
					convertBytes(buf, cfg)
					if err := writeAll(out, buf); err != nil {
						fmt.Fprintf(rc.Err, "dd: error writing output: %v\n", reason(err))
						return 1
					}
					if blocker == nil {
						outFull++
					}
				}
				// A Reader that reports an error without data has made no
				// progress. Regular input files can advance to the next input
				// record; for streams there is no portable recovery operation,
				// so stop after the required diagnostic rather than spin forever
				// or discard arbitrary later bytes.
				if inf != nil && !inputFIFO {
					if _, err := inf.Seek(cfg.ibs, io.SeekCurrent); err != nil {
						return 1
					}
					if cfg.sync {
						full++
					}
					continue
				}
				break
			}
			// A Reader may return data and an error together. The bytes above
			// were valid input; retry once more to continue after the error.
			if n > 0 {
				continue
			}
			break
		}
	}
	if blockConv != nil {
		if err := blockConv.Flush(); err != nil {
			fmt.Fprintf(rc.Err, "dd: error writing output: %v\n", reason(err))
			return 1
		}
	}
	if unblockConv != nil {
		if err := unblockConv.Flush(); err != nil {
			fmt.Fprintf(rc.Err, "dd: error writing output: %v\n", reason(err))
			return 1
		}
	}
	if blocker != nil {
		if err := blocker.Flush(); err != nil {
			fmt.Fprintf(rc.Err, "dd: error writing output: %v\n", reason(err))
			return 1
		}
		outFull, outPartial = blocker.full, blocker.partial
	}
	if outf != nil {
		if err := outf.Close(); err != nil {
			fmt.Fprintf(rc.Err, "dd: error closing '%s': %v\n", cfg.ofile, reason(err))
			return 1
		}
		outf = nil
	}
	if cfg.status == "none" {
		if hadReadError {
			return 1
		}
		return 0
	}
	fmt.Fprintf(rc.Err, "%d+%d records in\n", full, partial)
	fmt.Fprintf(rc.Err, "%d+%d records out\n", outFull, outPartial)
	if blockConv != nil && blockConv.truncated > 0 {
		suffix := "s"
		if blockConv.truncated == 1 {
			suffix = ""
		}
		fmt.Fprintf(rc.Err, "%d truncated record%s\n", blockConv.truncated, suffix)
	}
	if cfg.status != "noxfer" {
		fmt.Fprintf(rc.Err, "%d bytes copied\n", counter.n)
	}
	if hadReadError {
		return 1
	}
	return 0
}

func padInputBlock(buf []byte, n int, cfg config) {
	if cfg.block || cfg.unblock {
		for i := n; i < len(buf); i++ {
			buf[i] = ' '
		}
	} else {
		clear(buf[n:])
	}
}

func convertBytes(p []byte, cfg config) {
	if cfg.swab {
		swabBytes(p)
	}
	if cfg.lcase {
		lowercaseBytes(p)
	} else if cfg.ucase {
		uppercaseBytes(p)
	}
}

// swabBytes swaps adjacent bytes. GNU dd preserves an odd trailing byte.
func swabBytes(p []byte) {
	for i := 0; i+1 < len(p); i += 2 {
		p[i], p[i+1] = p[i+1], p[i]
	}
}

// GNU dd's lcase and ucase conversions are single-byte conversions. Keep the
// mapping explicit and locale-independent, as POSIX leaves multibyte behavior
// unspecified and this command operates on byte streams.
func lowercaseBytes(p []byte) {
	for i, c := range p {
		if c >= 'A' && c <= 'Z' {
			p[i] = c + ('a' - 'A')
		}
	}
}

func uppercaseBytes(p []byte) {
	for i, c := range p {
		if c >= 'a' && c <= 'z' {
			p[i] = c - ('a' - 'A')
		}
	}
}

// readInputBlock implements GNU iflag=fullblock. A normal read is one input
// record even when the underlying reader returns a short read. With fullblock,
// short reads are accumulated until ibs bytes have been read or EOF is seen.
func readInputBlock(in io.Reader, buf []byte, fullblock bool) (int, error) {
	if !fullblock {
		return in.Read(buf)
	}
	n, err := io.ReadFull(in, buf)
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return n, io.EOF
	}
	return n, err
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// blockWriter converts newline-terminated variable-length records to fixed
// width, space-padded records. Bytes beyond the conversion width are counted
// as one truncated record and discarded through the next newline.
type blockWriter struct {
	w         io.Writer
	record    []byte
	width     int
	overflow  bool
	truncated int64
}

func (b *blockWriter) Write(p []byte) (int, error) {
	for _, c := range p {
		if c == '\n' {
			if err := b.emit(); err != nil {
				return 0, err
			}
			continue
		}
		if len(b.record) < b.width {
			b.record = append(b.record, c)
		} else {
			b.overflow = true
		}
	}
	return len(p), nil
}

func (b *blockWriter) emit() error {
	for len(b.record) < b.width {
		b.record = append(b.record, ' ')
	}
	if err := writeAll(b.w, b.record); err != nil {
		return err
	}
	if b.overflow {
		b.truncated++
	}
	b.record = b.record[:0]
	b.overflow = false
	return nil
}

func (b *blockWriter) Flush() error {
	if len(b.record) == 0 && !b.overflow {
		return nil
	}
	return b.emit()
}

// unblockWriter converts fixed-width records to newline-terminated records,
// removing trailing spaces from each record.
type unblockWriter struct {
	w      io.Writer
	record []byte
	width  int
}

func (u *unblockWriter) Write(p []byte) (int, error) {
	for _, c := range p {
		u.record = append(u.record, c)
		if len(u.record) == u.width {
			if err := u.emit(); err != nil {
				return 0, err
			}
		}
	}
	return len(p), nil
}

func (u *unblockWriter) emit() error {
	end := len(u.record)
	for end > 0 && u.record[end-1] == ' ' {
		end--
	}
	if err := writeAll(u.w, u.record[:end]); err != nil {
		return err
	}
	if err := writeAll(u.w, []byte{'\n'}); err != nil {
		return err
	}
	u.record = u.record[:0]
	return nil
}

func (u *unblockWriter) Flush() error {
	if len(u.record) == 0 {
		return nil
	}
	return u.emit()
}

// obsWriter re-blocks writes into obs-sized output records, counting
// full and partial records the way GNU dd reports them.
type obsWriter struct {
	w             io.Writer
	obs           int64
	buf           []byte
	full, partial int64
}

func (o *obsWriter) Write(p []byte) (int, error) {
	total := len(p)
	for len(p) > 0 {
		if len(o.buf) == 0 && int64(len(p)) >= o.obs {
			if err := writeAll(o.w, p[:o.obs]); err != nil {
				return 0, err
			}
			o.full++
			p = p[o.obs:]
			continue
		}
		n := min(int(o.obs)-len(o.buf), len(p))
		o.buf = append(o.buf, p[:n]...)
		p = p[n:]
		if int64(len(o.buf)) == o.obs {
			if err := writeAll(o.w, o.buf); err != nil {
				return 0, err
			}
			o.full++
			o.buf = o.buf[:0]
		}
	}
	return total, nil
}

func (o *obsWriter) Flush() error {
	if len(o.buf) == 0 {
		return nil
	}
	if err := writeAll(o.w, o.buf); err != nil {
		return err
	}
	o.partial++
	o.buf = o.buf[:0]
	return nil
}

var byteMultipliers = map[string]int64{
	"":    1,
	"c":   1,
	"w":   2,
	"b":   512,
	"kB":  1000,
	"K":   1024,
	"KB":  1000,
	"M":   1024 * 1024,
	"MB":  1000 * 1000,
	"G":   1024 * 1024 * 1024,
	"GB":  1000 * 1000 * 1000,
	"KiB": 1024,
	"MiB": 1024 * 1024,
	"GiB": 1024 * 1024 * 1024,
}

func parseBytes(s string) (int64, error) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, strconv.ErrSyntax
	}
	n, err := strconv.ParseInt(s[:i], 10, 64)
	if err != nil {
		return 0, err
	}
	m, ok := byteMultipliers[s[i:]]
	if !ok {
		return 0, strconv.ErrSyntax
	}
	if m != 0 && n > int64(^uint64(0)>>1)/m {
		return 0, strconv.ErrRange
	}
	return n * m, nil
}

func parseCount(s string) (int64, error) {
	n, err := parseBytes(s)
	if err == nil {
		return n, nil
	}
	n, err = strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		if err == nil {
			err = strconv.ErrRange
		}
		return 0, err
	}
	return n, nil
}

func multiplyBlocks(blocks, size int64) (int64, bool) {
	if blocks < 0 || size < 0 || (size != 0 && blocks > int64(^uint64(0)>>1)/size) {
		return 0, false
	}
	return blocks * size, true
}

func makeBuffer(size int64) (buf []byte, err error) {
	if size <= 0 || uint64(size) > uint64(^uint(0)>>1) {
		return nil, strconv.ErrRange
	}
	defer func() {
		if recover() != nil {
			buf = nil
			err = errors.New("cannot allocate memory")
		}
	}()
	return make([]byte, int(size)), nil
}

func writeAll(w io.Writer, p []byte) error {
	n, err := w.Write(p)
	if err != nil {
		return err
	}
	if n != len(p) {
		return io.ErrShortWrite
	}
	return nil
}

func reason(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
