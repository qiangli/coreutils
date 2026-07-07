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
	count        int64
	skip, seek   int64
	notrunc      bool
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
			cfg.ibs, cfg.obs = n, n
			// bs= writes each input block as read — no re-blocking (GNU).
			cfg.reblock = false
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
		case "conv":
			if v == "notrunc" {
				cfg.notrunc = true
				continue
			}
			return tool.NotSupported(rc, cmd, "conv="+v)
		default:
			return tool.UsageError(rc, cmd, "unrecognized operand '%s'", op)
		}
	}
	return copyDD(rc, cfg)
}

func copyDD(rc *tool.RunContext, cfg config) int {
	var in io.Reader = rc.In
	var inf *os.File
	if cfg.ifile != "" {
		f, err := os.Open(rc.Path(cfg.ifile))
		if err != nil {
			fmt.Fprintf(rc.Err, "dd: failed to open '%s': %v\n", cfg.ifile, reason(err))
			return 1
		}
		defer f.Close()
		inf = f
		in = f
	}
	var out io.Writer = rc.Out
	var outf *os.File
	if cfg.ofile != "" {
		f, err := os.OpenFile(rc.Path(cfg.ofile), os.O_WRONLY|os.O_CREATE, 0o666)
		if err != nil {
			fmt.Fprintf(rc.Err, "dd: failed to open '%s': %v\n", cfg.ofile, reason(err))
			return 1
		}
		outf = f
		out = f
		if !cfg.notrunc {
			// POSIX: truncate at the seek offset, preserving the blocks
			// dd seeks over. Truncate can fail on special files
			// (/dev/null); GNU ignores that, so only surface the error
			// for regular files where it would mean silent stale data.
			if err := f.Truncate(cfg.seek * cfg.obs); err != nil {
				if fi, serr := f.Stat(); serr == nil && fi.Mode().IsRegular() {
					fmt.Fprintf(rc.Err, "dd: failed to truncate '%s': %v\n", cfg.ofile, reason(err))
					return 1
				}
			}
		}
	}
	if cfg.skip > 0 {
		n := cfg.skip * cfg.ibs
		if inf != nil {
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
		if outf == nil {
			return tool.NotSupported(rc, cmd, "seek= with standard output")
		}
		if _, err := outf.Seek(cfg.seek*cfg.obs, io.SeekStart); err != nil {
			fmt.Fprintf(rc.Err, "dd: failed to seek '%s': %v\n", cfg.ofile, reason(err))
			return 1
		}
	}

	var blocker *obsWriter
	if cfg.reblock {
		blocker = &obsWriter{w: out, obs: cfg.obs}
		out = blocker
	}
	buf := make([]byte, cfg.ibs)
	var full, partial, bytesCopied int64
	for cfg.count < 0 || full+partial < cfg.count {
		n, rerr := in.Read(buf)
		if n > 0 {
			if int64(n) == cfg.ibs {
				full++
			} else {
				partial++
			}
			if _, err := out.Write(buf[:n]); err != nil {
				fmt.Fprintf(rc.Err, "dd: error writing output: %v\n", reason(err))
				return 1
			}
			bytesCopied += int64(n)
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			fmt.Fprintf(rc.Err, "dd: error reading input: %v\n", reason(rerr))
			return 1
		}
	}
	outFull, outPartial := full, partial
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
		return 0
	}
	fmt.Fprintf(rc.Err, "%d+%d records in\n", full, partial)
	fmt.Fprintf(rc.Err, "%d+%d records out\n", outFull, outPartial)
	if cfg.status != "noxfer" {
		fmt.Fprintf(rc.Err, "%d bytes copied\n", bytesCopied)
	}
	return 0
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
			if _, err := o.w.Write(p[:o.obs]); err != nil {
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
			if _, err := o.w.Write(o.buf); err != nil {
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
	if _, err := o.w.Write(o.buf); err != nil {
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
	return n * m, nil
}

func parseCount(s string) (int64, error) {
	n, err := parseBytes(s)
	if err == nil {
		return n, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

func reason(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
