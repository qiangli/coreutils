// Package stringscmd implements strings(1) per the GNU binutils
// manual: print the sequences of printable characters in files.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/strings/strings.go (BSD-3-Clause).
// Changes: rewired to tool framework; tab counted as printable (GNU
// includes isblank characters); 7-wide right-aligned -t offsets per
// GNU output shape; offsets reset per file; removed the partial-flush
// optimization (it broke -t offset math).
package stringscmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "strings",
	Synopsis: "Print the sequences of printable characters in files.\nWith no FILE, read standard input.",
	Usage:    "strings [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	minLen := fs.IntP("bytes", "n", 4, "locate and print any sequence of at least NUMBER printable characters")
	radix := fs.StringP("radix", "t", "", "print the offset within the file before each string; RADIX is o (octal), d (decimal) or x (hexadecimal)")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	if *minLen < 1 {
		return tool.UsageError(rc, cmd, "invalid minimum string length %d", *minLen)
	}
	switch *radix {
	case "", "o", "d", "x":
	default:
		return tool.UsageError(rc, cmd, "invalid radix: %q (must be o, d or x)", *radix)
	}

	w := bufio.NewWriter(rc.Out)
	exit := 0
	if len(operands) == 0 {
		var in io.Reader = rc.In
		if in == nil {
			in = strings.NewReader("")
		}
		if err := scan(in, w, *minLen, *radix); err != nil {
			fmt.Fprintf(rc.Err, "strings: %v\n", err)
			exit = 1
		}
	}
	for _, name := range operands {
		f, err := os.Open(rc.Path(name))
		if err != nil {
			fmt.Fprintf(rc.Err, "strings: %s: %v\n", name, sysErr(err))
			exit = 1
			continue
		}
		err = scan(f, w, *minLen, *radix)
		f.Close()
		if err != nil {
			fmt.Fprintf(rc.Err, "strings: %s: %v\n", name, sysErr(err))
			exit = 1
		}
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "strings: write error: %v\n", err)
		return 1
	}
	return exit
}

// printable matches GNU strings' default character set: graphic ASCII
// plus blank characters (space is in 32..126; tab is the other blank).
func printable(c byte) bool {
	return (c >= 32 && c <= 126) || c == '\t'
}

func scan(r io.Reader, w *bufio.Writer, minLen int, radix string) error {
	br := bufio.NewReaderSize(r, 64*1024)
	var run []byte
	var offset, start int64
	flush := func() error {
		if len(run) >= minLen {
			switch radix {
			case "o":
				fmt.Fprintf(w, "%7o ", start)
			case "d":
				fmt.Fprintf(w, "%7d ", start)
			case "x":
				fmt.Fprintf(w, "%7x ", start)
			}
			if _, err := w.Write(run); err != nil {
				return err
			}
			if err := w.WriteByte('\n'); err != nil {
				return err
			}
		}
		run = run[:0]
		return nil
	}
	for {
		b, err := br.ReadByte()
		if err == io.EOF {
			return flush()
		}
		if err != nil {
			return err
		}
		if printable(b) {
			if len(run) == 0 {
				start = offset
			}
			run = append(run, b)
		} else {
			if ferr := flush(); ferr != nil {
				return ferr
			}
		}
		offset++
	}
}

func sysErr(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
