//go:build linux || darwin || freebsd || netbsd || openbsd

package sttycmd

import (
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

func getTerminalState(fd int) (*terminalState, error) {
	t, err := getTermios(fd)
	if err != nil {
		return nil, err
	}
	cc := make([]byte, len(t.Cc))
	copy(cc, t.Cc[:])
	ispeed, ospeed := nativeSpeeds(t)
	return &terminalState{
		Iflag: uint64(t.Iflag), Oflag: uint64(t.Oflag), Cflag: uint64(t.Cflag),
		Lflag: uint64(t.Lflag), Ispeed: ispeed, Ospeed: ospeed, Cc: cc,
	}, nil
}

func setTerminalState(fd int, state *terminalState) error {
	t, err := getTermios(fd)
	if err != nil {
		return err
	}
	if len(state.Cc) != len(t.Cc) {
		return fmt.Errorf("saved settings are for an incompatible platform")
	}
	stateToTermios(t, state)
	copy(t.Cc[:], state.Cc)
	return setTermios(fd, t)
}

func applyMode(fd int, mode string) error {
	if err := validateMode(mode); err != nil {
		return err
	}
	t, err := getTermios(fd)
	if err != nil {
		return err
	}
	applyTermiosMode(t, mode)
	return setTermios(fd, t)
}

func validateMode(mode string) error {
	base := strings.TrimPrefix(mode, "-")
	if base == "ofill" || base == "ofdel" {
		if _, ok := platformOutputFlag(base); !ok {
			return fmt.Errorf("%s is not supported on this platform", mode)
		}
	}
	if isDelayMode(base) {
		if _, _, ok := platformDelay(base); !ok {
			return fmt.Errorf("%s is not supported on this platform", mode)
		}
	}
	return nil
}

func validateSpeed(baud uint64) error {
	if _, ok := baudToNative(baud); !ok {
		return fmt.Errorf("unsupported speed %d", baud)
	}
	return nil
}

func validateControlChar(name string, ccLen int) error {
	index, ok := controlCharIndex(name)
	if !ok || index >= ccLen {
		return fmt.Errorf("control character %s is not supported on this platform", name)
	}
	return nil
}

func applyValue(fd int, name string, value uint8) error {
	t, err := getTermios(fd)
	if err != nil {
		return err
	}
	applyTermiosValue(t, name, value)
	return setTermios(fd, t)
}

func applyControlChar(fd int, name string, value uint8) error {
	t, err := getTermios(fd)
	if err != nil {
		return err
	}
	index, ok := controlCharIndex(name)
	if !ok || index >= len(t.Cc) {
		return fmt.Errorf("control character %s is not supported on this platform", name)
	}
	t.Cc[index] = value
	return setTermios(fd, t)
}

func applySpeed(fd int, which string, baud uint64) error {
	if err := validateSpeed(baud); err != nil {
		return err
	}
	code, _ := baudToNative(baud)
	t, err := getTermios(fd)
	if err != nil {
		return err
	}
	setNativeSpeed(t, which, code)
	return setTermios(fd, t)
}

func outputBaud(state *terminalState) uint64 { return nativeToBaud(state.Ospeed) }

func printReadableSettings(rc *tool.RunContext, s *terminalState, rows, cols int, all bool) {
	ispeed, ospeed := nativeToBaud(s.Ispeed), nativeToBaud(s.Ospeed)
	if ispeed == ospeed {
		fmt.Fprintf(rc.Out, "speed %d baud; rows %d; columns %d;\n", ospeed, rows, cols)
	} else {
		fmt.Fprintf(rc.Out, "ispeed %d baud; ospeed %d baud; rows %d; columns %d;\n", ispeed, ospeed, rows, cols)
	}
	if !all {
		return
	}
	for _, group := range [][]string{
		{"intr", "quit", "erase", "kill", "eof", "eol", "susp", "start", "stop"},
	} {
		for i, name := range group {
			if i > 0 {
				fmt.Fprint(rc.Out, "; ")
			}
			index, ok := controlCharIndex(name)
			if ok && index < len(s.Cc) {
				fmt.Fprintf(rc.Out, "%s = %s", name, formatControlChar(s.Cc[index]))
			}
		}
		fmt.Fprintln(rc.Out, ";")
	}
	if unix.VMIN < len(s.Cc) && unix.VTIME < len(s.Cc) {
		fmt.Fprintf(rc.Out, "min = %d; time = %d;\n", s.Cc[unix.VMIN], s.Cc[unix.VTIME])
	}
	printModeGroup(rc, s.Cflag, []flagMode{
		{"parenb", unix.PARENB}, {"parodd", unix.PARODD}, {"cstopb", unix.CSTOPB},
		{"hupcl", unix.HUPCL}, {"cread", unix.CREAD}, {"clocal", unix.CLOCAL},
	})
	cs := "cs5"
	switch s.Cflag & uint64(unix.CSIZE) {
	case uint64(unix.CS6):
		cs = "cs6"
	case uint64(unix.CS7):
		cs = "cs7"
	case uint64(unix.CS8):
		cs = "cs8"
	}
	fmt.Fprintln(rc.Out, cs+";")
	printModeGroup(rc, s.Iflag, []flagMode{
		{"ignbrk", unix.IGNBRK}, {"brkint", unix.BRKINT}, {"ignpar", unix.IGNPAR},
		{"parmrk", unix.PARMRK}, {"inpck", unix.INPCK}, {"istrip", unix.ISTRIP},
		{"inlcr", unix.INLCR}, {"igncr", unix.IGNCR}, {"icrnl", unix.ICRNL},
		{"ixon", unix.IXON}, {"ixany", unix.IXANY}, {"ixoff", unix.IXOFF},
	})
	outputModes := []flagMode{
		{"opost", unix.OPOST}, {"onlcr", unix.ONLCR}, {"ocrnl", unix.OCRNL},
		{"onocr", unix.ONOCR}, {"onlret", unix.ONLRET},
	}
	for _, name := range []string{"ofill", "ofdel"} {
		if bit, ok := platformOutputFlag(name); ok {
			outputModes = append(outputModes, flagMode{name, bit})
		}
	}
	printModeGroup(rc, s.Oflag, outputModes)
	if delays := platformDelayReport(s.Oflag); delays != "" {
		fmt.Fprintln(rc.Out, delays+";")
	}
	printModeGroup(rc, s.Lflag, []flagMode{
		{"isig", unix.ISIG}, {"icanon", unix.ICANON}, {"iexten", unix.IEXTEN},
		{"echo", unix.ECHO}, {"echoe", unix.ECHOE}, {"echok", unix.ECHOK},
		{"echonl", unix.ECHONL}, {"noflsh", unix.NOFLSH}, {"tostop", unix.TOSTOP},
	})
}

type flagMode struct {
	name string
	bit  uint64
}

func printModeGroup(rc *tool.RunContext, value uint64, modes []flagMode) {
	words := make([]string, 0, len(modes))
	for _, mode := range modes {
		name := mode.name
		if value&mode.bit == 0 {
			name = "-" + name
		}
		words = append(words, name)
	}
	fmt.Fprintln(rc.Out, strings.Join(words, " ")+";")
}

func formatControlChar(value byte) string {
	switch {
	case value == posixVDisable:
		return "undef"
	case value == 0x7f:
		return "^?"
	case value < 0x20:
		return "^" + string(value+'@')
	default:
		return string(value)
	}
}

func controlCharIndex(name string) (int, bool) {
	switch name {
	case "eof":
		return unix.VEOF, true
	case "eol":
		return unix.VEOL, true
	case "erase":
		return unix.VERASE, true
	case "intr":
		return unix.VINTR, true
	case "kill":
		return unix.VKILL, true
	case "quit":
		return unix.VQUIT, true
	case "susp":
		return unix.VSUSP, true
	case "start":
		return unix.VSTART, true
	case "stop":
		return unix.VSTOP, true
	default:
		return 0, false
	}
}

func applyTermiosMode(t *unix.Termios, mode string) {
	setFlag := func(word *termiosUint, bit uint64, enabled bool) {
		if enabled {
			*word |= termiosUint(bit)
		} else {
			*word &^= termiosUint(bit)
		}
	}
	base := strings.TrimPrefix(mode, "-")
	enabled := mode == base
	switch base {
	case "drain":
		return
	case "raw":
		if enabled {
			t.Cflag = (t.Cflag &^ termiosUint(unix.CSIZE)) | termiosUint(unix.CS8)
			t.Iflag &^= termiosUint(unix.INPCK)
			t.Oflag &^= termiosUint(unix.OPOST)
			for _, index := range []int{unix.VERASE, unix.VKILL, unix.VINTR, unix.VQUIT, unix.VEOF, unix.VEOL} {
				setControlChar(t, index, posixVDisable)
			}
		} else {
			applyTermiosMode(t, "cooked")
		}
	case "cooked":
		if enabled {
			t.Iflag |= termiosUint(unix.BRKINT | unix.IGNPAR | unix.ISTRIP | unix.ICRNL | unix.IXON)
			t.Oflag |= termiosUint(unix.OPOST)
			t.Lflag |= termiosUint(unix.ICANON | unix.ISIG)
			setControlChar(t, unix.VEOF, 4)
			setControlChar(t, unix.VEOL, 0)
		} else {
			applyTermiosMode(t, "raw")
		}
	case "cbreak":
		setFlag(&t.Lflag, unix.ICANON, !enabled)
	case "sane":
		t.Cflag &^= termiosUint(unix.PARENB | unix.PARODD | unix.CSIZE)
		t.Cflag |= termiosUint(unix.CREAD | unix.CS8)
		t.Iflag |= termiosUint(unix.BRKINT | unix.ICRNL | unix.IXON | unix.IMAXBEL)
		t.Iflag &^= termiosUint(unix.IGNBRK | unix.INLCR | unix.IGNCR | unix.IXOFF | unix.IXANY)
		t.Oflag |= termiosUint(unix.OPOST | unix.ONLCR)
		t.Oflag &^= termiosUint(unix.OCRNL | unix.ONOCR | unix.ONLRET)
		t.Lflag |= termiosUint(unix.ISIG | unix.ICANON | unix.IEXTEN | unix.ECHO | unix.ECHOE | unix.ECHOK)
		t.Lflag &^= termiosUint(unix.ECHONL | unix.NOFLSH | unix.TOSTOP)
	case "echo":
		setFlag(&t.Lflag, unix.ECHO, enabled)
	case "icanon":
		setFlag(&t.Lflag, unix.ICANON, enabled)
	case "isig":
		setFlag(&t.Lflag, unix.ISIG, enabled)
	case "iexten":
		setFlag(&t.Lflag, unix.IEXTEN, enabled)
	case "echoe":
		setFlag(&t.Lflag, unix.ECHOE, enabled)
	case "echok":
		setFlag(&t.Lflag, unix.ECHOK, enabled)
	case "echonl":
		setFlag(&t.Lflag, unix.ECHONL, enabled)
	case "noflsh":
		setFlag(&t.Lflag, unix.NOFLSH, enabled)
	case "tostop":
		setFlag(&t.Lflag, unix.TOSTOP, enabled)
	case "ixon":
		setFlag(&t.Iflag, unix.IXON, enabled)
	case "ixoff":
		setFlag(&t.Iflag, unix.IXOFF, enabled)
	case "ixany":
		setFlag(&t.Iflag, unix.IXANY, enabled)
	case "icrnl":
		setFlag(&t.Iflag, unix.ICRNL, enabled)
	case "ignbrk":
		setFlag(&t.Iflag, unix.IGNBRK, enabled)
	case "brkint":
		setFlag(&t.Iflag, unix.BRKINT, enabled)
	case "ignpar":
		setFlag(&t.Iflag, unix.IGNPAR, enabled)
	case "parmrk":
		setFlag(&t.Iflag, unix.PARMRK, enabled)
	case "inpck":
		setFlag(&t.Iflag, unix.INPCK, enabled)
	case "istrip":
		setFlag(&t.Iflag, unix.ISTRIP, enabled)
	case "inlcr":
		setFlag(&t.Iflag, unix.INLCR, enabled)
	case "igncr":
		setFlag(&t.Iflag, unix.IGNCR, enabled)
	case "opost":
		setFlag(&t.Oflag, unix.OPOST, enabled)
	case "onlcr":
		setFlag(&t.Oflag, unix.ONLCR, enabled)
	case "ocrnl":
		setFlag(&t.Oflag, unix.OCRNL, enabled)
	case "onocr":
		setFlag(&t.Oflag, unix.ONOCR, enabled)
	case "onlret":
		setFlag(&t.Oflag, unix.ONLRET, enabled)
	case "ofill", "ofdel":
		if bit, ok := platformOutputFlag(base); ok {
			setFlag(&t.Oflag, bit, enabled)
		}
	case "parenb":
		setFlag(&t.Cflag, unix.PARENB, enabled)
	case "parodd":
		setFlag(&t.Cflag, unix.PARODD, enabled)
	case "hupcl", "hup":
		setFlag(&t.Cflag, unix.HUPCL, enabled)
	case "cstopb":
		setFlag(&t.Cflag, unix.CSTOPB, enabled)
	case "cread":
		setFlag(&t.Cflag, unix.CREAD, enabled)
	case "clocal":
		setFlag(&t.Cflag, unix.CLOCAL, enabled)
	case "cs5":
		t.Cflag = (t.Cflag &^ termiosUint(unix.CSIZE)) | termiosUint(unix.CS5)
	case "cs6":
		t.Cflag = (t.Cflag &^ termiosUint(unix.CSIZE)) | termiosUint(unix.CS6)
	case "cs7":
		t.Cflag = (t.Cflag &^ termiosUint(unix.CSIZE)) | termiosUint(unix.CS7)
	case "cs8":
		t.Cflag = (t.Cflag &^ termiosUint(unix.CSIZE)) | termiosUint(unix.CS8)
	case "evenp", "parity":
		if enabled {
			t.Cflag = (t.Cflag &^ termiosUint(unix.PARODD|unix.CSIZE)) | termiosUint(unix.PARENB|unix.CS7)
		} else {
			t.Cflag = (t.Cflag &^ termiosUint(unix.PARENB|unix.CSIZE)) | termiosUint(unix.CS8)
		}
	case "oddp":
		if enabled {
			t.Cflag = (t.Cflag &^ termiosUint(unix.CSIZE)) | termiosUint(unix.PARENB|unix.PARODD|unix.CS7)
		} else {
			t.Cflag = (t.Cflag &^ termiosUint(unix.PARENB|unix.CSIZE)) | termiosUint(unix.CS8)
		}
	case "pass8", "litout":
		if enabled {
			t.Cflag = (t.Cflag &^ termiosUint(unix.PARENB|unix.CSIZE)) | termiosUint(unix.CS8)
			t.Iflag &^= termiosUint(unix.ISTRIP)
		} else {
			t.Cflag = (t.Cflag &^ termiosUint(unix.CSIZE)) | termiosUint(unix.PARENB|unix.CS7)
			t.Iflag |= termiosUint(unix.ISTRIP)
		}
		if base == "litout" {
			setFlag(&t.Oflag, unix.OPOST, !enabled)
		}
	case "nl":
		if enabled {
			t.Iflag &^= termiosUint(unix.ICRNL)
		} else {
			t.Iflag = (t.Iflag | termiosUint(unix.ICRNL)) &^ termiosUint(unix.INLCR|unix.IGNCR)
		}
	case "tabs":
		name := "tab0"
		if !enabled {
			name = "tab3"
		}
		setNamedDelay(&t.Oflag, name)
	case "cr0", "cr1", "cr2", "cr3":
		setNamedDelay(&t.Oflag, base)
	case "nl0", "nl1", "tab0", "tab1", "tab2", "tab3", "bs0", "bs1", "ff0", "ff1", "vt0", "vt1":
		setNamedDelay(&t.Oflag, base)
	case "ek":
		setControlChar(t, unix.VERASE, 0x7f)
		setControlChar(t, unix.VKILL, 0x15)
	case "decctlq":
		setFlag(&t.Iflag, unix.IXANY, enabled)
	case "dec":
		t.Lflag |= termiosUint(unix.ECHOE)
		t.Iflag &^= termiosUint(unix.IXANY)
		applyTermiosMode(t, "ek")
	case "crt":
		t.Lflag |= termiosUint(unix.ECHOE)
	}
}

func setNamedDelay(word *termiosUint, mode string) {
	if mask, value, ok := platformDelay(mode); ok {
		setDelay(word, mask, value)
	}
}

func isDelayMode(mode string) bool {
	for _, prefix := range []string{"cr", "nl", "tab", "bs", "ff", "vt"} {
		if strings.HasPrefix(mode, prefix) && mode != "nl" {
			return true
		}
	}
	return mode == "tabs"
}

func setDelay(word *termiosUint, mask, value uint64) {
	*word = (*word &^ termiosUint(mask)) | termiosUint(value)
}

func applyTermiosValue(t *unix.Termios, name string, value uint8) {
	if name == "min" {
		setControlChar(t, unix.VMIN, value)
	}
	if name == "time" {
		setControlChar(t, unix.VTIME, value)
	}
}

func setControlChar(t *unix.Termios, index int, value uint8) {
	if index >= 0 && index < len(t.Cc) {
		t.Cc[index] = value
	}
}

func applyWindowSize(fd int, rows, cols int) error {
	win, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		win = &unix.Winsize{}
	}
	if rows >= 0 {
		win.Row = uint16(rows)
	}
	if cols >= 0 {
		win.Col = uint16(cols)
	}
	return unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, win)
}
