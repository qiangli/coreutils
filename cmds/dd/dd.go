// Package ddcmd implements a practical dd(1) subset: copy bytes from
// input to output using dd-style KEY=VALUE operands.
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
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	cfg := config{ibs: 512, obs: 512, count: -1}
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
		case "ibs":
			n, err := parseBytes(v)
			if err != nil || n <= 0 {
				return tool.UsageError(rc, cmd, "invalid number: '%s'", v)
			}
			cfg.ibs = n
		case "obs":
			n, err := parseBytes(v)
			if err != nil || n <= 0 {
				return tool.UsageError(rc, cmd, "invalid number: '%s'", v)
			}
			cfg.obs = n
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
		flags := os.O_WRONLY | os.O_CREATE
		if !cfg.notrunc {
			flags |= os.O_TRUNC
		}
		f, err := os.OpenFile(rc.Path(cfg.ofile), flags, 0o666)
		if err != nil {
			fmt.Fprintf(rc.Err, "dd: failed to open '%s': %v\n", cfg.ofile, reason(err))
			return 1
		}
		outf = f
		out = f
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
	fmt.Fprintf(rc.Err, "%d+%d records out\n", full, partial)
	if cfg.status != "noxfer" {
		fmt.Fprintf(rc.Err, "%d bytes copied\n", bytesCopied)
	}
	return 0
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
