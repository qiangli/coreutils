// Package uuencodecmd implements the portable, historical uuencode(1)
// format.  It deliberately does not implement GNU's base64 (-m) variant;
// callers can use base64(1) when that representation is required.
package uuencodecmd

import (
	"fmt"
	"io"
	"os"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "uuencode",
	Synopsis: "Encode a binary file in printable ASCII.",
	Usage: "uuencode [FILE] REMOTE-FILE\n\n" +
		"Read FILE, or standard input when FILE is omitted or -, and write the\n" +
		"traditional uuencoded representation using REMOTE-FILE in its header.\n" +
		"The GNU base64 variant (-m/--base64) is not supported; use base64 instead.",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}
	if len(operands) > 2 {
		return tool.UsageError(rc, cmd, "extra operand '%s'", operands[2])
	}

	input := rc.In
	mode := os.FileMode(0o666)
	remote := operands[0]
	var file *os.File
	if len(operands) == 2 {
		remote = operands[1]
		if operands[0] != "-" {
			var err error
			file, err = os.Open(rc.Path(operands[0]))
			if err != nil {
				fmt.Fprintf(rc.Err, "uuencode: %s: %v\n", operands[0], err)
				return 1
			}
			defer file.Close()
			input = file
			if info, err := file.Stat(); err == nil {
				mode = info.Mode().Perm()
			}
		}
	}

	if _, err := fmt.Fprintf(rc.Out, "begin %03o %s\n", mode.Perm(), remote); err != nil {
		fmt.Fprintf(rc.Err, "uuencode: write error: %v\n", err)
		return 1
	}
	buf := make([]byte, 45)
	for {
		n, err := io.ReadFull(input, buf)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			fmt.Fprintf(rc.Err, "uuencode: read error: %v\n", err)
			return 1
		}
		if n == 0 {
			break
		}
		line := make([]byte, 1, 1+((n+2)/3)*4+1)
		line[0] = enc(byte(n))
		for i := 0; i < n; i += 3 {
			var v uint32
			for j := 0; j < 3; j++ {
				v <<= 8
				if i+j < n {
					v |= uint32(buf[i+j])
				}
			}
			line = append(line, enc(byte(v>>18)), enc(byte(v>>12)), enc(byte(v>>6)), enc(byte(v)))
		}
		line = append(line, '\n')
		if _, err := rc.Out.Write(line); err != nil {
			fmt.Fprintf(rc.Err, "uuencode: write error: %v\n", err)
			return 1
		}
		if err == io.ErrUnexpectedEOF {
			break
		}
	}
	if _, err := io.WriteString(rc.Out, "`\nend\n"); err != nil {
		fmt.Fprintf(rc.Err, "uuencode: write error: %v\n", err)
		return 1
	}
	return 0
}

func enc(b byte) byte {
	b = (b & 0x3f) + 0x20
	if b == 0x20 {
		return '`'
	}
	return b
}
