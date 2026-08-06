// Package uudecodecmd implements decoding of the portable, historical
// uuencode(1) representation.  Header output names are restricted to safe
// relative basenames; use -o to select an explicit path.
package uudecodecmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "uudecode",
	Synopsis: "Decode a uuencoded file.",
	Usage: "uudecode [OPTION]... [FILE]\n\n" +
		"Decode traditional uuencoded data from FILE, or standard input.\n\n" +
		"  -o, --output-file=FILE  write to FILE instead of the header name; - means stdout\n\n" +
		"The begin-base64 extension is rejected; use base64 --decode instead.",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	output := fs.StringP("output-file", "o", "", "write to FILE instead of the header name")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) > 1 {
		return tool.UsageError(rc, cmd, "extra operand '%s'", operands[1])
	}

	input := rc.In
	var file *os.File
	if len(operands) == 1 && operands[0] != "-" {
		var err error
		file, err = os.Open(rc.Path(operands[0]))
		if err != nil {
			fmt.Fprintf(rc.Err, "uudecode: %s: %v\n", operands[0], err)
			return 1
		}
		defer file.Close()
		input = file
	}
	r := bufio.NewReader(input)
	var header string
	for {
		line, err := r.ReadString('\n')
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if strings.HasPrefix(line, "begin-base64 ") {
			fmt.Fprintln(rc.Err, "uudecode: begin-base64 data is not supported; use base64 --decode")
			return 2
		}
		if strings.HasPrefix(line, "begin ") {
			header = line
			break
		}
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(rc.Err, "uudecode: no 'begin' line")
				return 1
			}
			fmt.Fprintf(rc.Err, "uudecode: read error: %v\n", err)
			return 1
		}
	}
	parts := strings.SplitN(header, " ", 3)
	if len(parts) != 3 {
		fmt.Fprintln(rc.Err, "uudecode: malformed 'begin' line")
		return 1
	}
	mode64, err := strconv.ParseUint(parts[1], 8, 12)
	if err != nil || parts[2] == "" {
		fmt.Fprintln(rc.Err, "uudecode: malformed 'begin' line")
		return 1
	}
	name := parts[2]
	if *output == "" && !safeHeaderName(name) {
		fmt.Fprintf(rc.Err, "uudecode: unsafe output name in header: %q; use --output-file\n", name)
		return 1
	}
	if *output != "" {
		name = *output
	}

	var out io.Writer = rc.Out
	var outf *os.File
	if name != "-" {
		outf, err = os.OpenFile(rc.Path(name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(mode64))
		if err != nil {
			fmt.Fprintf(rc.Err, "uudecode: %s: %v\n", name, err)
			return 1
		}
		defer outf.Close()
		out = outf
	}
	if err := decode(r, out); err != nil {
		fmt.Fprintf(rc.Err, "uudecode: %v\n", err)
		return 1
	}
	if outf != nil {
		if err := outf.Chmod(os.FileMode(mode64)); err != nil {
			fmt.Fprintf(rc.Err, "uudecode: %s: %v\n", name, err)
			return 1
		}
	}
	return 0
}

func safeHeaderName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !filepath.IsAbs(name) && !strings.ContainsAny(name, `/\\`)
}

func decode(r *bufio.Reader, out io.Writer) error {
	for {
		line, err := r.ReadString('\n')
		if err != nil && err != io.EOF {
			return fmt.Errorf("read error: %w", err)
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "end" {
			return fmt.Errorf("malformed data: missing zero-length line before 'end'")
		}
		if line == "" && err == io.EOF {
			return fmt.Errorf("short file")
		}
		if line == "" {
			return fmt.Errorf("malformed empty data line")
		}
		n := int(dec(line[0]))
		if n == 0 {
			next, nextErr := r.ReadString('\n')
			next = strings.TrimSuffix(strings.TrimSuffix(next, "\n"), "\r")
			if next != "end" {
				return fmt.Errorf("malformed data: expected 'end'")
			}
			if nextErr != nil && nextErr != io.EOF {
				return fmt.Errorf("read error: %w", nextErr)
			}
			return nil
		}
		if n > 45 {
			return fmt.Errorf("invalid data line length %d", n)
		}
		need := ((n + 2) / 3) * 4
		if len(line)-1 < need {
			return fmt.Errorf("short data line")
		}
		decoded := make([]byte, 0, n)
		for i := 1; i <= need; i += 4 {
			for j := 0; j < 4; j++ {
				c := line[i+j]
				if c < 0x20 || c > 0x60 {
					return fmt.Errorf("invalid encoded character 0x%02x", c)
				}
			}
			v := uint32(dec(line[i]))<<18 | uint32(dec(line[i+1]))<<12 | uint32(dec(line[i+2]))<<6 | uint32(dec(line[i+3]))
			decoded = append(decoded, byte(v>>16), byte(v>>8), byte(v))
		}
		if _, err := out.Write(decoded[:n]); err != nil {
			return fmt.Errorf("write error: %w", err)
		}
		if err == io.EOF {
			return fmt.Errorf("short file")
		}
	}
}

func dec(b byte) byte { return (b - 0x20) & 0x3f }
