//go:build linux || darwin || freebsd || netbsd || openbsd

package sttycmd

import "golang.org/x/sys/unix"

func applyTermiosMode(t *unix.Termios, mode string) {
	switch mode {
	case "drain", "-drain":
		return
	case "raw", "-cooked":
		t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.IGNPAR | unix.PARMRK | unix.INPCK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON | unix.IXOFF
		t.Oflag &^= unix.OPOST
		t.Lflag &^= unix.ICANON | unix.ISIG
		setControlChar(t, unix.VMIN, 1)
		setControlChar(t, unix.VTIME, 0)
	case "-raw", "cooked":
		t.Iflag |= unix.BRKINT | unix.IGNPAR | unix.ISTRIP | unix.ICRNL | unix.IXON
		t.Oflag |= unix.OPOST
		t.Lflag |= unix.ICANON | unix.ISIG
		setControlChar(t, unix.VEOF, 4)
		setControlChar(t, unix.VEOL, 0)
	case "cbreak":
		t.Lflag &^= unix.ICANON
	case "-cbreak":
		t.Lflag |= unix.ICANON
	case "sane":
		t.Cflag |= unix.CREAD | unix.CS8
		t.Cflag &^= unix.PARENB | unix.PARODD | unix.CSIZE
		t.Cflag |= unix.CS8
		t.Iflag |= unix.BRKINT | unix.ICRNL | unix.IXON | unix.IMAXBEL
		t.Iflag &^= unix.IGNBRK | unix.INLCR | unix.IGNCR | unix.IXOFF | unix.IXANY
		t.Oflag |= unix.OPOST | unix.ONLCR
		t.Oflag &^= unix.OCRNL | unix.ONOCR | unix.ONLRET
		t.Lflag |= unix.ISIG | unix.ICANON | unix.IEXTEN | unix.ECHO | unix.ECHOE | unix.ECHOK | unix.ECHOCTL | unix.ECHOKE
		t.Lflag &^= unix.ECHONL | unix.NOFLSH | unix.TOSTOP | unix.ECHOPRT | unix.EXTPROC | unix.FLUSHO
	case "echo":
		t.Lflag |= unix.ECHO
	case "-echo":
		t.Lflag &^= unix.ECHO
	case "icanon":
		t.Lflag |= unix.ICANON
	case "-icanon":
		t.Lflag &^= unix.ICANON
	case "isig":
		t.Lflag |= unix.ISIG
	case "-isig":
		t.Lflag &^= unix.ISIG
	case "iexten":
		t.Lflag |= unix.IEXTEN
	case "-iexten":
		t.Lflag &^= unix.IEXTEN
	case "echoe":
		t.Lflag |= unix.ECHOE
	case "-echoe":
		t.Lflag &^= unix.ECHOE
	case "echok":
		t.Lflag |= unix.ECHOK
	case "-echok":
		t.Lflag &^= unix.ECHOK
	case "echonl":
		t.Lflag |= unix.ECHONL
	case "-echonl":
		t.Lflag &^= unix.ECHONL
	case "noflsh":
		t.Lflag |= unix.NOFLSH
	case "-noflsh":
		t.Lflag &^= unix.NOFLSH
	case "ixon":
		t.Iflag |= unix.IXON
	case "-ixon":
		t.Iflag &^= unix.IXON
	case "ixoff":
		t.Iflag |= unix.IXOFF
	case "-ixoff":
		t.Iflag &^= unix.IXOFF
	case "icrnl":
		t.Iflag |= unix.ICRNL
	case "-icrnl":
		t.Iflag &^= unix.ICRNL
	case "opost":
		t.Oflag |= unix.OPOST
	case "-opost":
		t.Oflag &^= unix.OPOST
	case "onlcr":
		t.Oflag |= unix.ONLCR
	case "-onlcr":
		t.Oflag &^= unix.ONLCR
	case "parenb":
		t.Cflag |= unix.PARENB
	case "-parenb":
		t.Cflag &^= unix.PARENB
	case "parodd":
		t.Cflag |= unix.PARODD
	case "-parodd":
		t.Cflag &^= unix.PARODD
	case "cs5":
		t.Cflag = (t.Cflag &^ unix.CSIZE) | unix.CS5
	case "cs6":
		t.Cflag = (t.Cflag &^ unix.CSIZE) | unix.CS6
	case "cs7":
		t.Cflag = (t.Cflag &^ unix.CSIZE) | unix.CS7
	case "cs8":
		t.Cflag = (t.Cflag &^ unix.CSIZE) | unix.CS8
	case "evenp", "parity":
		t.Cflag |= unix.PARENB
		t.Cflag &^= unix.PARODD | unix.CSIZE
		t.Cflag |= unix.CS7
	case "-evenp", "-parity":
		t.Cflag &^= unix.PARENB | unix.CSIZE
		t.Cflag |= unix.CS8
	case "oddp":
		t.Cflag |= unix.PARENB | unix.PARODD
		t.Cflag &^= unix.CSIZE
		t.Cflag |= unix.CS7
	case "-oddp":
		t.Cflag &^= unix.PARENB | unix.CSIZE
		t.Cflag |= unix.CS8
	case "pass8":
		t.Cflag &^= unix.PARENB | unix.CSIZE
		t.Cflag |= unix.CS8
		t.Iflag &^= unix.ISTRIP
	case "-pass8":
		t.Cflag |= unix.PARENB
		t.Cflag &^= unix.CSIZE
		t.Cflag |= unix.CS7
		t.Iflag |= unix.ISTRIP
	case "litout":
		t.Cflag &^= unix.PARENB | unix.CSIZE
		t.Cflag |= unix.CS8
		t.Iflag &^= unix.ISTRIP
		t.Oflag &^= unix.OPOST
	case "-litout":
		t.Cflag |= unix.PARENB
		t.Cflag &^= unix.CSIZE
		t.Cflag |= unix.CS7
		t.Iflag |= unix.ISTRIP
		t.Oflag |= unix.OPOST
	case "nl":
		t.Iflag &^= unix.ICRNL
		t.Oflag &^= unix.ONLCR
	case "-nl":
		t.Iflag |= unix.ICRNL
		t.Iflag &^= unix.INLCR | unix.IGNCR
		t.Oflag |= unix.ONLCR
		t.Oflag &^= unix.OCRNL | unix.ONLRET
	case "crt":
		t.Lflag |= unix.ECHOE | unix.ECHOCTL | unix.ECHOKE
	case "dec":
		t.Lflag |= unix.ECHOE | unix.ECHOCTL | unix.ECHOKE
		t.Iflag &^= unix.IXANY
	case "decctlq":
		t.Iflag |= unix.IXANY
	case "-decctlq":
		t.Iflag &^= unix.IXANY
	}
}

func applyTermiosValue(t *unix.Termios, name string, value uint8) {
	switch name {
	case "min":
		setControlChar(t, unix.VMIN, value)
	case "time":
		setControlChar(t, unix.VTIME, value)
	}
}

func setControlChar(t *unix.Termios, index int, value uint8) {
	if index >= 0 && index < len(t.Cc) {
		t.Cc[index] = value
	}
}
