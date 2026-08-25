package sttycmd

import (
	"bytes"
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

var cmd = &tool.Tool{
	Name:     "stty",
	Synopsis: "Change and print terminal line settings.",
	Usage:    "stty [-a|-g]\n   or: stty operand...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type terminalState struct {
	Iflag, Oflag, Cflag, Lflag uint64
	Ispeed, Ospeed             uint64
	Cc                         []byte
}

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
		case a == "-ag" || a == "-ga":
			all, save = true, true
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
	state, err := getTerminalState(fd)
	if err != nil {
		fmt.Fprintf(rc.Err, "stty: %v\n", err)
		return 1
	}
	if save {
		if _, err := fmt.Fprintln(rc.Out, encodeState(state)); err != nil {
			fmt.Fprintf(rc.Err, "stty: write error: %v\n", err)
			return 1
		}
		return 0
	}
	w, h, err := term.GetSize(fd)
	if err != nil {
		w, h = 0, 0
	}
	var report bytes.Buffer
	reportRC := *rc
	reportRC.Out = &report
	printReadableSettings(&reportRC, state, h, w, all)
	if _, err := rc.Out.Write(report.Bytes()); err != nil {
		fmt.Fprintf(rc.Err, "stty: write error: %v\n", err)
		return 1
	}
	return 0
}

func encodeState(s *terminalState) string {
	return fmt.Sprintf("v1:%x:%x:%x:%x:%x:%x:%s", s.Iflag, s.Oflag, s.Cflag, s.Lflag, s.Ispeed, s.Ospeed, hex.EncodeToString(s.Cc))
}

func decodeState(value string) (*terminalState, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 8 || parts[0] != "v1" {
		return nil, fmt.Errorf("invalid saved settings %q", value)
	}
	values := make([]uint64, 6)
	for i := range values {
		n, err := strconv.ParseUint(parts[i+1], 16, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid saved settings %q", value)
		}
		values[i] = n
	}
	cc, err := hex.DecodeString(parts[7])
	if err != nil || len(cc) == 0 {
		return nil, fmt.Errorf("invalid saved settings %q", value)
	}
	return &terminalState{Iflag: values[0], Oflag: values[1], Cflag: values[2], Lflag: values[3], Ispeed: values[4], Ospeed: values[5], Cc: cc}, nil
}

var simpleModes = map[string]bool{
	"raw": true, "cooked": true, "cbreak": true, "sane": true,
	"echo": true, "icanon": true, "isig": true, "iexten": true,
	"echoe": true, "echok": true, "echonl": true, "noflsh": true, "tostop": true,
	"ixon": true, "ixoff": true, "ixany": true, "icrnl": true, "ignbrk": true,
	"brkint": true, "ignpar": true, "parmrk": true, "inpck": true, "istrip": true,
	"inlcr": true, "igncr": true, "opost": true, "onlcr": true, "ocrnl": true,
	"onocr": true, "onlret": true, "ofill": true, "ofdel": true,
	"parenb": true, "parodd": true, "hupcl": true, "hup": true, "cstopb": true,
	"cread": true, "clocal": true, "cs5": true, "cs6": true, "cs7": true, "cs8": true,
	"evenp": true, "parity": true, "oddp": true, "pass8": true, "litout": true,
	"nl": true, "crt": true, "dec": true, "decctlq": true, "ek": true,
	"drain": true, "cr0": true, "cr1": true, "cr2": true, "cr3": true,
	"nl0": true, "nl1": true, "tab0": true, "tab1": true, "tab2": true,
	"tab3": true, "tabs": true, "bs0": true, "bs1": true, "ff0": true,
	"ff1": true, "vt0": true, "vt1": true,
}

var controlChars = map[string]bool{
	"eof": true, "eol": true, "erase": true, "intr": true, "kill": true,
	"quit": true, "susp": true, "start": true, "stop": true,
}

