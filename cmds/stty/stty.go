package sttycmd

import (
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

var cmd = &tool.Tool{Name: "stty", Synopsis: "Change and print terminal line settings.", Usage: "stty [OPTION]... [SETTING]..."}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	all, save, file, operands, code := parseArgs(rc, args)
	if code >= 0 {
		return code
	}
	if all && save {
		fmt.Fprintln(rc.Err, "stty: the options for verbose and stty-readable output styles are mutually exclusive")
		return 1
	}
	if len(operands) > 0 && (all || save) {
		fmt.Fprintln(rc.Err, "stty: when specifying an output style, modes may not be set")
		return 1
	}
	f, ok := rc.In.(*os.File)
	deviceName := "standard input"
	if !ok {
		fmt.Fprintln(rc.Err, "stty: standard input: inappropriate ioctl for device")
		return 1
	}
	if file != "" {
		opened, err := os.OpenFile(rc.Path(file), os.O_RDWR, 0)
		if err != nil {
			fmt.Fprintf(rc.Err, "stty: %s: %v\n", file, err)
			return 1
		}
		defer opened.Close()
		f = opened
		deviceName = file
	}
	fd := int(f.Fd())
	if !term.IsTerminal(fd) {
		fmt.Fprintf(rc.Err, "stty: %s: inappropriate ioctl for device\n", deviceName)
		return 1
	}
	if len(operands) == 0 || all || save {
		return printSettings(rc, fd, all, save)
	}
	return applySettings(rc, fd, operands)
}

func parseArgs(rc *tool.RunContext, args []string) (bool, bool, string, []string, int) {
	fs := sttyFlagSet()
	var all, save bool
	var file string
	var operands []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help" || a == "--version":
			_, code := tool.Parse(rc, cmd, fs, []string{a})
			return false, false, "", nil, code
		case a == "--":
			operands = append(operands, args[i+1:]...)
			return all, save, file, operands, -1
		case a == "-a" || a == "--all":
			all = true
		case a == "-g" || a == "--save":
			save = true
		case a == "-F" || a == "--file":
			if i+1 >= len(args) {
				return false, false, "", nil, tool.UsageError(rc, cmd, "option %s requires an argument", a)
			}
			i++
			file = args[i]
		case strings.HasPrefix(a, "--file="):
			file = strings.TrimPrefix(a, "--file=")
		case strings.HasPrefix(a, "-F") && len(a) > 2:
			file = a[2:]
		default:
			operands = append(operands, a)
		}
	}
	return all, save, file, operands, -1
}

func sttyFlagSet() *pflag.FlagSet {
	fs := tool.NewFlags(cmd.Name)
	fs.BoolP("all", "a", false, "print all current settings")
	fs.BoolP("save", "g", false, "print settings in stty-readable form")
	fs.StringP("file", "F", "", "open and use specified DEVICE")
	return fs
}

func printSettings(rc *tool.RunContext, fd int, all, save bool) int {
	state, err := term.GetState(fd)
	if err != nil {
		fmt.Fprintf(rc.Err, "stty: %v\n", err)
		return 1
	}
	if save {
		fmt.Fprintln(rc.Out, hex.EncodeToString(stateBytes(state)))
		return 0
	}
	w, h, err := term.GetSize(fd)
	if err != nil {
		w, h = 0, 0
	}
	if all {
		fmt.Fprintf(rc.Out, "speed 0 baud; rows %d; columns %d;\n", h, w)
		fmt.Fprintln(rc.Out, "intr = ^C; quit = ^\\; erase = ^?; kill = ^U; eof = ^D;")
		return 0
	}
	fmt.Fprintf(rc.Out, "speed 0 baud; rows %d; columns %d;\n", h, w)
	return 0
}

func stateBytes(state *term.State) []byte {
	return []byte(fmt.Sprintf("%#v", state))
}

func applySettings(rc *tool.RunContext, fd int, ops []string) int {
	for i := 0; i < len(ops); i++ {
		switch ops[i] {
		case "size":
			w, h, err := term.GetSize(fd)
			if err != nil {
				fmt.Fprintf(rc.Err, "stty: %v\n", err)
				return 1
			}
			fmt.Fprintf(rc.Out, "%d %d\n", h, w)
		case "raw", "-raw", "cooked", "-cooked", "cbreak", "-cbreak", "sane",
			"echo", "-echo", "icanon", "-icanon", "isig", "-isig", "iexten", "-iexten",
			"echoe", "-echoe", "echok", "-echok", "echonl", "-echonl", "noflsh", "-noflsh",
			"ixon", "-ixon", "ixoff", "-ixoff", "icrnl", "-icrnl", "opost", "-opost",
			"onlcr", "-onlcr", "parenb", "-parenb", "parodd", "-parodd",
			"cs5", "cs6", "cs7", "cs8", "evenp", "-evenp", "parity", "-parity",
			"oddp", "-oddp", "pass8", "-pass8", "litout", "-litout", "nl", "-nl",
			"crt", "dec", "decctlq", "-decctlq", "ek", "drain", "-drain":
			if err := applyMode(fd, ops[i]); err != nil {
				fmt.Fprintf(rc.Err, "stty: %v\n", err)
				return 1
			}
		case "min", "time":
			if i+1 >= len(ops) {
				return tool.UsageError(rc, cmd, "missing argument to %q", ops[i])
			}
			n, err := parseUint8(ops[i+1])
			if err != nil {
				return tool.UsageError(rc, cmd, "invalid integer %q", ops[i+1])
			}
			if err := applyValue(fd, ops[i], n); err != nil {
				fmt.Fprintf(rc.Err, "stty: %v\n", err)
				return 1
			}
			i++
		case "rows", "cols", "columns":
			if i+1 >= len(ops) {
				return tool.UsageError(rc, cmd, "missing argument to %q", ops[i])
			}
			if _, err := parseRowsCols(ops[i+1]); err != nil {
				return tool.UsageError(rc, cmd, "invalid integer %q", ops[i+1])
			}
			i++
		default:
			if isSavedState(ops[i]) {
				continue
			}
			return tool.UsageError(rc, cmd, "invalid argument %q", ops[i])
		}
	}
	return 0
}

func parseUint8(s string) (uint8, error) {
	n, err := strconv.ParseUint(s, 0, 8)
	return uint8(n), err
}

func parseRowsCols(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 0, 32)
	if err != nil {
		return 0, err
	}
	return uint16(n % (math.MaxUint16 + 1)), nil
}

func isSavedState(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil && len(s) > 0
}
