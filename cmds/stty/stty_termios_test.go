//go:build linux || darwin || freebsd || netbsd || openbsd

package sttycmd

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestApplyTermiosModeCbreak(t *testing.T) {
	tio := &unix.Termios{Lflag: unix.ICANON | unix.ECHO}
	applyTermiosMode(tio, "cbreak")
	if tio.Lflag&unix.ICANON != 0 {
		t.Fatalf("cbreak left ICANON set: %#x", tio.Lflag)
	}
	if tio.Lflag&unix.ECHO == 0 {
		t.Fatalf("cbreak unexpectedly cleared ECHO: %#x", tio.Lflag)
	}

	applyTermiosMode(tio, "-cbreak")
	if tio.Lflag&unix.ICANON == 0 {
		t.Fatalf("-cbreak did not restore ICANON: %#x", tio.Lflag)
	}
}

func TestApplyTermiosModeRaw(t *testing.T) {
	tio := &unix.Termios{
		Iflag: unix.BRKINT | unix.ICRNL | unix.IXON | unix.IXOFF,
		Oflag: unix.OPOST,
		Lflag: unix.ICANON | unix.ISIG | unix.ECHO,
	}
	applyTermiosMode(tio, "raw")
	if tio.Iflag&(unix.BRKINT|unix.ICRNL|unix.IXON|unix.IXOFF) != 0 {
		t.Fatalf("raw left input flags set: %#x", tio.Iflag)
	}
	if tio.Oflag&unix.OPOST != 0 {
		t.Fatalf("raw left OPOST set: %#x", tio.Oflag)
	}
	if tio.Lflag&(unix.ICANON|unix.ISIG) != 0 {
		t.Fatalf("raw left local flags set: %#x", tio.Lflag)
	}
	if tio.Lflag&unix.ECHO == 0 {
		t.Fatalf("raw should match uutils and leave ECHO unchanged: %#x", tio.Lflag)
	}
	if tio.Cc[unix.VMIN] != 1 || tio.Cc[unix.VTIME] != 0 {
		t.Fatalf("raw control chars VMIN=%d VTIME=%d", tio.Cc[unix.VMIN], tio.Cc[unix.VTIME])
	}
}

func TestApplyTermiosValueMinTime(t *testing.T) {
	tio := &unix.Termios{}
	applyTermiosValue(tio, "min", 3)
	applyTermiosValue(tio, "time", 7)
	if tio.Cc[unix.VMIN] != 3 || tio.Cc[unix.VTIME] != 7 {
		t.Fatalf("VMIN=%d VTIME=%d", tio.Cc[unix.VMIN], tio.Cc[unix.VTIME])
	}
}