func applySettings(rc *tool.RunContext, fd int, ops []string) int {
	original, err := getTerminalState(fd)
	if err != nil {
		fmt.Fprintf(rc.Err, "stty: %v\n", err)
		return 1
	}
	if code := prevalidateSettings(rc, ops, original); code >= 0 {
		return code
	}

	originalWidth, originalHeight := 0, 0
	hasWindowChange := false
	for _, op := range ops {
		if op == "rows" || op == "cols" || op == "columns" {
			hasWindowChange = true
			break
		}
	}
	if hasWindowChange {
		originalWidth, originalHeight, err = term.GetSize(fd)
		if err != nil {
			fmt.Fprintf(rc.Err, "stty: %v\n", err)
			return 1
		}
	}

	rollback := func() {
		_ = setTerminalState(fd, original)
		if hasWindowChange {
			_ = applyWindowSize(fd, originalHeight, originalWidth)
		}
	}
	fail := func(err error) int {
		rollback()
		fmt.Fprintf(rc.Err, "stty: %v\n", err)
		return 1
	}

	for i := 0; i < len(ops); i++ {
		op := ops[i]
		base := strings.TrimPrefix(op, "-")
		switch {
		case op == "size":
			w, h, err := term.GetSize(fd)
			if err != nil {
				fmt.Fprintf(rc.Err, "stty: %v\n", err)
				return 1
			}
			if _, err := fmt.Fprintf(rc.Out, "%d %d\n", h, w); err != nil {
				return fail(fmt.Errorf("write error: %w", err))
			}
		case op == "baud":
			state, err := getTerminalState(fd)
			if err != nil {
				fmt.Fprintf(rc.Err, "stty: %v\n", err)
				return 1
			}
			if _, err := fmt.Fprintln(rc.Out, outputBaud(state)); err != nil {
				return fail(fmt.Errorf("write error: %w", err))
			}
		case simpleModes[base] && (op == base || modeMayBeNegated(base)):
			if err := applyMode(fd, op); err != nil {
				return fail(err)
			}
		case op == "min" || op == "time":
			value, next, code := valueOperand(rc, ops, i, parseUint8)
			if code >= 0 {
				return code
			}
			if err := applyValue(fd, op, value); err != nil {
				return fail(err)
			}
			i = next
		case op == "ispeed" || op == "ospeed":
			if i+1 >= len(ops) {
				return tool.UsageError(rc, cmd, "missing argument to %q", op)
			}
			baud, err := strconv.ParseUint(ops[i+1], 10, 64)
			if err != nil {
				return tool.UsageError(rc, cmd, "invalid speed %q", ops[i+1])
			}
			if err := applySpeed(fd, op, baud); err != nil {
				return fail(err)
			}
			i++
		case controlChars[op]:
			if i+1 >= len(ops) {
				return tool.UsageError(rc, cmd, "missing argument to %q", op)
			}
			value, err := parseControlChar(ops[i+1])
			if err != nil {
				return tool.UsageError(rc, cmd, "%v", err)
			}
			if err := applyControlChar(fd, op, value); err != nil {
				return fail(err)
			}
			i++
		case op == "rows" || op == "cols" || op == "columns":
			if i+1 >= len(ops) {
				return tool.UsageError(rc, cmd, "missing argument to %q", op)
			}
			n, err := parseRowsCols(ops[i+1])
			if err != nil {
				return tool.UsageError(rc, cmd, "invalid integer %q", ops[i+1])
			}
			rows, cols := -1, -1
			if op == "rows" {
				rows = int(n)
			} else {
				cols = int(n)
			}
			if err := applyWindowSize(fd, rows, cols); err != nil {
				return fail(err)
			}
			i++
		case isDecimal(op):
			baud, _ := strconv.ParseUint(op, 10, 64)
			if err := applySpeed(fd, "speed", baud); err != nil {
				return fail(err)
			}
		case strings.HasPrefix(op, "v1:"):
			state, err := decodeState(op)
			if err == nil {
				err = setTerminalState(fd, state)
			}
			if err != nil {
				return fail(err)
			}
		default:
			return tool.UsageError(rc, cmd, "invalid argument %q", op)
		}
	}
	return 0
}

