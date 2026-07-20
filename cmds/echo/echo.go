// Package echocmd implements echo(1) per the GNU coreutils manual:
// write arguments to standard output, separated by spaces, terminated
// by a newline.
//
// Portions adapted from https://github.com/guonaihong/coreutils echo/echo.go (Apache-2.0).
// Changes: rewired to the tool framework; GNU option scanning (an
// argument is an option only when it is exactly '-' followed by a run
// of [neE]; the first non-option stops scanning); \0 with zero octal
// digits emits NUL per GNU; escape interpreter rewritten over a byte
// buffer with \c aborting all output including the trailing newline.
package echocmd

import (
	"bytes"
	"fmt"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "echo",
	Synopsis: "Display a line of text.",
	Usage:    "echo [SHORT-OPTION]... [STRING]...\n   or: echo LONG-OPTION",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	// GNU echo recognizes --help / --version only as the sole argument;
	// otherwise they are operands and printed literally.
	posix := envPresent(rc.Env, "POSIXLY_CORRECT")
	if !posix && len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "--version" || args[0] == "-V") {
		if args[0] == "--help" || args[0] == "-h" {
			printHelp(rc)
			return 0
		}
		fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", cmd.Name, tool.Version)
		return 0
	}

	noNewline := false
	escapes := false // -E is the documented default
	if posix {
		// In POSIX compatibility mode GNU echo recognizes only an
		// initial, exact -n as an option. Escapes are always enabled,
		// including when -E appears after that -n.
		escapes = true
		if len(args) == 0 || args[0] != "-n" {
			return writeOperands(rc, args, escapes, noNewline)
		}
	}

	// GNU echo's option scan: consume leading args that are exactly
	// '-' followed by one or more of [neE]. Anything else (including
	// "--" and mixed runs like "-na") is an operand and ends the scan.
	i := 0
scan:
	for ; i < len(args); i++ {
		a := args[i]
		if len(a) < 2 || a[0] != '-' {
			break
		}
		for j := 1; j < len(a); j++ {
			switch a[j] {
			case 'n', 'e', 'E':
			default:
				break scan
			}
		}
		for j := 1; j < len(a); j++ {
			switch a[j] {
			case 'n':
				noNewline = true
			case 'e':
				escapes = true
			case 'E':
				escapes = false
			}
		}
	}
	if posix {
		// POSIXLY_CORRECT forces escape interpretation even if -E was
		// accepted while scanning after the initial -n.
		escapes = true
	}

	return writeOperands(rc, args[i:], escapes, noNewline)
}

func writeOperands(rc *tool.RunContext, operands []string, escapes, noNewline bool) int {
	var buf bytes.Buffer
	stopped := false
	for k, s := range operands {
		var operand bytes.Buffer
		if escapes {
			if interpretEscapes(&operand, s) {
				stopped = true
				if k > 0 && operand.Len() > 0 {
					buf.WriteByte(' ')
				}
				buf.Write(operand.Bytes())
				break
			}
		} else {
			operand.WriteString(s)
		}
		if k > 0 {
			buf.WriteByte(' ')
		}
		buf.Write(operand.Bytes())
	}
	if !stopped && !noNewline {
		buf.WriteByte('\n')
	}
	rc.Out.Write(buf.Bytes())
	return 0
}

func envPresent(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func printHelp(rc *tool.RunContext) {
	fmt.Fprintf(rc.Out, "Usage: %s\n%s\n", cmd.Usage, cmd.Synopsis)
	fmt.Fprint(rc.Out, `
Options:
  -n          do not output the trailing newline
  -e          enable interpretation of backslash escapes
  -E          disable interpretation of backslash escapes (default)
  -h, --help     display this help and exit
  -V, --version  output version information and exit
`)
}

// interpretEscapes appends s to buf interpreting the GNU echo -e
// escape set (\a \b \c \e \f \n \r \t \v \\ \0NNN \xHH). It reports
// true when \c was seen: produce no further output at all.
func interpretEscapes(buf *bytes.Buffer, s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' {
			buf.WriteByte(c)
			continue
		}
		if i == len(s)-1 {
			// Trailing lone backslash: literal.
			buf.WriteByte('\\')
			continue
		}
		i++
		switch s[i] {
		case 'a':
			buf.WriteByte('\a')
		case 'b':
			buf.WriteByte('\b')
		case 'c':
			return true
		case 'e':
			buf.WriteByte(0x1b)
		case 'f':
			buf.WriteByte('\f')
		case 'n':
			buf.WriteByte('\n')
		case 'r':
			buf.WriteByte('\r')
		case 't':
			buf.WriteByte('\t')
		case 'v':
			buf.WriteByte('\v')
		case '\\':
			buf.WriteByte('\\')
		case '0':
			// \0NNN: byte with octal value NNN (0 to 3 octal digits).
			v := 0
			j := i + 1
			for j < len(s) && j-i-1 < 3 && s[j] >= '0' && s[j] <= '7' {
				v = v*8 + int(s[j]-'0')
				j++
			}
			buf.WriteByte(byte(v))
			i = j - 1
		case 'x':
			// \xHH: byte with hexadecimal value HH (1 to 2 hex
			// digits); with no hex digit, "\x" is literal.
			v, n := 0, 0
			j := i + 1
			for j < len(s) && n < 2 {
				d, ok := hexVal(s[j])
				if !ok {
					break
				}
				v = v*16 + d
				j++
				n++
			}
			if n == 0 {
				buf.WriteString("\\x")
			} else {
				buf.WriteByte(byte(v))
				i = j - 1
			}
		default:
			// Unknown escape: backslash and character pass through.
			buf.WriteByte('\\')
			buf.WriteByte(s[i])
		}
	}
	return false
}

func hexVal(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	}
	return 0, false
}
