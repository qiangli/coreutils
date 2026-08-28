// Package killcmd implements the POSIX kill(1) utility.
package killcmd

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "kill",
	Synopsis: "Send a signal to one or more processes.",
	Usage:    "kill [-s SIGNAL | -SIGNAL] PID...\n       kill -l [EXIT_STATUS]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h" || args[0] == "--version" || args[0] == "-V") {
		fs := tool.NewFlags(cmd.Name)
		_, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args[:1]))
		return code
	}

	sig := signalByName("TERM")
	operands := args
	if len(operands) > 0 {
		switch {
		case operands[0] == "-l" || operands[0] == "-L":
			return listSignals(rc, operands[1:])
		case operands[0] == "-s":
			if len(operands) < 2 {
				return usage(rc, "option requires an argument -- 's'")
			}
			var ok bool
			sig, ok = parseSignal(operands[1])
			if !ok {
				return badSignal(rc, operands[1])
			}
			operands = operands[2:]
		case operands[0] == "-n":
			if len(operands) < 2 {
				return usage(rc, "option requires an argument -- 'n'")
			}
			var ok bool
			sig, ok = parseSignalNumber(operands[1])
			if !ok {
				return badSignal(rc, operands[1])
			}
			operands = operands[2:]
		case strings.HasPrefix(operands[0], "-s") && len(operands[0]) > 2:
			var ok bool
			sig, ok = parseSignal(operands[0][2:])
			if !ok {
				return badSignal(rc, operands[0][2:])
			}
			operands = operands[1:]
		case strings.HasPrefix(operands[0], "-n") && len(operands[0]) > 2:
			var ok bool
			sig, ok = parseSignalNumber(operands[0][2:])
			if !ok {
				return badSignal(rc, operands[0][2:])
			}
			operands = operands[1:]
		case operands[0] == "--":
			operands = operands[1:]
		case strings.HasPrefix(operands[0], "-") && operands[0] != "-":
			var ok bool
			sig, ok = parseSignal(operands[0][1:])
			if !ok {
				return badSignal(rc, operands[0][1:])
			}
			operands = operands[1:]
		}
	}
	// POSIX permits the option delimiter between a selected signal and a
	// negative process-group operand: kill -s TERM -- -PGID.
	if len(operands) > 0 && operands[0] == "--" {
		operands = operands[1:]
	}
	if len(operands) == 0 {
		return usage(rc, "missing process operand")
	}

	exit := 0
	for _, operand := range operands {
		pid, err := strconv.ParseInt(operand, 10, 32)
		if err != nil {
			fmt.Fprintf(rc.Err, "kill: invalid process id %q\n", operand)
			exit = 1
			continue
		}
		if err := sendSignal(int(pid), sig); err != nil {
			fmt.Fprintf(rc.Err, "kill: (%s): %v\n", operand, tool.SysErr(err))
			exit = 1
		}
	}
	return exit
}

func listSignals(rc *tool.RunContext, args []string) int {
	if len(args) == 0 {
		var b strings.Builder
		for i, name := range signalNames() {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(name)
		}
		b.WriteByte('\n')
		return writeOut(rc, b.String())
	}
	if len(args) != 1 {
		return usage(rc, "too many operands for -l")
	}
	if n, err := strconv.Atoi(args[0]); err == nil {
		if n > 128 {
			n -= 128
		}
		if name, ok := signalName(n); ok {
			return writeOut(rc, name+"\n")
		}
		return badSignal(rc, args[0])
	}
	sig, ok := parseSignal(args[0])
	if !ok {
		return badSignal(rc, args[0])
	}
	return writeOut(rc, strconv.Itoa(signalNumber(sig))+"\n")
}

// writeOut emits one -l result and reports write failures as errors (POSIX:
// a nonzero exit status follows "an error occurred").
func writeOut(rc *tool.RunContext, s string) int {
	if _, err := io.WriteString(rc.Out, s); err != nil {
		fmt.Fprintf(rc.Err, "kill: write error: %v\n", tool.SysErr(err))
		return 1
	}
	return 0
}

func parseSignal(spec string) (nativeSignal, bool) {
	if sig, ok := parseSignalNumber(spec); ok {
		return sig, true
	}
	name := strings.ToUpper(spec)
	name = strings.TrimPrefix(name, "SIG")
	sig := signalByName(name)
	return sig, signalNumber(sig) >= 0
}

func parseSignalNumber(spec string) (nativeSignal, bool) {
	n, err := strconv.Atoi(spec)
	if err != nil || n < 0 || n > maxSignalNumber() {
		return invalidSignal(), false
	}
	return signalFromNumber(n), true
}

func usage(rc *tool.RunContext, message string) int {
	fmt.Fprintf(rc.Err, "kill: %s\nUsage: %s\n", message, cmd.Usage)
	return 2
}

func badSignal(rc *tool.RunContext, spec string) int {
	fmt.Fprintf(rc.Err, "kill: invalid signal specification %q\n", spec)
	return 1
}