// prevalidateSettings rejects the complete operand list before the first
// ioctl.  Besides producing all-or-nothing behavior for malformed lists, this
// keeps a later unsupported platform mode from leaving earlier modes applied.
func prevalidateSettings(rc *tool.RunContext, ops []string, current *terminalState) int {
	for i := 0; i < len(ops); i++ {
		op := ops[i]
		base := strings.TrimPrefix(op, "-")
		switch {
		case op == "size" || op == "baud":
		case simpleModes[base] && (op == base || modeMayBeNegated(base)):
			if err := validateMode(op); err != nil {
				fmt.Fprintf(rc.Err, "stty: %v\n", err)
				return 1
			}
		case op == "min" || op == "time":
			_, next, code := valueOperand(rc, ops, i, parseUint8)
			if code >= 0 {
				return code
			}
			i = next
		case op == "ispeed" || op == "ospeed":
			if i+1 >= len(ops) {
				return tool.UsageError(rc, cmd, "missing argument to %q", op)
			}
			baud, err := strconv.ParseUint(ops[i+1], 10, 64)
			if err != nil {
				return tool.UsageError(rc, cmd, "invalid speed %q", ops[i+1])
			}
			if err := validateSpeed(baud); err != nil {
				fmt.Fprintf(rc.Err, "stty: %v\n", err)
				return 1
			}
			i++
		case controlChars[op]:
			if i+1 >= len(ops) {
				return tool.UsageError(rc, cmd, "missing argument to %q", op)
			}
			if _, err := parseControlChar(ops[i+1]); err != nil {
				return tool.UsageError(rc, cmd, "%v", err)
			}
			if err := validateControlChar(op, len(current.Cc)); err != nil {
				fmt.Fprintf(rc.Err, "stty: %v\n", err)
				return 1
			}
			i++
		case op == "rows" || op == "cols" || op == "columns":
			if i+1 >= len(ops) {
				return tool.UsageError(rc, cmd, "missing argument to %q", op)
			}
			if _, err := parseRowsCols(ops[i+1]); err != nil {
				return tool.UsageError(rc, cmd, "invalid integer %q", ops[i+1])
			}
			i++
		case isDecimal(op):
			baud, _ := strconv.ParseUint(op, 10, 64)
			if err := validateSpeed(baud); err != nil {
				fmt.Fprintf(rc.Err, "stty: %v\n", err)
				return 1
			}
		case strings.HasPrefix(op, "v1:"):
			state, err := decodeState(op)
			if err != nil {
				fmt.Fprintf(rc.Err, "stty: %v\n", err)
				return 1
			}
			if len(state.Cc) != len(current.Cc) {
				fmt.Fprintln(rc.Err, "stty: saved settings are for an incompatible platform")
				return 1
			}
		default:
			return tool.UsageError(rc, cmd, "invalid argument %q", op)
		}
	}
	return -1
}

func modeMayBeNegated(mode string) bool {
	switch mode {
	case "cs5", "cs6", "cs7", "cs8", "sane", "ek", "crt", "dec",
		"drain", "cr0", "cr1", "cr2", "cr3", "nl0", "nl1", "tab0",
		"tab1", "tab2", "tab3", "bs0", "bs1", "ff0", "ff1", "vt0", "vt1":
		return false
	default:
		return true
	}
}

func valueOperand(rc *tool.RunContext, ops []string, i int, parse func(string) (uint8, error)) (uint8, int, int) {
	if i+1 >= len(ops) {
		return 0, i, tool.UsageError(rc, cmd, "missing argument to %q", ops[i])
	}
	n, err := parse(ops[i+1])
	if err != nil {
		return 0, i, tool.UsageError(rc, cmd, "invalid integer %q", ops[i+1])
	}
	return n, i + 1, -1
}

func parseControlChar(s string) (uint8, error) {
	if s == "^-" || s == "undef" {
		return posixVDisable, nil
	}
	if len(s) == 2 && s[0] == '^' {
		if s[1] == '?' {
			return 0x7f, nil
		}
		if s[1] >= '@' && s[1] <= '_' {
			return s[1] & 0x1f, nil
		}
		if s[1] >= 'a' && s[1] <= 'z' {
			return (s[1] - 'a' + 'A') & 0x1f, nil
		}
	}
	if len(s) == 1 {
		return s[0], nil
	}
	return 0, fmt.Errorf("invalid control character %q", s)
}

func parseUint8(s string) (uint8, error) {
	n, err := strconv.ParseUint(s, 10, 8)
	return uint8(n), err
}

func parseRowsCols(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, err
	}
	if n > math.MaxInt32 {
		return 0, fmt.Errorf("invalid integer argument %q: value too large", s)
	}
	return uint16(n % (math.MaxUint16 + 1)), nil
}

func isDecimal(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
