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
		Iflag: unix.BRKINT | unix.INPCK | unix.ICRNL | unix.IXON | unix.IXOFF,
		Oflag: unix.OPOST | unix.ONLCR | unix.OCRNL,
		Cflag: unix.CS7 | unix.PARENB | unix.CREAD,
		Lflag: unix.ICANON | unix.ISIG | unix.ECHO,
	}
	tio.Cc[unix.VMIN] = 7
	tio.Cc[unix.VTIME] = 9
	applyTermiosMode(tio, "raw")
	if want := termiosUint(unix.BRKINT | unix.ICRNL | unix.IXON | unix.IXOFF); tio.Iflag != want {
		t.Fatalf("raw input flags=%#x, want only INPCK cleared (%#x)", tio.Iflag, want)
	}
	if want := termiosUint(unix.ONLCR | unix.OCRNL); tio.Oflag != want {
		t.Fatalf("raw output flags=%#x, want only OPOST cleared (%#x)", tio.Oflag, want)
	}
	if want := termiosUint(unix.ICANON | unix.ISIG | unix.ECHO); tio.Lflag != want {
		t.Fatalf("raw changed local flags: got %#x want %#x", tio.Lflag, want)
	}
	if tio.Cflag&termiosUint(unix.CSIZE) != termiosUint(unix.CS8) || tio.Cflag&termiosUint(unix.PARENB|unix.CREAD) != termiosUint(unix.PARENB|unix.CREAD) {
		t.Fatalf("raw did not select CS8 while preserving unrelated control flags: %#x", tio.Cflag)
	}
	for _, index := range []int{unix.VERASE, unix.VKILL, unix.VINTR, unix.VQUIT, unix.VEOF, unix.VEOL} {
		if tio.Cc[index] != posixVDisable {
			t.Errorf("raw cc[%d]=%d, want _POSIX_VDISABLE %d", index, tio.Cc[index], posixVDisable)
		}
	}
	if tio.Cc[unix.VMIN] != 7 || tio.Cc[unix.VTIME] != 9 {
		t.Fatalf("raw changed unrelated control chars VMIN=%d VTIME=%d", tio.Cc[unix.VMIN], tio.Cc[unix.VTIME])
	}
	t.Run("nl only changes input mapping", testApplyTermiosModeNLOnlyChangesInputMapping)
}

func testApplyTermiosModeNLOnlyChangesInputMapping(t *testing.T) {
	tio := &unix.Termios{
		Iflag: unix.ICRNL | unix.INLCR | unix.IGNCR,
		Oflag: unix.OPOST | unix.ONLCR | unix.OCRNL | unix.ONLRET,
	}
	wantOutput := tio.Oflag
	applyTermiosMode(tio, "nl")
	if tio.Iflag&termiosUint(unix.ICRNL) != 0 {
		t.Fatalf("nl left ICRNL set: %#x", tio.Iflag)
	}
	if tio.Iflag&termiosUint(unix.INLCR|unix.IGNCR) != termiosUint(unix.INLCR|unix.IGNCR) {
		t.Fatalf("nl changed unrelated input flags: %#x", tio.Iflag)
	}
	if tio.Oflag != wantOutput {
		t.Fatalf("nl changed output flags: got %#x want %#x", tio.Oflag, wantOutput)
	}
}

func TestApplyTermiosValueMinTime(t *testing.T) {
	tio := &unix.Termios{}
	applyTermiosValue(tio, "min", 3)
	applyTermiosValue(tio, "time", 7)
	if tio.Cc[unix.VMIN] != 3 || tio.Cc[unix.VTIME] != 7 {
		t.Fatalf("VMIN=%d VTIME=%d", tio.Cc[unix.VMIN], tio.Cc[unix.VTIME])
	}
	t.Run("required flag modes", testApplyTermiosRequiredFlagModes)
	t.Run("required delay modes", testApplyTermiosRequiredDelayModes)
}

func testApplyTermiosRequiredFlagModes(t *testing.T) {
	tests := []struct {
		name string
		bit  uint64
	}{
		{"hupcl", unix.HUPCL}, {"cstopb", unix.CSTOPB},
		{"cread", unix.CREAD}, {"clocal", unix.CLOCAL},
		{"ignbrk", unix.IGNBRK}, {"brkint", unix.BRKINT},
		{"ignpar", unix.IGNPAR}, {"parmrk", unix.PARMRK},
		{"inpck", unix.INPCK}, {"istrip", unix.ISTRIP},
		{"inlcr", unix.INLCR}, {"igncr", unix.IGNCR},
		{"ixany", unix.IXANY}, {"ocrnl", unix.OCRNL},
		{"onocr", unix.ONOCR}, {"onlret", unix.ONLRET},
		{"tostop", unix.TOSTOP},
	}
	for _, name := range []string{"ofill", "ofdel"} {
		if bit, ok := platformOutputFlag(name); ok {
			tests = append(tests, struct {
				name string
				bit  uint64
			}{name, bit})
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tio := &unix.Termios{}
			applyTermiosMode(tio, tt.name)
			var got uint64
			switch tt.name {
			case "hupcl", "cstopb", "cread", "clocal":
				got = uint64(tio.Cflag)
			case "ignbrk", "brkint", "ignpar", "parmrk", "inpck", "istrip", "inlcr", "igncr", "ixany":
				got = uint64(tio.Iflag)
			case "ocrnl", "onocr", "onlret", "ofill", "ofdel":
				got = uint64(tio.Oflag)
			default:
				got = uint64(tio.Lflag)
			}
			if got&tt.bit == 0 {
				t.Fatalf("%s did not set %#x (word %#x)", tt.name, tt.bit, got)
			}
			applyTermiosMode(tio, "-"+tt.name)
			switch tt.name {
			case "hupcl", "cstopb", "cread", "clocal":
				got = uint64(tio.Cflag)
			case "ignbrk", "brkint", "ignpar", "parmrk", "inpck", "istrip", "inlcr", "igncr", "ixany":
				got = uint64(tio.Iflag)
			case "ocrnl", "onocr", "onlret", "ofill", "ofdel":
				got = uint64(tio.Oflag)
			default:
				got = uint64(tio.Lflag)
			}
			if got&tt.bit != 0 {
				t.Fatalf("-%s did not clear %#x (word %#x)", tt.name, tt.bit, got)
			}
		})
	}
}

func testApplyTermiosRequiredDelayModes(t *testing.T) {
	for _, mode := range []string{"cr0", "cr1", "cr2", "cr3", "nl0", "nl1", "tab0", "tab1", "tab2", "tab3", "bs0", "bs1", "ff0", "ff1", "vt0", "vt1"} {
		t.Run(mode, func(t *testing.T) {
			tio := &unix.Termios{Oflag: ^termiosUint(0)}
			applyTermiosMode(tio, mode)
			// Applying the same mode twice must be stable and preserve unrelated bits.
			first := tio.Oflag
			applyTermiosMode(tio, mode)
			if tio.Oflag != first {
				t.Fatalf("%s is not idempotent: %#x then %#x", mode, first, tio.Oflag)
			}
		})
	}
}
